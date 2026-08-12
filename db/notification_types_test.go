package db

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
)

func TestRequireNotificationTypeID(t *testing.T) {
	assert := assert.New(t)

	db, mock, err := sqlmock.New()
	ctx := context.Background()
	assert.NoError(err, "unable to open the mock database connection")
	defer func() { _ = db.Close() }()

	// Set up the expectations.
	mock.ExpectBegin()
	testID := "a6a97fd2-74c5-42af-ab22-0549a63d3abd"
	rows := sqlmock.NewRows([]string{"id"}).AddRow(testID)
	mock.ExpectQuery("SELECT id::text FROM notification_types WHERE name =").
		WithArgs("test").
		WillReturnRows(rows)
	mock.ExpectRollback()

	// Look up a notification type.
	tx, err := db.Begin()
	assert.NoError(err, "unable to begin a transaction")
	id, err := RequireNotificationTypeID(ctx, tx, "test")
	assert.NoError(err, "unexpected error occurred while looking up the notification type ID")
	assert.Equal(testID, id)
	_ = tx.Rollback()

	// Verify that all mock expectations were met.
	err = mock.ExpectationsWereMet()
	assert.NoError(err, "not all mock expectations were met")
}

func TestRegisterNotificationType(t *testing.T) {
	assert := assert.New(t)

	db, mock, err := sqlmock.New()
	ctx := context.Background()
	assert.NoError(err, "unable to open the mock database connection")
	defer func() { _ = db.Close() }()

	// Set up the expectations.
	mock.ExpectBegin()
	testType := "test_notification_type"
	mock.ExpectExec("INSERT INTO notification_types \\(name\\)").
		WithArgs(testType).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectRollback()

	// Register the notification type.
	tx, err := db.Begin()
	assert.NoError(err, "unable to begin a transaction")
	err = RegisterNotificationType(ctx, tx, testType)
	assert.NoError(err, "unexpected error occurred while registering the notification type")
	_ = tx.Rollback()

	// Verify that all mock expectations were met.
	err = mock.ExpectationsWereMet()
	assert.NoError(err, "not all mock expectaions were met")
}
