package db

import (
	"database/sql"
	"fmt"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/pkg/errors"
)

// GetNotificationTypeID returns the ID for the notification type with the given ID.
func GetNotificationTypeID(tx *sql.Tx, name string) (string, error) {
	wrapMsg := "unable to get the notification type ID"

	// Build the query.
	query, args, err := psql.Select().
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

// GetNotificationTimestamp returns the timestamp for the notification with the given identifier if the notification
// exists.
func GetNotificationTimestamp(tx *sql.Tx, notificationID string) (*time.Time, error) {
	wrapMsg := fmt.Sprintf("unable to get the timestmp for notification %s", notificationID)

	// Build the query.
	query, args, err := psql.Select().
		Column("time_created").
		From("notifications").
		Where(sq.Eq{"id": notificationID}).
		ToSql()
	if err != nil {
		return nil, errors.Wrap(err, wrapMsg)
	}

	// Query the database.
	rows, err := tx.Query(query, args...)
	if err != nil {
		return nil, errors.Wrap(err, wrapMsg)
	}
	defer rows.Close()

	// There should be at most one result; it's not an error if there are no results.
	if rows.Next() {
		var timestamp time.Time
		err := rows.Scan(&timestamp)
		if err != nil {
			return nil, errors.Wrap(err, wrapMsg)
		}
		return &timestamp, nil
	}

	// If we get here then there was no matching notification.
	return nil, nil
}

// FilterMissingIDs returns the IDs in the given ID list that refer to notifications that either don't exist or were
// not directed to the user with the given user ID.
func FilterMissingIDs(tx *sql.Tx, userID string, ids []string) ([]string, error) {
	wrapMsg := "error encountered while verifying notification IDs"

	// Build the query.
	query, args, err := psql.Select().
		From("notifications").
		Column("id").
		Where(sq.Eq{"id": ids}).
		Where(sq.Eq{"user_id": userID}).
		ToSql()
	if err != nil {
		return nil, errors.Wrap(err, wrapMsg)
	}

	// Query the database.
	rows, err := tx.Query(query, args...)
	if err != nil {
		return nil, errors.Wrap(err, wrapMsg)
	}
	defer rows.Close()

	// Load the list of extant notification IDs.
	extantIDs := make([]string, 0)
	for rows.Next() {
		var id string
		err := rows.Scan(&id)
		if err != nil {
			return nil, errors.Wrap(err, wrapMsg)
		}
		extantIDs = append(extantIDs, id)
	}

	// Create a map of extant IDs and use it as a set.
	extantIDSet := make(map[string]bool)
	for _, id := range extantIDs {
		extantIDSet[id] = true
	}

	// Build a map of missing IDs.
	missingIDSet := make(map[string]bool)
	for _, id := range ids {
		if !extantIDSet[id] {
			missingIDSet[id] = true
		}
	}

	// Extract the missing IDs into a slice.
	missingIDs := make([]string, 0)
	for id := range missingIDSet {
		missingIDs = append(missingIDs, id)
	}

	return missingIDs, nil
}
