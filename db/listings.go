package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/cyverse-de/notifications/model"
	"github.com/cyverse-de/notifications/query"
	"github.com/pkg/errors"

	sq "github.com/Masterminds/squirrel"
)

// formatNotification formats a single outgoing notification.
func formatNotification(
	messageText []byte,
	notificationType string,
	seen bool,
	deleted bool,
) (*model.Notification, error) {
	wrapMsg := "unable to format the notification"

	// Unmarshal the notification.
	var message model.Notification
	err := json.Unmarshal(messageText, &message)
	if err != nil {
		return nil, errors.Wrap(err, wrapMsg)
	}

	// Update any fields that we need to update.
	message.Type = notificationType
	message.Seen = seen
	message.Deleted = deleted

	return &message, nil
}

// V1NotificationListingParameters describes the parameters available for listing notifications.
type V1NotificationListingParameters struct {
	User             string
	Offset           uint64
	Limit            uint64
	Seen             *bool
	SortOrder        query.SortOrder
	SortField        query.V1ListingSortField
	NotificationType string
}

// getNotificationListingSortColumn returns the sort column to use for a V1ListingSortField value.
func getV1NotificationListingSortColumn(sortField query.V1ListingSortField) (string, error) {
	switch sortField {
	case query.V1ListingSortFieldTimestamp, query.V1ListingSortFieldDateCreated:
		return "n.time_created", nil
	case query.V1ListingSortFieldUUID:
		return "n.id", nil
	case query.V1ListingSortFieldSubject:
		return "n.subject", nil
	}
	return "", fmt.Errorf("unrecognized sort field: %s", string(sortField))
}

// V1ListNotifications lists notifications for a user.
func V1ListNotifications(tx *sql.Tx, params *V1NotificationListingParameters) (*model.V1NotificationListing, error) {
	wrapMsg := "unable to obtain the notification listing"

	// Begin building the query.
	queryBuilder := sq.StatementBuilder.
		PlaceholderFormat(sq.Dollar).
		Select().
		Column("nt.name AS type").
		Column("n.seen").
		Column("n.deleted").
		Column("n.outgoing_json AS message").
		Column("count(*) OVER () AS total").
		From("notifications n").
		Join("users u ON n.user_id = u.id").
		Join("notification_types nt ON n.notification_type_id = nt.id").
		Where(sq.Eq{"u.username": params.User}).
		Where(sq.Eq{"n.deleted": false})

	// Apply the seen parameter if requested.
	if params.Seen != nil {
		queryBuilder = queryBuilder.Where(sq.Eq{"n.seen": *params.Seen})
	}

	// Apply the notification type parameter if requested.
	if params.NotificationType != "" {
		queryBuilder = queryBuilder.Where(sq.Eq{"nt.name": params.NotificationType})
	}

	// Apply the limit if requested.
	if params.Limit > 0 {
		queryBuilder = queryBuilder.Limit(params.Limit)
	}

	// Apply the offset if requested.
	if params.Offset != 0 {
		queryBuilder = queryBuilder.Offset(params.Offset)
	}

	// Apply sorting.
	sortColumn, err := getV1NotificationListingSortColumn(params.SortField)
	if err != nil {
		return nil, errors.Wrap(err, wrapMsg)
	}
	queryBuilder = queryBuilder.OrderBy(fmt.Sprintf("%s %s", sortColumn, string(params.SortOrder)))

	// Build the query.
	query, args, err := queryBuilder.ToSql()
	if err != nil {
		return nil, errors.Wrap(err, wrapMsg)
	}

	// Query the database.
	rows, err := tx.Query(query, args...)
	if err != nil {
		return nil, errors.Wrap(err, wrapMsg)
	}
	defer rows.Close()

	// Build the listing from the result set.
	var total int
	listing := make([]*model.Notification, 0)
	for rows.Next() {
		var notificationType string
		var messageText []byte
		var seen, deleted bool

		// Fetch the data for the current row from the database.
		err = rows.Scan(&notificationType, &seen, &deleted, &messageText, &total)
		if err != nil {
			return nil, errors.Wrap(err, wrapMsg)
		}

		// Unmarshal the message and plug in any values that might have changed.
		message, err := formatNotification(messageText, notificationType, seen, deleted)
		if err != nil {
			return nil, errors.Wrap(err, wrapMsg)
		}

		listing = append(listing, message)
	}

	result := &model.V1NotificationListing{
		Messages: listing,
		Total:    total,
	}
	return result, nil
}

