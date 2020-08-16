package api

import (
	"net/http"

	v1 "github.com/cyverse-de/notifications/api/v1"
	v2 "github.com/cyverse-de/notifications/api/v2"
	"github.com/cyverse-de/notifications/common"
	"github.com/cyverse-de/notifications/model"
	"github.com/labstack/echo"
)

// API defines the REST API of the notifications service
type API struct {
	Echo         *echo.Echo
	AMQPSettings *common.AMQPSettings
	Service      string
	Title        string
	Version      string
}

// RootHandler handles GET requests to the / endpoint.
func (a API) RootHandler(ctx echo.Context) error {
	resp := model.RootResponse{
		Service: a.Service,
		Title:   a.Title,
		Version: a.Version,
	}
	return ctx.JSON(http.StatusOK, resp)
}

// RegisterHandlers registers the supported request handlers.
func (a API) RegisterHandlers() {
	a.Echo.GET("/", a.RootHandler)

	// Register the group for API version 1.
	v1Group := a.Echo.Group("/v1")
	v1API := v1.API{
		Group:        v1Group,
		AMQPSettings: a.AMQPSettings,
		Service:      a.Service,
		Title:        a.Title,
		Version:      a.Version,
	}
	v1API.RegisterHandlers()

	// Register the group for API version 2.
	v2Group := a.Echo.Group("/v2")
	v2API := v2.API{
		Group:        v2Group,
		AMQPSettings: a.AMQPSettings,
		Service:      a.Service,
		Title:        a.Title,
		Version:      a.Version,
	}
	v2API.RegisterHandlers()
}
