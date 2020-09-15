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
type notificationListingParametersV1 struct {

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
type notificationListingV1 struct {
	// in:body
	Body model.V1NotificationListing
}

// swagger:route GET /v1/count-messages v1 countMessagesV1
//
// Count Notification Messages
//
// Returns notification counts for the user.
//
// responses:
//   200: v1NotificationCounts
//   400: errorResponse
//   500: errorResponse

// Parameters for the /v1/count-messages endpoint.
// swagger:parameters countMessagesV1
type countMessagesV1Parameters struct {

	// The username of the person to count notifications for.
	//
	// in:query
	// required: true
	User string `json:"user"`

	// If true, only messages that have been marked as seen will be counted. If false, only messages that have not
	// been marked as seen will be counted. If not specified, messages will be counted regardless of whether or
	// not they've been marked as seen.
	//
	// in:query
	Seen bool `json:"seen"`

	// The type of notifications to count. The value of this query parameter is modified before executing the database
	// query; letters are convered to lower case and spaces are replaced with underscores. For example,
	// `NOTIFICATION Type` is equivalent to `notification_type`.
	//
	// in:query
	Filter string `json:"filter"`
}

// Notification Count Listing
// swagger:response v1NotificationCounts
type v1NotificationCounts struct {
	// in:body
	Body model.V1NotificationCounts
}

// swagger:route POST /v1/seen v1 markMessagesAsSeenV1
//
// Mark Messages as Seen
//
// This endpoint updates messages in the notifications database to indicate that the user has seen them before.
//
// The response body contains a success flag along with the number of messages that were marked as seen. Messages that
// have already been marked as seen are included in the count so that callers can compare the number of message IDs
// in the request to the number of messages affected. If the numbers are different then chances are that some of the
// message IDs in the request body either don't exist or are directed to a different user.
//
// responses:
//   200: successCount
//   400: errorResponse
//   500: errorResponse

// Parameters for the /v1/seen endpoint.
// swagger:parameters markMessagesAsSeenV1
type markMessagesAsSeenV1Parameters struct {

	// The username of the person whose notifications are being marked as having been seen.
	//
	// in:query
	// required: true
	User string `json:"user"`

	// The list of message UUIDs to mark as having been seen.
	//
	// in:body
	// required: true
	Body model.UUIDList
}

// swagger:route POST /v1/mark-all-seen v1 markAllMessagesAsSeenV1
//
// Mark All Messages for a User As Seen
//
// This endpoint updates all notifications in the database for the specified user to indicate that the user has seen
// them before.
//
// The count in the response body indicates the number of messages that were marked as seen. Only messages that
// previously were not marked as seen are included in this count.
//
// responses:
//   200: successCount
//   400: errorResponse
//   500: errorResponse

// Parameters for the /v1/mark-all-seen endpoint.
// swagger:parameters markAllMessagesAsSeenV1
type markAllMessagesAsSeenV1Parameters struct {

	// A request body containing the username of the person whose messages are being marked as seen.
	//
	// in:body
	// required: true
	Body model.UsernameWrapper
}

// swagger:route POST /v1/delete v1 deleteMessagesV1
//
// Delete Notification Messages
//
// This endpoint marks messages in the database as having been deleted. The user will no longer be able to view the
// messages after they have been marked as deleted.
//
// The response body contains a success flag along with the number of messages that were marked as deleted. Messages
// that were already marked as deleted are included in the count so that callers can compare the number of message
// IDs in the request body to the number of messages affected. If the numbers are different then chances are that some
// of the message IDs in the request body either don't exist or are directed to a different user.
//
// responses:
//   200: successCount
//   400: errorResponse
//   500: errorResponse

// Parameters for the /v1/delete endpoint.
// swagger:parameters deleteMessagesV1
type deleteMessagesV1Parameters struct {

	// The username of the person whose notifications are being deleted.
	//
	// in:query
	// required: true
	User string `json:"user"`

	// The list of message UUIDs to delete.
	//
	// in:body
	// required: true
	Body model.UUIDList
}

// swagger:route DELETE /v1/delete-all v1 deleteMatchingMessagesV1
//
// Delete Matching Notification Messages
//
// This endpoint deletes messages that match conditions in the search criteria passed in the query parameters.
//
// The response body contains a success flag along with the number of messages that were marked as deleted. Any
// messages that were already marked as deleted will not be included in the count.
//
// responses:
//   200: successCount
//   400: errorResponse
//   500: errorResponse

// Parameters for the /v1/delete-all endpoint.
// swagger:parameters deleteMatchingMessagesV1
type deleteMatchingMessagesV1Parameters struct {

	// the username of the person whose notifications are being deleted.
	//
	// in:query
	// required: true
	User string `json:"user"`

	// The type of notifications to delete or `new`. The value of this query parameter is modified before updating
	// the database; letters are convered to lower case and spaces are replaced with underscores. For example,
	// `NOTIFICATION Type` is equivalent to `notification_type`. The special value, `new`, can be used to indicate
	// that messages of any type that have not been marked as seen should be deleted.
	//
	// in:query
	Filter string `json:"filter"`
}

// Success flag with count.
// swagger:response successCount
type successCountResponse struct {
	// in:body
	Body model.SuccessCount
}
