package recorder

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"regexp"
	"testing"

	"github.com/cyverse-de/messaging/v12"
	"github.com/cyverse-de/notifications/common"
	"github.com/stretchr/testify/assert"
)

// MockMessagingClient provides mock implementations of the functions we need from messaging.Client.
type MockMessagingClient struct {
	PublishedNotificationMessage *messaging.WrappedNotificationMessage
	PublishedEmailRequest        *messaging.EmailRequest
}

// PublishNotificationMessageContext stores a copy of the notification message for later inspection.
func (c *MockMessagingClient) PublishNotificationMessageContext(_ context.Context, msg *messaging.WrappedNotificationMessage) error {
	c.PublishedNotificationMessage = msg
	return nil
}

// PublishEmailRequestContext stores a copy of the email request for later inspection.
func (c *MockMessagingClient) PublishEmailRequestContext(_ context.Context, req *messaging.EmailRequest) error {
	c.PublishedEmailRequest = req
	return nil
}

// NewMockMessagingClient creates a new mock messaging client for testing.
func NewMockMessagingClient() *MockMessagingClient {
	return &MockMessagingClient{}
}

// FakeNotificationID is the identifier assigned to notifications by the mock database client.
const FakeNotificationID = "46ae63be-7030-4cdd-8eb9-66aa49fcf38b"

// FakeRoutingKey is the routing key used for all deliveries in these tests.
const FakeRoutingKey = "events.notification.update.foo"

// MockDatabaseClient provides mock implementations of the database functions the recorder calls.
type MockDatabaseClient struct {
	BeginCalled                bool
	CommitCalled               bool
	RollbackCalled             bool
	RegisteredNotificationType string
	SavedNotification          *common.Notification
	savedOutgoingMessage       *messaging.NotificationMessage
	unreadMessageCount         int64

	// CommitErr, when set, makes Commit fail so post-commit behavior can be tested.
	CommitErr error
}

// Begin records the fact that it was called.
func (c *MockDatabaseClient) Begin() (*sql.Tx, error) {
	c.BeginCalled = true
	return nil, nil
}

// Commit records the fact that it was called and reports CommitErr.
func (c *MockDatabaseClient) Commit(*sql.Tx) error {
	c.CommitCalled = true
	return c.CommitErr
}

// Rollback records the fact that it was called.
func (c *MockDatabaseClient) Rollback(*sql.Tx) error {
	c.RollbackCalled = true
	return nil
}

// RegisterNotificationType records a notification type that has been registered.
func (c *MockDatabaseClient) RegisterNotificationType(_ context.Context, _ *sql.Tx, notificationType string) error {
	c.RegisteredNotificationType = notificationType
	return nil
}

// SaveNotification records a copy of the notification that was saved.
func (c *MockDatabaseClient) SaveNotification(_ context.Context, _ *sql.Tx, notification *common.Notification) error {
	notification.ID = FakeNotificationID
	c.SavedNotification = notification
	return nil
}

// SaveOutgoingNotification records a copy of the notification message that was saved.
func (c *MockDatabaseClient) SaveOutgoingNotification(
	_ context.Context,
	_ *sql.Tx,
	outgoingNotification *messaging.NotificationMessage,
) error {
	c.savedOutgoingMessage = outgoingNotification
	return nil
}

// CountUnreadNotifications returns the canned unread count.
func (c *MockDatabaseClient) CountUnreadNotifications(_ context.Context, _ *sql.Tx, _ string) (int64, error) {
	return c.unreadMessageCount, nil
}

// NewMockDatabaseClient creates a new mock database client for testing.
func NewMockDatabaseClient(unreadMessageCount int64) *MockDatabaseClient {
	return &MockDatabaseClient{unreadMessageCount: unreadMessageCount}
}

// getNotificationRequest returns a map that can be used as an incoming notification request.
func getNotificationRequest() map[string]any {
	return map[string]any{
		"type":      "analysis",
		"user":      "sarahr",
		"subject":   "some job status changed",
		"message":   "This is a test message",
		"timestamp": "2020-07-07T17:59:59-07:00",
		"payload": map[string]any{
			"action":                "job_status_change",
			"analysisname":          "some job",
			"analysisdescription":   "some job description",
			"analysisstatus":        "Completed",
			"analysisstartdate":     "2020-07-07T17:59:59-07:00",
			"analysisresultsfolder": "/iplant/home/foo/analyses",
			"description":           "some job description",
			"email_address":         "sarahr@cyverse.org",
			"name":                  "some job",
			"resultfolderid":        "/iplant/home/foo/analyses",
			"startdate":             "2020-07-07T17:59:59-07:00",
			"status":                "Completed",
			"user":                  "sarahr",
		},
		"email_template": "analysis_status_change",
		"email":          true,
	}
}

