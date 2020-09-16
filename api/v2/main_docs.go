// Package v2 DE Notifications API Version 2
//
// Documentation of the DE Notifications API Version 2
//
//     Schemes: http
//     BasePath: /v2
//     Version: 1.0.0
//
//     Consumes:
//     - application/json
//
//     Produces:
//     - application/json
//
// swagger:meta
package v2

import "github.com/cyverse-de/notifications/model"

// swagger:route GET /v2 v2 getRoot
//
// Information About API Version 2
//
// Lists general information about API version 2.
//
// responses:
//   200: v2RootResponse

// Information About API Version 2
// swagger:response v2RootResponse
type rootResponseWrapper struct {
	// in:body
	Body model.VersionRootResponse
}

// swagger:route GET /v2/messages v2 listMessagesV2
//
// List Notification Messages
//
// This endpoint lists notifications that match the criteria specified in the query parameters. Notifications are
// always sorted by timestamp then by ID.
//
// Paging is managed using two query parameters. The `before-id` query parameter indicates that only messages that
// were created after the message with the given ID should be listed. Similarly, the `after-id` query parameter
// indicates that only messages that were created after the message with the given ID should be listed. These two
// parameters are intended to be used exclusively for paging. They cannot be used together to list messages that were
// created within a specific time window.
//
// responses:
//   200: v2NotificationListing
//   400: errorResponse
//   500: errorResponse

// Parameters for the /v2/messages endpoint.
// swagger:parameters listMessagesV2
type notificationListingParametersV2 struct {

	// The username of the person to list notifications for.
	//
	// in:query
	// required: true
	User string `json:"user"`

	// The maximum number of results to return. If left unspecified or set to zero, there will be no limit to the
	// number of results returned.
	//
	// in:query
	// default: 0
	Limit uint64 `json:"limit"`

	// If true, messages that have been seen before will be included in the response.
	//
	// in:query
	// default: false
	Seen bool `json:"seen"`

	// The direction to use when sorting results.
	//
	// in:query
	// enum: asc,desc
	// default: desc
	SortDir string `json:"sort-dir"`

	// If specified, only messages created before the message with the given ID will be returned.
	//
	// Note: `before-id` may not be used in conjunction with `after-id` to list messages created within a time window.
	//
	// in:query
	BeforeID string `json:"before-id"`

	// If specified, only messages created after the message with the given ID will be returned.
	//
	// Note: `after-id` may not be used in conjunction with `before-id` to list messages created within a time window.
	//
	// in:query
	AfterID string `json:"after-id"`

	// If true, only the number of matching messages will be returned.
	//
	// in:query
	// default: false
	CountOnly bool `json:"count-only"`

	// If specified, only messages with subjects containing the search string will be returned.
	//
	// in:query
	SubjectSearch string `json:"subject-search"`

	// If specified, only messages of the given type will be returned.
	//
	// in:query
	MessageType string `json:"type"`
}

// Notification Listing
// swagger:response v2NotificationListing
type notificationListingV2 struct {
	// in:body
	Body model.V2NotificationListing
}

// swagger:route GET /v2/messages/{id} v2 getMessageV2
//
// Get Notification Details
//
// This endpoint returns the notification with the specified ID.
//
// responses:
//   200: v2Notification
//   400: errorResponse
//   404: errorResponse
//   500: errorResponse

// Notification
// swagger:response v2Notification
type notificationV2 struct {
	// in:body
	Body model.Notification
}

// Parameters for the `/v2/messages/{id}` endpoint.
// swagger:parameters getMessageV2
type getNotificationParametersV2 struct {
	// The username of the person retrieving the notification.
	//
	// in:query
	// required: true
	User string `json:"user"`

	// The notification ID.
	//
	// in:path
	ID string
}
