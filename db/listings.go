package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
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

	// If true, only the notification counts will be returned.
	CountOnly bool

	// If specified, only messages with subjects matching the given search string will be returned.
	SubjectSearch string
}

// v2SubjectSearchString converts a string to a search string suitable for a subject search.
func v2SubjectSearchString(s string) string {
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, "%", "\\%")
	s = strings.ReplaceAll(s, "_", "\\_")
	s = fmt.Sprintf("%%%s%%", s)
	return s
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

	// Apply the subject search parameter if it was specified.
	if params.SubjectSearch != "" {
		queryBuilder = queryBuilder.Where(sq.Like{"lower(n.subject)": v2SubjectSearchString(params.SubjectSearch)})
	}

	return queryBuilder
}

// v2AddPaginationSettings adds pagination settings to a notification listing query for version 2 of the API.
func v2AddPaginationSettings(
	listingQueryBuilder sq.SelectBuilder,
	params *V2NotificationListingParameters,
) (sq.SelectBuilder, error) {

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

	return listingQueryBuilder, nil
}

// v2ApplySortOrder applies the sort order to a notification listing for version 2 of the API.
func v2ApplySortOrder(queryBuilder sq.SelectBuilder, sortOrder query.SortOrder) sq.SelectBuilder {
	return queryBuilder.OrderBy(fmt.Sprintf("n.time_created %s, n.id %s", sortOrder, sortOrder))
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

	// Sort the inner query based on the
	innerSortOrder := params.SortOrder
	if params.AfterTimestamp != nil {
		innerSortOrder = query.SortOrderAscending
	} else if params.BeforeTimestamp != nil {
		innerSortOrder = query.SortOrderDescending
	}
	listingQueryBuilder = v2ApplySortOrder(listingQueryBuilder, innerSortOrder)

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

// runBoundaryIDQuery runs a single boundary ID query and returns the result.
func runBoundaryIDQuery(
	tx *sql.Tx,
	queryBuilder sq.SelectBuilder,
) (string, error) {

	// Build the query.
	query, args, err := queryBuilder.ToSql()
	if err != nil {
		return "", err
	}

	// Execute the query.
	rows, err := tx.Query(query, args...)
	if err != nil {
		return "", err
	}
	defer rows.Close()

	// Get the ID of the next message if applicable.
	var id string
	if rows.Next() {
		err = rows.Scan(&id)
		if err != nil {
			return "", err
		}
	}

	return id, nil
}

// v2GetBoundaryIDs obtains the IDs of the messages just beyond the boundaries of the current page.
func v2GetBoundaryIDs(tx *sql.Tx, params *V2NotificationListingParameters) (string, string, error) {
	var beforeID, afterID string
	var err error

	// Begin building the query.
	queryBuilder := v2ListNotificationsBaseQuery(params).Column("n.id")

	// Handle the case where the before ID was specified.
	if params.BeforeID != "" {
		if params.Limit > 0 {
			beforeID, err = runBoundaryIDQuery(
				tx,
				v2ApplySortOrder(queryBuilder, query.SortOrderDescending).
					Where(sq.LtOrEq{"n.id": params.BeforeID}).
					Where(sq.LtOrEq{"n.time_created": params.BeforeTimestamp}).
					Offset(params.Limit).
					Limit(1),
			)
			if err != nil {
				return "", "", err
			}
		}

		afterID, err = runBoundaryIDQuery(
			tx,
			v2ApplySortOrder(queryBuilder, query.SortOrderAscending).
				Where(sq.GtOrEq{"n.id": params.BeforeID}).
				Where(sq.GtOrEq{"n.time_created": params.BeforeTimestamp}).
				Offset(1).
				Limit(1),
		)
		if err != nil {
			return "", "", err
		}

		return beforeID, afterID, nil
	}

	// Handle the case where the after ID was specified.
	if params.AfterID != "" {
		if params.Limit > 0 {
			afterID, err = runBoundaryIDQuery(
				tx,
				v2ApplySortOrder(queryBuilder, query.SortOrderAscending).
					Where(sq.GtOrEq{"n.id": params.AfterID}).
					Where(sq.GtOrEq{"n.time_created": params.AfterTimestamp}).
					Offset(params.Limit).
					Limit(1),
			)
			if err != nil {
				return "", "", err
			}
		}

		beforeID, err = runBoundaryIDQuery(
			tx,
			v2ApplySortOrder(queryBuilder, query.SortOrderDescending).
				Where(sq.LtOrEq{"n.id": params.AfterID}).
				Where(sq.LtOrEq{"n.time_created": params.AfterTimestamp}).
				Offset(1).
				Limit(1),
		)
		if err != nil {
			return "", "", err
		}

		return beforeID, afterID, nil
	}

	// Handle the case where no boundary ID was specified and the sort order is ascending.
	if params.SortOrder == query.SortOrderAscending {
		beforeID = ""

		if params.Limit > 0 {
			afterID, err = runBoundaryIDQuery(
				tx,
				v2ApplySortOrder(queryBuilder, query.SortOrderAscending).
					Offset(params.Limit).
					Limit(1),
			)
			if err != nil {
				return "", "", err
			}
		} else {
			afterID = ""
		}

		return beforeID, afterID, nil
	}

	// Handle the case where no boundary ID was specified and the sort order is descending.
	if params.SortOrder == query.SortOrderDescending {
		afterID = ""

		if params.Limit > 0 {
			beforeID, err = runBoundaryIDQuery(
				tx,
				v2ApplySortOrder(queryBuilder, query.SortOrderDescending).
					Offset(params.Limit).
					Limit(1),
			)
			if err != nil {
				return "", "", err
			}
		} else {
			beforeID = ""
		}

		return beforeID, afterID, nil
	}

	// If execution get to this point, we have problems.
	return "", "", fmt.Errorf("unexpected condition encountered while obtaining boundary IDs")
}

// V2ListNotifications lists notifications for a user.
func V2ListNotifications(tx *sql.Tx, params *V2NotificationListingParameters) (*model.V2NotificationListing, error) {
	wrapMsg := "unable to obtain the notification listing"
	var err error

	// Obtain the listing.
	var listing []*model.Notification
	if !params.CountOnly {
		listing, err = v2GetListing(tx, params)
		if err != nil {
			return nil, errors.Wrap(err, wrapMsg)
		}
	}

	// Obtain the count.
	total, err := v2GetNotificationCount(tx, params)
	if err != nil {
		return nil, errors.Wrap(err, wrapMsg)
	}

	// Get the boundary IDs.
	var beforeID, afterID string
	if !params.CountOnly {
		beforeID, afterID, err = v2GetBoundaryIDs(tx, params)
		if err != nil {
			return nil, errors.Wrap(err, wrapMsg)
		}
	}

	result := &model.V2NotificationListing{
		Messages: listing,
		Total:    total,
		BeforeID: beforeID,
		AfterID:  afterID,
	}
	return result, nil
}
