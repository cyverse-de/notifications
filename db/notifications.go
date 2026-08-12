package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/cyverse-de/messaging/v12"
	"github.com/cyverse-de/notifications/common"
	"github.com/pkg/errors"

	sq "github.com/Masterminds/squirrel"
)

// CountUnreadNotifications counts the number of notifications for the user that haven't been marked as read.
func CountUnreadNotifications(ctx context.Context, tx *sql.Tx, user string) (int64, error) {
	wrapMsg := "unable to count unread notifications"
	var total int64

	// Build the statement to count the unread notifications.
	statement, args, err := psql.Select("count(*)").
		From("notifications n").
		Join("users u ON n.user_id = u.id").
		Where(sq.Eq{"u.username": user}).
		Where(sq.Eq{"n.deleted": false}).
		Where(sq.Eq{"n.seen": false}).
		ToSql()
	if err != nil {
		return 0, errors.Wrap(err, wrapMsg)
	}

	// Execute the statement.
	err = tx.QueryRowContext(ctx, statement, args...).Scan(&total)
	if err != nil {
		return 0, errors.Wrap(err, wrapMsg)
	}

	return total, nil
}

// SaveNotification saves a single notification into the database.
func SaveNotification(ctx context.Context, tx *sql.Tx, notification *common.Notification) error {
	wrapMsg := "unable to save notification"

	// Get the notification type ID.
	notificationTypeID, err := RequireNotificationTypeID(ctx, tx, notification.NotificationType)
	if err != nil {
		return errors.Wrap(err, wrapMsg)
	}

	// Get the user ID.
	userID, err := GetOrCreateUserID(ctx, tx, notification.User)
	if err != nil {
		return errors.Wrap(err, wrapMsg)
	}

	// Build the statement to insert the notifications.
	statement, args, err := psql.Insert("notifications").
		Columns(
			"notification_type_id",
			"user_id",
			"subject",
			"seen",
			"deleted",
			"time_created",
			"incoming_json",
			"routing_key").
		Values(
			notificationTypeID,
			userID,
			notification.Subject,
			notification.Seen,
			notification.Deleted,
			notification.TimeCreated,
			notification.Message,
			notification.RoutingKey).
		Suffix("RETURNING id").
		ToSql()
	if err != nil {
		return errors.Wrap(err, wrapMsg)
	}

	// Execute the insert statement, scanning the ID into the notification structure.
	row := tx.QueryRowContext(ctx, statement, args...)
	err = row.Scan(&notification.ID)
	if err != nil {
		return errors.Wrap(err, wrapMsg)
	}

	return nil
}

// SaveOutgoingNotification adds the outgoing notification JSON to the notification in the database.
func SaveOutgoingNotification(ctx context.Context, tx *sql.Tx, outgoingNotification *messaging.NotificationMessage) error {
	wrapMsg := "unable to save outgoing notification JSON"

	// Marshal the outgoing notification message.
	outgoingJSON, err := json.Marshal(outgoingNotification)
	if err != nil {
		return errors.Wrap(err, wrapMsg)
	}

	// The ID identifies the notification to update, so it has to be present and a string.
	id, ok := outgoingNotification.Message["id"].(string)
	if !ok {
		return fmt.Errorf("%s: the outgoing notification has no string ID", wrapMsg)
	}

	// Build the statement to add the notification.
	statement, args, err := psql.Update("notifications").
		Set("outgoing_json", outgoingJSON).
		Where(sq.Eq{"id": id}).
		ToSql()
	if err != nil {
		return errors.Wrap(err, wrapMsg)
	}

	// Execute the update statement and verify that the correct number of rows was affected.
	result, err := tx.ExecContext(ctx, statement, args...)
	if err != nil {
		return errors.Wrap(err, wrapMsg)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return errors.Wrap(err, wrapMsg)
	}
	if rowsAffected != 1 {
		return fmt.Errorf("%s: unexpected number of rows affected: %d", wrapMsg, rowsAffected)
	}

	return nil
}
