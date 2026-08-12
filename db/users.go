package db

import (
	"context"
	"database/sql"
	"fmt"

	sq "github.com/Masterminds/squirrel"
	"github.com/pkg/errors"
)

<<<<<<< Updated upstream
=======
// AddUser adds a user to the `users` table in the notifications database, returning the ID
// assigned to the user. Adding a user who is already in the table returns that user's existing ID.
func AddUser(ctx context.Context, tx *sql.Tx, user string) (string, error) {
	wrapMsg := fmt.Sprintf("unable to add `%s` to the users table", user)

	// The upsert keeps concurrent recordings of a user's first notification from colliding on
	// users_username_key. DO UPDATE rather than DO NOTHING so that the ID is returned either way.
	statement, args, err := psql.Insert("users").
		Columns("username").
		Values(user).
		Suffix("ON CONFLICT (username) DO UPDATE SET username = EXCLUDED.username RETURNING id").
		ToSql()
	if err != nil {
		return "", errors.Wrap(err, wrapMsg)
	}

	// Execute the statement.
	var id string
	row := tx.QueryRowContext(ctx, statement, args...)
	err = row.Scan(&id)
	if err != nil {
		return "", errors.Wrap(err, wrapMsg)
	}

	return id, nil
}

// GetOrCreateUserID obtains the user ID for `user`, adding the user to the `users` table if
// necessary. This differs from GetUserID, which reports an unknown user as an empty string
// rather than creating one; callers storing the result as a foreign key need this variant.
func GetOrCreateUserID(ctx context.Context, tx *sql.Tx, user string) (string, error) {
	wrapMsg := fmt.Sprintf("unable to get the user ID for `%s`", user)

	// Build the query.
	statement, args, err := psql.Select().
		Column("id").
		From("users").
		Where(sq.Eq{"username": user}).
		ToSql()
	if err != nil {
		return "", errors.Wrap(err, wrapMsg)
	}

	// Query the database.
	var id string
	row := tx.QueryRowContext(ctx, statement, args...)
	err = row.Scan(&id)
	if err == nil {
		return id, nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return AddUser(ctx, tx, user)
	}

	return "", errors.Wrap(err, wrapMsg)
}

>>>>>>> Stashed changes
// GetUserID returns the ID for a user or the empty string if the user isn't in the database.
func GetUserID(ctx context.Context, tx *sql.Tx, username string) (string, error) {
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
	rows, err := tx.QueryContext(ctx, query, args...)
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
