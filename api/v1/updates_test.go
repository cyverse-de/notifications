package v1

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/cyverse-de/notifications/common"
	"github.com/go-playground/validator/v10"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
)

// testValidator mirrors the validator that main.go installs on the Echo instance, which the
// handlers under test rely on to reject malformed request bodies.
type testValidator struct {
	validator *validator.Validate
}

func (v testValidator) Validate(i interface{}) error {
	return v.validator.Struct(i)
}

// TestUpdateHandlersQualifyUsernames pins the qualification of every username the update handlers
// resolve to a user ID, regardless of whether the handler takes it from the query string or from
// the request body. An unqualified lookup misses the user row that the recorder writes, so the
// handler reports success after updating nothing.
func TestUpdateHandlersQualifyUsernames(t *testing.T) {
	const userID = "e26b7f58-8f6e-4b5f-9b23-cfe6f2bbd7c1"
	const notificationID = "1e1e8a24-fbbc-4dd1-a5a4-e0dcb4a4b5c9"

	tests := []struct {
		name    string
		handler func(*API) func(echo.Context) error
		target  string
		body    string
		expect  func(sqlmock.Sqlmock)
	}{
		{
			name:    "mark all messages as seen takes the username from the body",
			handler: func(a *API) func(echo.Context) error { return a.MarkAllMessagesAsSeen },
			target:  "/v1/mark-all-seen",
			body:    `{"user":"sarahr"}`,
			expect: func(mock sqlmock.Sqlmock) {
				mock.ExpectExec("UPDATE notifications SET seen").
					WithArgs(true, userID, false).
					WillReturnResult(sqlmock.NewResult(0, 3))
			},
		},
		{
			name:    "mark messages as seen takes the username from the query string",
			handler: func(a *API) func(echo.Context) error { return a.MarkMessagesAsSeen },
			target:  "/v1/seen?user=sarahr",
			body:    `{"uuids":["` + notificationID + `"]}`,
			expect: func(mock sqlmock.Sqlmock) {
				mock.ExpectExec("UPDATE notifications SET seen").
					WithArgs(true, userID, notificationID).
					WillReturnResult(sqlmock.NewResult(0, 1))
			},
		},
		{
			name:    "delete messages takes the username from the query string",
			handler: func(a *API) func(echo.Context) error { return a.DeleteMessages },
			target:  "/v1/delete?user=sarahr",
			body:    `{"uuids":["` + notificationID + `"]}`,
			expect: func(mock sqlmock.Sqlmock) {
				mock.ExpectExec("UPDATE notifications SET deleted").
					WithArgs(true, userID, notificationID).
					WillReturnResult(sqlmock.NewResult(0, 1))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert := assert.New(t)

			db, mock, err := sqlmock.New()
			assert.NoError(err, "unable to open the mock database connection")
			defer func() { _ = db.Close() }()

			mock.ExpectBegin()
			mock.ExpectQuery("SELECT id FROM users WHERE username =").
				WithArgs("sarahr@example.org").
				WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(userID))
			tt.expect(mock)
			mock.ExpectCommit()

			e := echo.New()
			e.Validator = testValidator{validator: validator.New()}
			a := &API{Echo: e, DB: db, UserSuffix: common.NewUserSuffix("example.org")}

			req := httptest.NewRequest(http.MethodPost, tt.target, strings.NewReader(tt.body))
			req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
			rec := httptest.NewRecorder()

			assert.NoError(tt.handler(a)(e.NewContext(req, rec)))
			assert.Equal(http.StatusOK, rec.Code)
			assert.NoError(mock.ExpectationsWereMet(), "not all mock expectations were met")
		})
	}
}
