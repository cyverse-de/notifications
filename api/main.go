package api

import (
	"net/http"

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
}
