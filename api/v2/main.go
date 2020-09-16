package v2

import (
	"database/sql"
	"net/http"

	"github.com/cyverse-de/notifications/common"
	"github.com/cyverse-de/notifications/model"
	"github.com/labstack/echo"
	"gopkg.in/cyverse-de/messaging.v7"
)

// API defines version 1 of the REST API for the notifications service.
type API struct {
	Echo         *echo.Echo
	Group        *echo.Group
	AMQPSettings *common.AMQPSettings
	AMQPClient   *messaging.Client
	DB           *sql.DB
	Service      string
	Title        string
	Version      string
}

// RootHandler handles GET requests to the /v1/ endpoint.
func (a API) RootHandler(ctx echo.Context) error {
	resp := model.VersionRootResponse{
		Service:    a.Service,
		Title:      a.Title,
		Version:    a.Version,
		APIVersion: "v2",
	}
	return ctx.JSON(http.StatusOK, resp)
}

// RegisterHandlers registers the supported request handlers.
func (a API) RegisterHandlers() {
	a.Group.GET("", a.RootHandler)
	a.Group.GET("/", a.RootHandler)
	a.Group.GET("/messages", a.GetMessagesHandler)
	a.Group.GET("/messages/:id", a.GetMessageHandler)
}