// marshalRequest applies the given mutations to a base request and serializes it.
func marshalRequest(t *testing.T, mutate func(map[string]any)) []byte {
	t.Helper()
	req := getNotificationRequest()
	if mutate != nil {
		mutate(req)
	}
	body, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("unable to marshal the notification request: %s", err.Error())
	}
	return body
}

// isEpochMillis returns true if a timestamp has been converted to milliseconds since the epoch.
func isEpochMillis(timestamp string) bool {
	return regexp.MustCompile(`^\d+$`).MatchString(timestamp)
}

func TestRecord(t *testing.T) {
	tests := []struct {
		name        string
		mutate      func(map[string]any)
		updateType  string
		wantType    string
		wantEmail   bool
		wantMsgText string
		wantUnread  int64
	}{
		{
			name:        "a full request is recorded and published",
			updateType:  "analysis",
			wantType:    "analysis",
			wantEmail:   true,
			wantMsgText: "This is a test message",
			wantUnread:  42,
		},
		{
			name:        "no email is published when none was requested",
			mutate:      func(m map[string]any) { m["email"] = false },
			updateType:  "analysis",
			wantType:    "analysis",
			wantEmail:   false,
			wantMsgText: "This is a test message",
			wantUnread:  42,
		},
		{
			name:        "an empty message falls back to the subject",
			mutate:      func(m map[string]any) { m["message"] = "" },
			updateType:  "analysis",
			wantType:    "analysis",
			wantEmail:   true,
			wantMsgText: "some job status changed",
			wantUnread:  42,
		},
		{
			name:        "an upper case update type is lower cased",
			updateType:  "ANALYSIS",
			wantType:    "analysis",
			wantEmail:   true,
			wantMsgText: "This is a test message",
			wantUnread:  42,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert := assert.New(t)

			body := marshalRequest(t, tt.mutate)
			databaseClient := NewMockDatabaseClient(tt.wantUnread)
			messagingClient := NewMockMessagingClient()
			r := New(databaseClient, messagingClient)

			err := r.Record(context.Background(), tt.updateType, body, FakeRoutingKey)
			assert.NoError(err)

			// The transaction was started and committed.
			assert.True(databaseClient.BeginCalled, "no database transaction was started")
			assert.True(databaseClient.CommitCalled, "the database transaction was not committed")

			// The notification type was registered, lower cased.
			assert.Equal(tt.wantType, databaseClient.RegisteredNotificationType)

			// The notification was saved with the expected fields.
			saved := databaseClient.SavedNotification
			if saved == nil {
				t.Fatal("no notification was saved")
			}
			assert.Equal(tt.wantType, saved.NotificationType, "incorrect notification type")
			assert.Equal("sarahr", saved.User, "incorrect user")
			assert.Equal(FakeRoutingKey, saved.RoutingKey, "incorrect routing key")
			assert.JSONEq(string(body), saved.Message, "the raw request body must be stored verbatim")

			// The outgoing notification was saved.
			outgoing := databaseClient.savedOutgoingMessage
			if outgoing == nil {
				t.Fatal("the outbound notification message was not recorded in the database")
			}
			assert.Equal(FakeNotificationID, outgoing.Message["id"], "incorrect ID")
			assert.Equal(tt.wantType, outgoing.Type, "incorrect notification type")
			assert.Truef(
				isEpochMillis(outgoing.Message["timestamp"].(string)),
				"incorrect timestamp format: %s", outgoing.Message["timestamp"],
			)

			// The email request was published only when one was requested.
			emailRequest := messagingClient.PublishedEmailRequest
			if tt.wantEmail {
				if emailRequest == nil {
					t.Fatal("no email request was published")
				}
				assert.Equal("some job status changed", emailRequest.Subject, "incorrect email subject")
				assert.Equal("sarahr@cyverse.org", emailRequest.ToAddress, "incorrect email address")
			} else {
				assert.Nil(emailRequest, "an email request was published when none was expected")
			}

			// The UI notification was published.
			notification := messagingClient.PublishedNotificationMessage
			if notification == nil {
				t.Fatal("no notification was published")
			}
			assert.Equal(FakeNotificationID, notification.Message.Message["id"], "incorrect ID")
			assert.Equal(tt.wantUnread, notification.Total, "incorrect unread total")
			assert.Equal(tt.wantMsgText, notification.Message.Message["text"], "incorrect message text")
			assert.Equal(tt.wantType, notification.Message.Type, "incorrect notification type")

			// Timestamps in the payload were converted, and no enddate was invented.
			payload, ok := notification.Message.Payload.(map[string]any)
			if !ok {
				t.Fatal("payload doesn't appear to be a map")
			}
			assert.Truef(
				isEpochMillis(payload["startdate"].(string)),
				"incorrect timestamp format: %s", payload["startdate"],
			)
			_, ok = payload["enddate"]
			assert.False(ok, "enddate was found in the payload when it wasn't expected")
		})
	}
}

