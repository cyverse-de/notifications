package v1

import (
	"fmt"
	"net/http"

	"github.com/cyverse-de/notifications/common"
	"github.com/cyverse-de/notifications/model"
	"github.com/labstack/echo"
)

// API defines version 1 of the REST API for the notifications service.
type API struct {
	Echo         *echo.Echo
	Group        *echo.Group
	AMQPSettings *common.AMQPSettings
	Service      string
	Title        string
	Version      string
}

// RootHandler handles GET requests to the /v1 endpoint.
func (a API) RootHandler(ctx echo.Context) error {
	resp := model.VersionRootResponse{
		Service:    a.Service,
		Title:      a.Title,
		Version:    a.Version,
		APIVersion: "v1",
	}
	return ctx.JSON(http.StatusOK, resp)
}

// NotificationRequestHandler handles POST requests to the /notification endpoint.
func (a API) NotificationRequestHandler(ctx echo.Context) error {

	// Extract and validate the request body.
	notificationRequest := new(model.V1NotificationRequest)
	err := ctx.Bind(notificationRequest)
	if err != nil {
		return ctx.JSON(http.StatusBadRequest, model.ErrorResponse{
			Message: fmt.Sprintf("invalid request body: %s", err.Error()),
		})
	}

	// Log the notification request.
	a.Echo.Logger.Infof("%+v", notificationRequest)

	return nil
}

// RegisterHandlers registers the supported request handlers.
func (a API) RegisterHandlers() {
	a.Group.GET("", a.RootHandler)
	a.Group.GET("/", a.RootHandler)
	a.Group.POST("/notification", a.NotificationRequestHandler)
}
