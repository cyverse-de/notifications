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

// NotFound formats an error response indicating that an entity could not be found.
func NotFound(desc string) *ErrorResponse {
	return &ErrorResponse{
		Message: fmt.Sprintf("%s not found", desc),
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

	// The message text of the notification.
	Message string `json:"message"`

	// True if an email should be sent for this notification.
	Email bool `json:"email"`

	// The email template to use if an email should be sent.
	EmailTemplate string `json:"email_template"`

	// The notification payload, which contains arbitrary information about the notification.
	Payload map[string]interface{} `json:"payload"`
}

// Notification describes a single notification in a notification listing.
type Notification struct {

	// True if the notification has been marked as deleted.
	Deleted bool `json:"deleted"`

	// The email address to send the notification to, if an email was requested.
	Email bool `json:"email"`

	// The template to use to generate the email, if an email was requested.
	EmailTemplate string `json:"email_template"`

	// General information about the notification message.
	Message map[string]interface{} `json:"message"`

	// More specific free-form information about the notification message.
	Payload interface{} `json:"payload"`

	// Indicates whether or not the message has been marked as seen by the user.
	Seen bool `json:"seen"`

	// The subject line of the notification.
	Subject string `json:"subject"`

	// The notification type.
	Type string `json:"type"`

	// The username of the notification recipient.
	User string `json:"user"`
}

// V1NotificationListing describes the response body to a notification listing request in version 1 of the API.
type V1NotificationListing struct {

	// The message listing.
	Messages []*Notification `json:"messages"`

	// The total number of messages available to be listed.
	Total string `json:"total"`

	// The total number of messages that haven't been marked as seen yet.
	UnseenTotal string `json:"unseen_total"`
}

// V2NotificationListing describes the response body to a notification listing request in version 2 of the API.
type V2NotificationListing struct {

	// The message listing. Note: this element will be missing if there are no matching messages or if only a message
	// count was requested.
	Messages []*Notification `json:"messages,omitempty"`

	// The total number of messages available to be listed.
	Total int `json:"total"`

	// The ID of the most recent messsage that preceded all of the messages in the current listing page. Note: this
	// element will be missing if there are no matching messages older than the all of the messages on the curernt
	// page or if only a message ount was requested.
	BeforeID string `json:"before_id,omitempty"`

	// The ID of the oldest message that succeeded the all of the messages in the current listing page. Note: this
	// element will be missing if there are no matching messages newer than all of the messages on the current page or
	// if only a message count was requested.
	AfterID string `json:"after_id,omitempty"`
}

// V1NotificationCounts describes the response body for a notification count request in version 1 of the API.
type V1NotificationCounts struct {

	// The number of messages counted for the user,
	UserTotal int `json:"user-total"`
}

// UUIDList represents a list of UUIDs sent in a request body.
type UUIDList struct {

	// The list of UUIDs.
	UUIDs []string `json:"uuids" validate:"dive,required,uuid_rfc4122"`
}

// UsernameWrapper represents a request body containing only a username.
type UsernameWrapper struct {

	// The username.
	User string `json:"user" validate:"required"`
}

// SuccessCount describes a response body that returns a success flag and the number of items updated.
type SuccessCount struct {

	// The success flag.
	Success bool `json:"success"`

	// The number of items updated.
	Count int `json:"count"`
}

// MultipleMessageUpdateRequest describes a request body indicating that multilpe notification messages should be
// updated.
type MultipleMessageUpdateRequest struct {

	// The list of IDs to update.
	IDs []string `json:"ids" validate:"dive,uuid_rfc4122"`

	// If true, all messages for the user will be updated and any identifiers sent in the `ids` element will be
	// ignored.
	AllNotifications bool `json:"all_notifications"`
}
