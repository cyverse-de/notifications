package db

import (
	"database/sql"
	"encoding/json"
	"fmt"

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
func V1ListNotifications(tx *sql.Tx, params *V1NotificationListingParameters) (*model.NotificationListing, error) {
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
		Where(sq.Eq{"u.username": params.User})

	// Apply the seen parameter if requested.
	if params.Seen != nil {
		queryBuilder = queryBuilder.Where(sq.Eq{"n.seen": *params.Seen})
	}

	// Apply the notification type parameter if requested.
	if params.NotificationType != "" {
		queryBuilder = queryBuilder.Where(sq.Eq{"nt.name": params.NotificationType})
	}

	// Apply the limit if requested.
	if params.Limit != 0 {
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

	result := &model.NotificationListing{
		Messages: listing,
		Total:    total,
	}
	return result, nil
}
