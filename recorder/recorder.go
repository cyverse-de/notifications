package recorder

import (
	"context"
	"encoding/json"
	"maps"
	"strings"
	"time"

	"github.com/cyverse-de/messaging/v12"
	"github.com/cyverse-de/notifications/common"
	"github.com/sirupsen/logrus"
)

// log derives from the standard logrus logger, so the formatting and level that main sets up
// apply here too.
var log = logrus.WithFields(logrus.Fields{"package": "recorder"})

// Request represents a deserialized incoming notification request.
type Request struct {
	RequestType   string                 `json:"type"`
	User          string                 `json:"user"`
	Subject       string                 `json:"subject"`
	Timestamp     string                 `json:"timestamp"`
	Email         bool                   `json:"email"`
	EmailTemplate string                 `json:"email_template"`
	Payload       map[string]interface{} `json:"payload"`
	Message       string                 `json:"message"`
}

// Recorder records incoming notification requests and publishes the outgoing messages.
type Recorder struct {
	dbc             DatabaseClient
	messagingClient MessagingClient
}

// New returns a new recorder.
func New(dbc DatabaseClient, messagingClient MessagingClient) *Recorder {
	return &Recorder{
		dbc:             dbc,
		messagingClient: messagingClient,
	}
}

// buildEmailRequest validates the email portion of a notification request and builds the
// outgoing email request. Validation happens before the notification is committed so that a
// bad address discards the delivery rather than leaving a committed row behind.
func (r *Recorder) buildEmailRequest(request *Request) (*messaging.EmailRequest, error) {
	wrapMsg := "unable to build the email request"

	// Extract the email address from the notification request payload.
	var emailAddress string
	switch str := request.Payload["email_address"].(type) {
	case string:
		emailAddress = str
	default:
		return nil, NewUnrecoverableError("%s: %s", wrapMsg, "no email address provided or invalid data type in request")
	}

	// Validate the email address.
	if err := common.ValidateEmailAddress(emailAddress); err != nil {
		return nil, NewUnrecoverableError("%s: %s", wrapMsg, err.Error())
	}

	// Validate the template name.
	if request.EmailTemplate == "" {
		return nil, NewUnrecoverableError("%s: %s", wrapMsg, "no email template provided")
	}

	// The payload is copied because buildNotificationMessage rewrites the timestamps in it, and the
	// email templates expect the timestamps exactly as they arrived.
	return &messaging.EmailRequest{
		Subject:        request.Subject,
		ToAddress:      emailAddress,
		TemplateName:   request.EmailTemplate,
		TemplateValues: maps.Clone(request.Payload),
	}, nil
}

// buildNotificationMessage formats the outgoing notification message destined
// for the Discovery Environment UI. This function changes the message payload,
// so it should only be called after an exact copy of the incoming message body
// is no longer needed.
func (r *Recorder) buildNotificationMessage(
	request *common.Notification,
	payload *Request,
) (*messaging.NotificationMessage, error) {
	wrapMsg := "unable to build notification message"

	// Determine the primary text of the message portion of the notification.
	messageText := payload.Message
	if messageText == "" {
		messageText = payload.Subject
	}

	// The message portion of the request sent to the UI is a JSON object.
	outgoingMessage := map[string]interface{}{
		"id":        request.ID,
		"timestamp": common.FormatTimestamp(request.TimeCreated),
		"text":      messageText,
	}

	// Ensure that the analysis start date is in the correct format if it's present. A timestamp
	// that can't be converted is a defect in the payload, so retrying would fail the same way.
	err := common.FixTimestampInMap(payload.Payload, "startdate")
	if err != nil {
		return nil, NewUnrecoverableError("%s: %s", wrapMsg, err.Error())
	}

	// Ensure that the analysis end date is in the correct format if it's present.
	err = common.FixTimestampInMap(payload.Payload, "enddate")
	if err != nil {
		return nil, NewUnrecoverableError("%s: %s", wrapMsg, err.Error())
	}

	// Replace underscores with spaces in the notification type.
	payload.RequestType = strings.ReplaceAll(payload.RequestType, "_", " ")

	// Build the notification message.
	notificationMessage := &messaging.NotificationMessage{
		Deleted:       request.Deleted,
		Email:         payload.Email,
		EmailTemplate: payload.EmailTemplate,
		Message:       outgoingMessage,
		Payload:       payload.Payload,
		Seen:          request.Seen,
		Subject:       request.Subject,
		Type:          strings.ReplaceAll(request.NotificationType, "_", " "),
		User:          request.User,
	}

	return notificationMessage, nil
}

