package db

import (
	"database/sql"
	"fmt"

	sq "github.com/Masterminds/squirrel"
	"github.com/pkg/errors"
)

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
