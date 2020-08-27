// Package v1 DE Notifications API Version 1
//
// Documentation of the DE Notifications API Version 1
//
//     Schemes: http
//     BasePath: /v1
//     Version: 1.0.0
//
//     Consumes:
//     - application/json
//
//     Produces:
//     - application/json
//
// swagger:meta
package v1

import "github.com/cyverse-de/notifications/model"

// swagger:route GET /v1 v1 getRoot
//
// Information About API Version 1
//
// Lists general information about API version 1.
//
// responses:
//   200: v1RootResponse

// Information About API Version 1
// swagger:response v1RootResponse
type rootResponseWrapper struct {
	// in:body
	Body model.VersionRootResponse
}

// swagger:route POST /v1/notification v1 requestNotificationV1
//
// Request a Notification
//
// Submits a request for a notification to be sent to a user.
//
// responses:
//   200: emptyResponse
//   400: errorResponse
//   500: errorResponse

// Parameters for requesting a notification.
// swagger:parameters requestNotificationV1
type requestNotificationParameters struct {

	// The notificaiton request.
	//
	// in:body
	Body model.V1NotificationRequest
}

// swagger:route GET /v1/messages v1 listMessagesV1
//
// List Notification Messages
//
// Lists notification messages for a user.
//
// responses:
//   200: v1NotificationListing
//   400: errorResponse
//   500: errorResponse

// Parameters for the /v1/messages endpoint.
// swagger:parameters listMessagesV1
type notificationListingParameters struct {

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

	// The index of the first result to return.
	//
	// in:query
	// default: 0
	Offset uint64 `json:"offset"`

	// If true, only messages that have been marked as seen will be displayed. If false, only messages that have not
	// been marked as seen will be displayed. If not specified, messages will be displayed regardless of whether or
	// not they've been marked as seen.
	//
	// in:query
	Seen bool `json:"seen"`

	// The field to use when sorting results.
	//
	// in:query
	// enum: date_created,timestamp,uuid,subject
	// default: timestamp
	SortField string `json:"sort_field"`

	// The direction to use when sorting results.
	//
	// in:query
	// enum: asc,desc
	// default: desc
	SortDir string `json:"sort_dir"`

	// The type of notifications to return in the response or `new`. The value of this query parameter is modified
	// before searching the database; letters are convered to lower case and spaces are replaced with underscores.
	// For example, `NOTIFICATION Type` is equivalent to `notification_type`.
	//
	// The special value, `new`, will cause the endpoint to list only unseen notifications. This is equivalent to
	// setting the `seen` query parameter to `false`, and it will override the `seen` query parameter.
	//
	// in:query
	Filter string `json:"filter"`
}

// swagger:route GET /v1/unseen_messages v1 listUnseenMessagesV1
//
// List Unseen Notification Messages
//
// Lists notification messages that have not been marked as seen yet for a user.
//
// responses:
//   200: v1NotificationListing
//   400: errorResponse
//   500: errorResponse

// Parameters for the /v1/unseen-messages endpoint.
// swagger:parameters listUnseenMessagesV1
type unseenNotificationListingParameters struct {

	// The username of the person to list notifications for.
	//
	// in:query
	// required: true
	User string `json:"user"`
}

// Notification Listing
// swagger:response v1NotificationListing
type notificationListing struct {
	// in:body
	Body model.NotificationListing
}
