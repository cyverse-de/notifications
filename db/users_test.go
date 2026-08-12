package db

import (
	"context"
	"database/sql"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
)

func TestGetOrCreateUserIDAddsUnknownUsers(t *testing.T) {
	assert := assert.New(t)

	db, mock, err := sqlmock.New()
	ctx := context.Background()
	assert.NoError(err, "unable to open the mock database connection")
	defer func() { _ = db.Close() }()

	// The insert has to tolerate a conflict; two deliveries for a user who isn't in the database
	// yet are recorded concurrently.
	testID := "e26b7f58-8f6e-4b5f-9b23-cfe6f2bbd7c1"
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT id FROM users WHERE username =").
		WithArgs("sarahr").
		WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery(
		`INSERT INTO users \(username\) VALUES \(\$1\) ` +
			`ON CONFLICT \(username\) DO UPDATE SET username = EXCLUDED.username RETURNING id`,
	).
		WithArgs("sarahr").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(testID))
	mock.ExpectRollback()

	tx, err := db.Begin()
	assert.NoError(err, "unable to begin a transaction")
	id, err := GetOrCreateUserID(ctx, tx, "sarahr")
	assert.NoError(err, "unexpected error occurred while looking up the user ID")
	assert.Equal(testID, id)
	_ = tx.Rollback()

	assert.NoError(mock.ExpectationsWereMet(), "not all mock expectations were met")
}
