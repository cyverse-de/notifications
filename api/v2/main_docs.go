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
// This endpoint lists notifications that match the criteria specified in the query parameters.
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
	// in:query
	BeforeID string `json:"before-id"`

	// If specified, only messages created after the message with the given ID will be returned.
	//
	// in:query
	AfterID string `json:"after-id"`

	// If specified, only messages created before the given timestamp will be returned. The timestamp should be in
	// RFC-3339 format with optional nanosecond precision. If both `before` and `before-id` are specified, `before-id`
	// takes precedence.
	//
	// in:query
	Before string `json:"before"`

	// If specified, only messages created after the given timestamp will be returned. The timestamp should be in
	// RFC-3339 format with optional nanosecond precision. If both `after` and `after-id` are specified, `after-id`
	// takes precedence.
	//
	// in:querhy
	After string `json:"after"`

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
	Body model.NotificationListing
}