// V1NotificationCountingParameters describes the parameters available for counting notification messages.
type V1NotificationCountingParameters struct {
	User             string
	Seen             *bool
	NotificationType string
}

// V1CountNotifications counts notifications for a user.
func V1CountNotifications(tx *sql.Tx, params *V1NotificationCountingParameters) (*model.V1NotificationCounts, error) {
	wrapMsg := "unable to obtain the notification counts"

	// Begin building the query.
	queryBuilder := sq.StatementBuilder.
		PlaceholderFormat(sq.Dollar).
		Select().
		Column("count(*)").
		From("notifications n").
		Join("users u ON n.user_id = u.id").
		Join("notification_types nt ON n.notification_type_id = nt.id").
		Where(sq.Eq{"u.username": params.User}).
		Where(sq.Eq{"n.deleted": false})

	// Apply the seen parameter if requested.
	if params.Seen != nil {
		queryBuilder = queryBuilder.Where(sq.Eq{"n.seen": *params.Seen})
	}

	// Apply the notification type parameter if requested.
	if params.NotificationType != "" {
		queryBuilder = queryBuilder.Where(sq.Eq{"nt.name": params.NotificationType})
	}

	// Build the query.
	query, args, err := queryBuilder.ToSql()
	if err != nil {
		return nil, errors.Wrap(err, wrapMsg)
	}

	// Query the database and extract the count.
	var result = &model.V1NotificationCounts{}
	row := tx.QueryRow(query, args...)
	err = row.Scan(&result.UserTotal)
	if err != nil {
		return nil, errors.Wrap(err, wrapMsg)
	}

	return result, nil
}

// V2NotificationListingParameters describes the parameters available for listing notifications.
type V2NotificationListingParameters struct {

	// The username of the user we're listing notifications for.
	User string

	// The maximum number of results to return.
	Limit uint64

	// If true, only notifications that have been marked as seen will be returned.
	Seen bool

	// The sort order of the listing.
	SortOrder query.SortOrder

	// If specified, only messages that would appear before the message with the given ID will be included in the
	// listing. BeforeID and BeforeTimestamp must be used in conjunction with each other and BeforeTimestamp must
	// be the timestamp associated with BeforeID. These parameters are used in this manner because the caller has to
	// ensure that the message associated with BeforeID exists. Since the existence check has to be done by the
	// caller, the message timestamp might as well be retrieved at the same time.
	BeforeID        string
	BeforeTimestamp *time.Time

	// If specified, only messages that would appear after the message with the givn ID will be included in the
	// listing. AfterID and AfterTimestamp must be used in conjunction with each otehr and AfterTimestamp must be the
	// timestamp associated with AfterID. These parameters are used in this manner because the caller has to ensure
	// that the message associated with AfterID exists. Since the existence check has to be done by the caller, the
	// message timestamp might as well be retrieved at the same time.
	AfterID        string
	AfterTimestamp *time.Time
}

// v2ListNotificationsBaseQuery builds the base query for notification listings in version 2 of the API.
func v2ListNotificationsBaseQuery(params *V2NotificationListingParameters) sq.SelectBuilder {

	// Begin building the base query.
	queryBuilder := sq.StatementBuilder.
		PlaceholderFormat(sq.Dollar).
		Select().
		From("notifications n").
		Join("users u ON n.user_id = u.id").
		Join("notification_types nt ON n.notification_type_id = nt.id").
		Where(sq.Eq{"u.username": params.User}).
		Where(sq.Eq{"n.deleted": false})

	// Apply the seen parameter if the user didn't request to see messages that have been marked as seen.
	if !params.Seen {
		queryBuilder = queryBuilder.Where(sq.Eq{"n.seen": false})
	}

	return queryBuilder
}

