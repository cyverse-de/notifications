package db

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/pkg/errors"

	sq "github.com/Masterminds/squirrel"
)

// RequireNotificationTypeID obtains the ID of the notification type with the given name. An error
// is returned if the database can't be queried or the notification type doesn't exist.
func RequireNotificationTypeID(ctx context.Context, tx *sql.Tx, notificationType string) (string, error) {
	wrapMsg := fmt.Sprintf("unable to get the notification type ID for `%s`", notificationType)

	// Build the SQL query and arguments.
	query, args, err := psql.Select("id::text").
		From("notification_types").
		Where(sq.Eq{"name": notificationType}).
		ToSql()
	if err != nil {
		return "", errors.Wrap(err, wrapMsg)
	}

	// Query the database.
	var id string
	row := tx.QueryRowContext(ctx, query, args...)
	err = row.Scan(&id)
	if err != nil {
		return "", errors.Wrap(err, wrapMsg)
	}

	return id, nil
}

// RegisterNotificationType registers a new notification type.
func RegisterNotificationType(ctx context.Context, tx *sql.Tx, notificationType string) error {
	wrapMsg := fmt.Sprintf("unable to register the notification type, `%s`", notificationType)

	// Build the SQL statement and arguments.
	query, args, err := psql.Insert("notification_types").
		Columns("name").
		Values(notificationType).
		Suffix("ON CONFLICT DO NOTHING").
		ToSql()
	if err != nil {
		return errors.Wrap(err, wrapMsg)
	}

	// Query the database.
	_, err = tx.ExecContext(ctx, query, args...)
	if err != nil {
		return errors.Wrap(err, wrapMsg)
	}

	return nil
}
