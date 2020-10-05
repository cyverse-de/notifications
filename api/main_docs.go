// Package api DE Notifications API
//
// Documentation of the DE Notifications API
//
//     Schemes: http
//     BasePath: /
//     Version: 1.0.0
//
//     Consumes:
//     - application/json
//
//     Produces:
//     - application/json
//
// swagger:meta
package api

import "github.com/cyverse-de/notifications/model"

// swagger:route GET / misc getRoot
//
// General API Information
//
// Lists general information about the service API itself.
//
// responses:
//   200: rootResponse

// General information about the API.
// swagger:response rootResponse
type rootResponseWrapper struct {
	// in:body
	Body model.RootResponse
}

// Basic error response.
// swagger:response errorResponse
type errorResponseWrapper struct {
	// in:body
	Body model.ErrorResponse
}

// An empty response body.
// swagger:response emptyResponse
type emptyResponseWrapper struct{}
