package db

import (
	"database/sql"

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

	// Execute the SQL Statement
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