// Record stores an incoming notification request and publishes the outgoing email and UI
// messages. The body and routing key are passed in rather than an AMQP delivery so that the
// recording logic stays independent of the messaging library.
func (r *Recorder) Record(ctx context.Context, updateType string, body []byte, routingKey string) error {
	updateType = strings.ToLower(updateType)

	// Parse the message body.
	var request Request
	if err := json.Unmarshal(body, &request); err != nil {
		return NewUnrecoverableError("unable to parse message body: %s", err.Error())
	}

	// Parse the timestamp.
	timeCreated, err := time.Parse(time.RFC3339Nano, request.Timestamp)
	if err != nil {
		return NewUnrecoverableError("unable to parse timestamp: %s", err.Error())
	}

	// Validate the email request before anything is committed, so that a bad address discards
	// the delivery instead of leaving a recorded notification behind.
	var emailRequest *messaging.EmailRequest
	if request.Email {
		emailRequest, err = r.buildEmailRequest(&request)
		if err != nil {
			return err
		}
	}

	// Begin a database transaction.
	tx, err := r.dbc.Begin()
	if err != nil {
		return NewRecoverableError("unable to begin a database transaction: %s", err.Error())
	}
	committed := false
	defer func() {
		if !committed {
			if rollbackErr := r.dbc.Rollback(tx); rollbackErr != nil {
				log.Errorf("unable to roll back the database transaction: %s", rollbackErr.Error())
			}
		}
	}()

	// Register the notification type in case it doesn't exist in the database yet.
	if err = r.dbc.RegisterNotificationType(ctx, tx, updateType); err != nil {
		return classifyDatabaseError(err, "unable to register the notification type")
	}

	// Store the message in the database.
	storableRequest := &common.Notification{
		NotificationType: updateType,
		User:             request.User,
		Subject:          request.Subject,
		Seen:             false,
		Deleted:          false,
		TimeCreated:      timeCreated,
		Message:          string(body),
		RoutingKey:       routingKey,
	}
	if err = r.dbc.SaveNotification(ctx, tx, storableRequest); err != nil {
		return classifyDatabaseError(err, "unable to save the notification")
	}

	// Build the notification message.
	notificationMessage, err := r.buildNotificationMessage(storableRequest, &request)
	if err != nil {
		return err
	}

	// Save the outgoing notification in the database.
	if err = r.dbc.SaveOutgoingNotification(ctx, tx, notificationMessage); err != nil {
		return classifyDatabaseError(err, "unable to save the outgoing notification")
	}

	// Count the number of unread notifications.
	unreadNotificationCount, err := r.dbc.CountUnreadNotifications(ctx, tx, request.User)
	if err != nil {
		return classifyDatabaseError(err, "unable to count the unread notifications")
	}

	// Add the wrapper around the notification message.
	wrappedNotificationMessage := &messaging.WrappedNotificationMessage{
		Message: notificationMessage,
		Total:   unreadNotificationCount,
	}

	// Commit the transaction.
	if err = r.dbc.Commit(tx); err != nil {
		return NewRecoverableError("unable to commit the database transaction: %s", err.Error())
	}
	committed = true

	// Publishing after the commit means a publish failure cannot requeue the delivery and write
	// a duplicate notification. The cost is a recorded notification the user is never pinged
	// about, which is logged rather than retried.
	if emailRequest != nil {
		if err := r.messagingClient.PublishEmailRequestContext(ctx, emailRequest); err != nil {
			log.Errorf(
				"notification %s was recorded but its email request could not be published; "+
					"the AMQP exchange is probably unreachable: %s",
				storableRequest.ID, err.Error(),
			)
		}
	}
	if err := r.messagingClient.PublishNotificationMessageContext(ctx, wrappedNotificationMessage); err != nil {
		log.Errorf(
			"notification %s was recorded but could not be published to the UI; "+
				"the AMQP exchange is probably unreachable: %s",
			storableRequest.ID, err.Error(),
		)
	}

	return nil
}
