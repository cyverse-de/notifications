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
