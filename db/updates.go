package db

import (
	"database/sql"
	"fmt"

	sq "github.com/Masterminds/squirrel"
	"github.com/pkg/errors"
)

// MarkMessageAsSeen marks a single message as seen in the database if it exists and is targeted to the user with the
// given user ID. The number of messages that were updated is returned.
func MarkMessageAsSeen(tx *sql.Tx, userID string, id string) (int, error) {
	wrapMsg := fmt.Sprintf("unable to mark message %s as seen", id)

	// Build the SQL statement.
	statement, args, err := sq.StatementBuilder.
		PlaceholderFormat(sq.Dollar).
		Update("notifications").
		Set("seen", true).
		Where(sq.Eq{"user_id": userID}).
		Where(sq.Eq{"id": id}).
		ToSql()
	if err != nil {
		return 0, errors.Wrap(err, wrapMsg)
	}

	// Execute the SQL statement.
	result, err := tx.Exec(statement, args...)
	if err != nil {
		return 0, errors.Wrap(err, wrapMsg)
	}

	// Determine how many rows were affected. We require any DBMS that we use to support this.
	count, err := result.RowsAffected()
	if err != nil {
		return 0, errors.Wrap(err, wrapMsg)
	}

	return int(count), nil
}

// DeleteMessage marks a single message as deleted in the database if it exists and is targeted to the user with the
// given user ID. The number of messages that were updated is returned.
func DeleteMessage(tx *sql.Tx, userID string, id string) (int, error) {
	wrapMsg := fmt.Sprintf("unable to makre message %s as seen", id)

	// Build the SQL statement.
	statement, args, err := sq.StatementBuilder.
		PlaceholderFormat(sq.Dollar).
		Update("notifications").
		Set("deleted", true).
		Where(sq.Eq{"user_id": userID}).
		Where(sq.Eq{"id": id}).
		ToSql()
	if err != nil {
		return 0, errors.Wrap(err, wrapMsg)
	}

	// Execute the SQL statement.
	result, err := tx.Exec(statement, args...)
	if err != nil {
		return 0, errors.Wrap(err, wrapMsg)
	}

	// Determine how many rows were affected. We require any DBMS that we use to support this.
	count, err := result.RowsAffected()
	if err != nil {
		return 0, errors.Wrap(err, wrapMsg)
	}

	return int(count), nil
}

// MarkMessagesAsSeen takes a list of UUIDs and marks the corresponding messages as seen in the database if the
// corresponding messages exist and were targeted to the user with the given user ID. A count of the number of
// messages that were eligible to be updated (even if some messages were already marked as seen) is returned.
func MarkMessagesAsSeen(tx *sql.Tx, userID string, uuids []string) (int, error) {
	wrapMsg := "unable to mark messages as seen"

	// Build the SQL statement.
	statement, args, err := sq.StatementBuilder.
		PlaceholderFormat(sq.Dollar).
		Update("notifications").
		Set("seen", true).
		Where(sq.Eq{"user_id": userID}).
		Where(sq.Eq{"id": uuids}).
		ToSql()
	if err != nil {
		return 0, errors.Wrap(err, wrapMsg)
	}

	// Execute the SQL statement
	result, err := tx.Exec(statement, args...)
	if err != nil {
		return 0, errors.Wrap(err, wrapMsg)
	}

	// Determine how many rows were affected. We require any DBMS that we use to support this.
	count, err := result.RowsAffected()
	if err != nil {
		return 0, errors.Wrap(err, wrapMsg)
	}

	return int(count), nil
}

// MarkAllMessagesAsSeen marks all messages in the database that are targeted for the specified user ID as having
// been seen by the user.
func MarkAllMessagesAsSeen(tx *sql.Tx, userID string) (int, error) {
	wrapMsg := fmt.Sprintf("unable to mark all messages for user, %s, as seen", userID)

	// Build the SQL statement.
	statement, args, err := sq.StatementBuilder.
		PlaceholderFormat(sq.Dollar).
		Update("notifications").
		Set("seen", true).
		Where(sq.Eq{"user_id": userID}).
		Where(sq.Eq{"seen": false}).
		ToSql()
	if err != nil {
		return 0, errors.Wrap(err, wrapMsg)
	}

	// Execute the SQL statement.
	result, err := tx.Exec(statement, args...)
	if err != nil {
		return 0, errors.Wrap(err, wrapMsg)
	}

	// Determine how many rows were affected. We require any DBMS that we use to support this.
	count, err := result.RowsAffected()
	if err != nil {
		return 0, errors.Wrap(err, wrapMsg)
	}

	return int(count), nil
}

// DeleteMessages takes a list of UUIDs and deletes the corresponding messages in the database if they exist and were
// targeted to the user with the given user ID. A count of the number of messages that were eligible to be updated
// (even if some messages had already been deleted) is returned.
func DeleteMessages(tx *sql.Tx, userID string, uuids []string) (int, error) {
	wrapMsg := "unable to delete messages"

	// Build the SQL statement.
	statement, args, err := sq.StatementBuilder.
		PlaceholderFormat(sq.Dollar).
		Update("notifications").
		Set("deleted", true).
		Where(sq.Eq{"user_id": userID}).
		Where(sq.Eq{"id": uuids}).
		ToSql()
	if err != nil {
		return 0, errors.Wrap(err, wrapMsg)
	}

	// Execute the SQL statement.
	result, err := tx.Exec(statement, args...)
	if err != nil {
		return 0, errors.Wrap(err, wrapMsg)
	}

	// Determine how many rows were affected. We require any DBMS that we use to support this.
	count, err := result.RowsAffected()
	if err != nil {
		return 0, errors.Wrap(err, wrapMsg)
	}

	return int(count), nil
}

// DeleteMatchingMessagesParameters represents the parameters that may be specified when deleting messages matching a
// set of filters.
type DeleteMatchingMessagesParameters struct {
	Seen               *bool
	NotificationTypeID string
}

// DeleteMatchingMessages deletes matching messages in the database.
func DeleteMatchingMessages(tx *sql.Tx, userID string, params *DeleteMatchingMessagesParameters) (int, error) {
	wrapMsg := "unable to delete matching messages"

	// Begin building the sql statement. Having both `Set("deleted", true)` and `Where(Sq.Eq{"deleted", false})`
	// produced an error saying that the incorrect number of arguments was given for the query. Changing the code
	// adding the WHERE clause to `Where("not deleted")` fixed the problem.
	queryBuilder := sq.StatementBuilder.
		PlaceholderFormat(sq.Dollar).
		Update("notifications").
		Set("deleted", true).
		Where("not deleted").
		Where(sq.Eq{"user_id": userID})

	// Apply the seen parameter if requested.
	if params.Seen != nil {
		queryBuilder = queryBuilder.Where(sq.Eq{"seen": *params.Seen})
	}

	// Apply the notification type parameter if requested.
	if params.NotificationTypeID != "" {
		queryBuilder = queryBuilder.Where(sq.Eq{"notification_type_id": params.NotificationTypeID})
	}

	// Generate the SQL statement.
	statement, args, err := queryBuilder.ToSql()
	if err != nil {
		return 0, errors.Wrap(err, wrapMsg)
	}

	// Execute the SQL statement.
	result, err := tx.Exec(statement, args...)
	if err != nil {
		return 0, errors.Wrap(err, wrapMsg)
	}

	// Determine how many rows were affected. We require any DBMS that we use to support this.
	count, err := result.RowsAffected()
	if err != nil {
		return 0, errors.Wrap(err, wrapMsg)
	}

	return int(count), nil
}