// v2AddPaginationSettings adds pagination settings to a notification listing query for version 2 of the API.
func v2AddPaginationSettings(listingQueryBuilder sq.SelectBuilder, params *V2NotificationListingParameters) (
	sq.SelectBuilder, error,
) {
	// Add the parameters to get to the correct page.
	if params.BeforeID != "" {
		if params.BeforeTimestamp == nil {
			return listingQueryBuilder, fmt.Errorf("before ID provided without before timestamp")
		}
		listingQueryBuilder = listingQueryBuilder.
			Where(sq.LtOrEq{"n.id": params.BeforeID}).
			Where(sq.LtOrEq{"n.time_created": *params.BeforeTimestamp})
	}
	if params.AfterID != "" {
		if params.AfterTimestamp == nil {
			return listingQueryBuilder, fmt.Errorf("after ID provided without after timestamp")
		}
		listingQueryBuilder = listingQueryBuilder.
			Where(sq.GtOrEq{"n.id": params.AfterID}).
			Where(sq.GtOrEq{"n.time_created": *params.AfterTimestamp})
	}

	// Add the sort order so that we can be sure to get the right results.
	innerSortOrder := params.SortOrder
	if params.AfterTimestamp != nil {
		innerSortOrder = query.SortOrderAscending
	} else if params.BeforeTimestamp != nil {
		innerSortOrder = query.SortOrderDescending
	}
	listingQueryBuilder = listingQueryBuilder.OrderBy(
		fmt.Sprintf("n.time_created %s, n.id %s", innerSortOrder, innerSortOrder),
	)

	return listingQueryBuilder, nil
}

// v2GetListing returns an acutal notification listing for version 2 of the API.
func v2GetListing(tx *sql.Tx, params *V2NotificationListingParameters) ([]*model.Notification, error) {

	// Begin building the query.
	listingQueryBuilder := v2ListNotificationsBaseQuery(params).
		Column("nt.name AS type").
		Column("n.seen").
		Column("n.deleted").
		Column("n.outgoing_json AS message").
		Column("n.id").
		Column("n.time_created AS time_created")

	// Add the pagination settings. These only apply to the listing query.
	listingQueryBuilder, err := v2AddPaginationSettings(listingQueryBuilder, params)
	if err != nil {
		return nil, err
	}

	// Apply the limit.
	if params.Limit > 0 {
		listingQueryBuilder = listingQueryBuilder.Limit(params.Limit)
	}

	// Generate the listing query.
	listingQuery, listingQueryArgs, err := listingQueryBuilder.ToSql()
	if err != nil {
		return nil, err
	}

	// Embed the listing query in an outer query to ensure that the notifictions are sorted correctly.
	listingQuery = fmt.Sprintf(
		"SELECT * FROM (%s) AS listing ORDER BY time_created %s, id %s",
		listingQuery,
		params.SortOrder,
		params.SortOrder,
	)

	// Obtain the result set for the listing.
	listingRows, err := tx.Query(listingQuery, listingQueryArgs...)
	if err != nil {
		return nil, err
	}
	defer listingRows.Close()

	// Build the listing from the result set.
	listing := make([]*model.Notification, 0)
	for listingRows.Next() {
		var notificationType string
		var messageText []byte
		var seen, deleted bool
		var id, timeCreated string

		// Fetch the data for the current row from the database.
		err = listingRows.Scan(&notificationType, &seen, &deleted, &messageText, &id, &timeCreated)
		if err != nil {
			return nil, err
		}

		// Unmarshal the message and plug in any values that might have changed.
		message, err := formatNotification(messageText, notificationType, seen, deleted)
		if err != nil {
			return nil, err
		}

		listing = append(listing, message)
	}

	return listing, nil
}

