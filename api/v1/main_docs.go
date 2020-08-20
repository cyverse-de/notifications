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
	User *string `json:"user"`

	// The maximum number of results to return.
	//
	// in:query
	Limit *uint64 `json:"limit"`

	// The index of the first result to return.
	//
	// in:query
	Offset *uint64 `json:"offset"`
}

// Notification Listing
// swagger:response v1NotificationListing
type notificationListing struct {
	// in:body
	Body model.NotificationListing
}
