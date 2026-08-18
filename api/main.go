package api

import (
	"database/sql"
	"net/http"

	"github.com/cyverse-de/messaging/v12"
	v1 "github.com/cyverse-de/notifications/api/v1"
	v2 "github.com/cyverse-de/notifications/api/v2"
	"github.com/cyverse-de/notifications/common"
	"github.com/cyverse-de/notifications/mailer"
	"github.com/cyverse-de/notifications/model"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

// emailRequestBodyLimit caps the size of a request to the /mail endpoint. Attachments arrive
// base64-encoded in the JSON body, so this allows roughly 12MB of attachments, comfortably more
// than the SMTP relay will accept anyway.
const emailRequestBodyLimit = "16M"

// API defines the REST API of the notifications service
type API struct {
	Echo         *echo.Echo
	AMQPSettings *common.AMQPSettings
	AMQPClient   *messaging.Client
	DB           *sql.DB
	UserSuffix   common.UserSuffix
	Mailer       *mailer.EmailProcessor
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

	// Outbound email. Unversioned because it isn't part of the notifications API proper; it
	// was absorbed from the retired de-mailer service, whose callers post to a bare base URL.
	// The body limit applies only to this route because it's the only one whose handler reads
	// the whole body into memory, and this service also serves the notifications API.
	a.Echo.POST("/mail", a.EmailRequestHandler, middleware.BodyLimit(emailRequestBodyLimit))

	// Register the group for API version 1.
	v1Group := a.Echo.Group("/v1")
	v1API := v1.API{
		Echo:         a.Echo,
		Group:        v1Group,
		AMQPSettings: a.AMQPSettings,
		AMQPClient:   a.AMQPClient,
		DB:           a.DB,
		UserSuffix:   a.UserSuffix,
		Service:      a.Service,
		Title:        a.Title,
		Version:      a.Version,
	}
	v1API.RegisterHandlers()

	// Register the group for API version 2.
	v2Group := a.Echo.Group("/v2")
	v2API := v2.API{
		Echo:         a.Echo,
		Group:        v2Group,
		AMQPSettings: a.AMQPSettings,
		AMQPClient:   a.AMQPClient,
		DB:           a.DB,
		UserSuffix:   a.UserSuffix,
		Service:      a.Service,
		Title:        a.Title,
		Version:      a.Version,
	}
	v2API.RegisterHandlers()
}
