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
