package model

import "fmt"

// RootResponse describes the response of the root endpoint.
type RootResponse struct {

	// The name of the service.
	Service string `json:"service"`

	// The service title.
	Title string `json:"title"`

	// The service version.
	Version string `json:"version"`
}

// ErrorResponse describes an error response for any endpoint.
type ErrorResponse struct {

	// A message describing the error.
	Message string `json:"message"`
}

// InvalidRequestBody formats an error response indicating that the request body was invalid.
func InvalidRequestBody(err error) *ErrorResponse {
	return &ErrorResponse{
		Message: fmt.Sprintf("invalid request body: %s", err.Error()),
	}
}

// InternalError formats an error message indicating that an internal error occurred.
func InternalError(err error) *ErrorResponse {
	return &ErrorResponse{
		Message: fmt.Sprintf("an internal error occurred: %s", err.Error()),
	}
}

// SuccessResponse describes a succesfful response for endpoints with nothing else to return.
type SuccessResponse struct {

	// A simple success indicator.
	Success bool `json:"success"`
}

// VersionRootResponse describes the response of the root enpdpoint for an API version.
type VersionRootResponse struct {

	// The name of the service.
	Service string `json:"service"`

	// The service title.
	Title string `json:"title"`

	// The service version.
	Version string `json:"version"`

	// The API version.
	APIVersion string `json:"api_version"`
}

// V1NotificationRequest describes an incoming request to publish a notification.
type V1NotificationRequest struct {

	// The notification type.
	Type string `json:"type" validate:"required"`

	// The username of the notification recipient.
	User string `json:"user" validate:"required"`

	// The subject line of the notification.
	Subject string `json:"subject" validate:"required"`

	// The message text of the notification
	Message string `json:"message"`

	// True if an email should be sent for this notification.
	Email bool `json:"email"`

	// The email template to use if an email should be sent.
	EmailTemplate string `json:"email_template"`

	// The notification payload, which contains arbitrary information about the notification.
	Payload map[string]interface{} `json:"payload"`
}
