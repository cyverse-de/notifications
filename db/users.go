package db

import (
	"database/sql"
	"fmt"

	sq "github.com/Masterminds/squirrel"
	"github.com/pkg/errors"
)

// GetUserID returns the ID for a user or the empty string if the user isn't in the database.
func GetUserID(tx *sql.Tx, username string) (string, error) {
	wrapMsg := fmt.Sprintf("unable to look up the username for %s", username)

	// Build the query.
	query, args, err := psql.Select().
		Column("id").
		From("users").
		Where(sq.Eq{"username": username}).
		ToSql()
	if err != nil {
		return "", errors.Wrap(err, wrapMsg)
	}

	// Execute the query.
	rows, err := tx.Query(query, args...)
	if err != nil {
		return "", errors.Wrap(err, wrapMsg)
	}
	defer rows.Close()

	// There should be at most one result; it's not an error if there are no results.
	var userID string
	if rows.Next() {
		err = rows.Scan(&userID)
		if err != nil {
			return "", errors.Wrap(err, wrapMsg)
		}
	}

	return userID, nil
}