func TestRecordRejectsBadInput(t *testing.T) {
	tests := []struct {
		name   string
		body   []byte
		mutate func(map[string]any)
	}{
		{
			name: "an unparseable body is rejected",
			body: []byte("{not json"),
		},
		{
			name:   "an unparseable timestamp is rejected",
			mutate: func(m map[string]any) { m["timestamp"] = "not a timestamp" },
		},
		{
			name: "a missing email address is rejected when email was requested",
			mutate: func(m map[string]any) {
				delete(m["payload"].(map[string]any), "email_address")
			},
		},
		{
			name: "an invalid email address is rejected",
			mutate: func(m map[string]any) {
				m["payload"].(map[string]any)["email_address"] = "not-an-address"
			},
		},
		{
			name:   "a missing email template is rejected when email was requested",
			mutate: func(m map[string]any) { m["email_template"] = "" },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert := assert.New(t)

			body := tt.body
			if body == nil {
				body = marshalRequest(t, tt.mutate)
			}

			databaseClient := NewMockDatabaseClient(42)
			messagingClient := NewMockMessagingClient()
			r := New(databaseClient, messagingClient)

			err := r.Record(context.Background(), "analysis", body, FakeRoutingKey)

			var unrecoverable UnrecoverableError
			assert.ErrorAs(err, &unrecoverable, "bad input must be an unrecoverable error so the delivery is discarded")
			assert.False(databaseClient.CommitCalled, "nothing may be committed for a rejected request")
			assert.Nil(messagingClient.PublishedEmailRequest, "no email may be published for a rejected request")
			assert.Nil(messagingClient.PublishedNotificationMessage, "no notification may be published for a rejected request")
		})
	}
}

func TestPublishesHappenAfterCommit(t *testing.T) {
	assert := assert.New(t)

	databaseClient := NewMockDatabaseClient(42)
	databaseClient.CommitErr = errors.New("commit failed")
	messagingClient := NewMockMessagingClient()
	r := New(databaseClient, messagingClient)

	err := r.Record(context.Background(), "analysis", marshalRequest(t, nil), FakeRoutingKey)

	assert.Error(err, "a failed commit must be reported")
	assert.Nil(messagingClient.PublishedEmailRequest,
		"no email may be published when the transaction did not commit")
	assert.Nil(messagingClient.PublishedNotificationMessage,
		"no UI notification may be published when the transaction did not commit")

	var recoverable RecoverableError
	assert.ErrorAs(err, &recoverable, "a failed commit is recoverable so the delivery is requeued")
}

func TestEmailPayloadIsNotRewritten(t *testing.T) {
	assert := assert.New(t)

	messagingClient := NewMockMessagingClient()
	r := New(NewMockDatabaseClient(42), messagingClient)

	err := r.Record(context.Background(), "analysis", marshalRequest(t, nil), FakeRoutingKey)
	assert.NoError(err)

	emailRequest := messagingClient.PublishedEmailRequest
	if emailRequest == nil {
		t.Fatal("no email request was published")
	}
	assert.Equal("2020-07-07T17:59:59-07:00", emailRequest.TemplateValues["startdate"],
		"the email templates expect the timestamps exactly as they arrived")
}

func TestOutgoingJSONShapeIsUnchanged(t *testing.T) {
	databaseClient := NewMockDatabaseClient(42)
	r := New(databaseClient, NewMockMessagingClient())

	if err := r.Record(context.Background(), "analysis", marshalRequest(t, nil), FakeRoutingKey); err != nil {
		t.Fatalf("unexpected error: %s", err.Error())
	}

	actual, err := json.MarshalIndent(databaseClient.savedOutgoingMessage, "", "  ")
	if err != nil {
		t.Fatalf("unable to marshal the outgoing message: %s", err.Error())
	}

	expected, err := os.ReadFile("testdata/outgoing_json_golden.json")
	if err != nil {
		t.Fatalf("unable to read the golden file: %s", err.Error())
	}

	assert.JSONEq(t, string(expected), string(actual),
		"outgoing_json is a wire contract read by db/listings.go; its shape must not change")
}