// v2GetNotificationCount determines the total number of notifications that match a notification listing for version 2
// of the API, not considering paging parameters.
func v2GetNotificationCount(tx *sql.Tx, params *V2NotificationListingParameters) (int, error) {
	var total int
	countQuery, countQueryArgs, err := v2ListNotificationsBaseQuery(params).Column("count(*) AS count").ToSql()
	if err != nil {
		return 0, err
	}
	err = tx.QueryRow(countQuery, countQueryArgs...).Scan(&total)
	if err != nil {
		return 0, err
	}
	return total, nil
}

// v2GetNextBeforeID obtains the ID of the most recent message that precedes all messages on the current page.
func v2GetNextBeforeID(tx *sql.Tx, params *V2NotificationListingParameters) (string, error) {

	// If the before ID was specified then we can simply return it.
	if params.BeforeID != "" {
		return params.BeforeID, nil
	}

	// If there's no limit and the before ID was not specified in the request then there can't be a new before ID.
	if params.Limit == 0 {
		return "", nil
	}

	// If the sort order is descending and no before ID was specified then we're on the first page.
	if params.SortOrder == query.SortOrderDescending {
		return "", nil
	}

	// We've accounted for the simple cases; we'll need to use a query to get the before ID.
	queryBuilder := v2ListNotificationsBaseQuery(params).Column("n.id")
	queryBuilder, err := v2AddPaginationSettings(queryBuilder, params)
	if err != nil {
		return "", err
	}

	// Apply the offset and limit.
	queryBuilder = queryBuilder.Offset(params.Limit).Limit(1)

	// Build the query.
	query, args, err := queryBuilder.ToSql()
	if err != nil {
		return "", err
	}

	// Get the identifier.
	var id string
	err = tx.QueryRow(query, args...).Scan(&id)
	if err != nil {
		return "", err
	}

	return id, err
}

// v2GetNextAfterID obtains the ID of the oldest message that succeeds all messages on the current page.
func v2GetNextAfterID(tx *sql.Tx, params *V2NotificationListingParameters) (string, error) {

	// If the after ID was specified then we can simply return it.
	if params.AfterID != "" {
		return params.AfterID, nil
	}

	// If there's no limit and the after ID was not specified in the request then there can't be a new after ID.
	if params.Limit == 0 {
		return "", nil
	}

	// If the sort order is ascending and no after ID was specified then we're on the first page.
	if params.SortOrder == query.SortOrderAscending {
		return "", nil
	}

	// We've accounted for the simple cases; we'll need to use a query to get the after ID.
	queryBuilder := v2ListNotificationsBaseQuery(params).Column("n.id")
	queryBuilder, err := v2AddPaginationSettings(queryBuilder, params)
	if err != nil {
		return "", err
	}

	// Apply the offset and limit.
	queryBuilder = queryBuilder.Offset(params.Limit).Limit(1)

	// Build the query.
	query, args, err := queryBuilder.ToSql()
	if err != nil {
		return "", err
	}

	// Get the identifier.
	var id string
	err = tx.QueryRow(query, args...).Scan(&id)
	if err != nil {
		return "", err
	}

	return id, err
}

// V2ListNotifications lists notifications for a user.
func V2ListNotifications(tx *sql.Tx, params *V2NotificationListingParameters) (*model.V2NotificationListing, error) {
	wrapMsg := "unable to obtain the notification listing"

	// Obtain the listing.
	listing, err := v2GetListing(tx, params)
	if err != nil {
		return nil, errors.Wrap(err, wrapMsg)
	}

	// Obtain the count.
	total, err := v2GetNotificationCount(tx, params)
	if err != nil {
		return nil, errors.Wrap(err, wrapMsg)
	}

	// Obtain the ID of the most recent message that precedes all messages on the current page.
	beforeID, err := v2GetNextBeforeID(tx, params)
	if err != nil {
		return nil, errors.Wrap(err, wrapMsg)
	}

	// Obtain the ID of the oldest message that succeeds all messages on the current page.
	afterID, err := v2GetNextAfterID(tx, params)
	if err != nil {
		return nil, errors.Wrap(err, wrapMsg)
	}

	result := &model.V2NotificationListing{
		Messages: listing,
		Total:    total,
		BeforeID: beforeID,
		AfterID:  afterID,
	}
	return result, nil
}
