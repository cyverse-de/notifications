package v1

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/cyverse-de/messaging/v9"
	"github.com/cyverse-de/notifications/common"
	"github.com/cyverse-de/notifications/model"
	"github.com/labstack/echo/v4"
	"github.com/pkg/errors"
)

// fixTimestamp fixes a timestamp stored as a string in a map.
func fixTimestamp(m map[string]interface{}, k string) error {
	wrapMsg := fmt.Sprintf("unable to fix the timestamp in key '%s'", k)

	// Extract the current value.
	v, present := m[k]
	if !present {
		return nil
	}

	// Convert the value to a string. We only have to check types used by the json package.
	var stringValue string
	switch val := v.(type) {
	case string:
		stringValue = val
	case float64:
		stringValue = fmt.Sprintf("%d", int64(val))
	default:
		return fmt.Errorf("%s: %s", wrapMsg, "invalid data type")
	}

	// Convert the timestamp to milliseconds since the epoch.
	convertedValue, err := common.FixTimestamp(stringValue)
	if err != nil {
		return errors.Wrap(err, wrapMsg)
	}

	// Directly update the value in the map.
	m[k] = convertedValue

	return nil
}

// validateEmailRequest verifies that we have the required fields if an email was requested.
func validateEmailRequest(request *model.V1NotificationRequest) error {

	// Verify that an email template was provided.
	if common.IsBlank(request.EmailTemplate) {
		return fmt.Errorf("an email was requested, but no email template was specified")
	}

	// Verify that a valid email address was provided.
	v, ok := request.Payload["email_address"]
	if !ok || v == nil {
		return fmt.Errorf("an email was requested, but no email address was provided")
	}
	addr, ok := v.(string)
	if !ok {
		return fmt.Errorf("an email was requested, but the type of the email address was invalid")
	}
	if common.IsBlank(addr) {
		return fmt.Errorf("an email was requested, but the email address was blank")
	}
	if err := common.ValidateEmailAddress(addr); err != nil {
		return fmt.Errorf("an email was requsted, but an invalid email address was specified: %s", err.Error())
	}

	return nil
}

// OutboundRequest represents a notification request that is about to be published to the AMQP exchange.
type OutboundRequest struct {
	RequestType   string                 `json:"type"`
	User          string                 `json:"user"`
	Subject       string                 `json:"subject"`
	Timestamp     string                 `json:"timestamp"`
	Email         bool                   `json:"email"`
	EmailTemplate string                 `json:"email_template"`
	Payload       map[string]interface{} `json:"payload"`
	Message       string                 `json:"message"`
}

// NotificationRequestHandler handles POST requests to the /notification endpoint.
func (a API) NotificationRequestHandler(ctx echo.Context) error {
	var err error

	// Extract and validate the request body.
	notificationRequest := new(model.V1NotificationRequest)
	if err := ctx.Bind(notificationRequest); err != nil {
		return ctx.JSON(http.StatusBadRequest, model.InvalidRequestBody(err))
	}
	if err = ctx.Validate(notificationRequest); err != nil {
		return ctx.JSON(http.StatusBadRequest, model.InvalidRequestBody(err))
	}

	// Verify that we have the required values if an email was requested.
	if notificationRequest.Email {
		if err = validateEmailRequest(notificationRequest); err != nil {
			return ctx.JSON(http.StatusBadRequest, model.InvalidRequestBody(err))
		}
	}

	// Ensure that the analysis start date is in the correct format if it's present.
	err = fixTimestamp(notificationRequest.Payload, "startdate")
	if err != nil {
		return ctx.JSON(http.StatusBadRequest, model.InvalidRequestBody(err))
	}

	// Ensure that the analysis end date is in the correct format if it's present.
	err = fixTimestamp(notificationRequest.Payload, "enddate")
	if err != nil {
		return ctx.JSON(http.StatusBadRequest, model.InvalidRequestBody(err))
	}

	// Build and serialize the outbound request.
	outboundRequest := &OutboundRequest{
		RequestType:   notificationRequest.Type,
		User:          notificationRequest.User,
		Subject:       notificationRequest.Subject,
		Timestamp:     time.Now().Format(time.RFC3339Nano),
		Email:         notificationRequest.Email,
		EmailTemplate: notificationRequest.EmailTemplate,
		Payload:       notificationRequest.Payload,
		Message:       notificationRequest.Message,
	}
	body, err := json.Marshal(outboundRequest)
	if err != nil {
		a.Echo.Logger.Errorf("unable to marshal outbound request: %s", err.Error())
		return ctx.JSON(http.StatusInternalServerError, model.InternalError(err))
	}

	// Publish the request.
	routingKey := fmt.Sprintf("events.notification.update.%s", outboundRequest.RequestType)
	err = a.AMQPClient.PublishContextOpts(ctx.Request().Context(), routingKey, body, messaging.JSONPublishingOpts)
	if err != nil {
		a.Echo.Logger.Errorf("unable to publish outbound request: %s", err.Error())
		return ctx.JSON(http.StatusInternalServerError, model.InternalError(err))
	}

	return nil
}
