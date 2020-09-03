package db

import (
	"database/sql"

	sq "github.com/Masterminds/squirrel"
	"github.com/pkg/errors"
)

// GetNotificationTypeID returns the ID for the notification type with the given ID.
func GetNotificationTypeID(tx *sql.Tx, name string) (string, error) {
	wrapMsg := "unable to get the notification type ID"

	// Build the query.
	query, args, err := sq.StatementBuilder.
		PlaceholderFormat(sq.Dollar).
		Select().
		Column("id").
		From("notification_types").
		Where(sq.Eq{"name": name}).
		ToSql()
	if err != nil {
		return "", errors.Wrap(err, wrapMsg)
	}

	// Query the database.
	rows, err := tx.Query(query, args...)
	if err != nil {
		return "", errors.Wrap(err, wrapMsg)
	}
	defer rows.Close()

	// There should be at most one result; it's not an error if there are no results.
	var notificationTypeID string
	if rows.Next() {
		err := rows.Scan(&notificationTypeID)
		if err != nil {
			return "", errors.Wrap(err, wrapMsg)
		}
	}

	return notificationTypeID, nil
}
