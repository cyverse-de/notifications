package api

import (
	"io"
	"net/http"

	"github.com/cyverse-de/notifications/mailer"
	"github.com/cyverse-de/notifications/model"
	"github.com/labstack/echo/v4"
	"go.opentelemetry.io/otel/trace"
)

// EmailRequestHandler handles POST requests to the /mail endpoint. The path differs from the
// retired de-mailer service, which served this at its root, but callers post to a configured
// base URL with nothing appended, so only that base URL had to change.
func (a API) EmailRequestHandler(ctx echo.Context) error {
	span := trace.SpanFromContext(ctx.Request().Context())

	body, err := io.ReadAll(ctx.Request().Body)
	if err != nil {
		a.Echo.Logger.Errorf("failed to read email request body: %s", err.Error())
		span.RecordError(err)
		return ctx.JSON(http.StatusInternalServerError, model.InternalError(err))
	}

	if err := a.Mailer.Process(ctx.Request().Context(), body); err != nil {
		a.Echo.Logger.Errorf("failed to process email request: %s", err.Error())
		span.RecordError(err)

		code := mailer.ErrorCode(err)
		message := err.Error()

		// Server-side failures (SMTP dial errors, template bugs) can carry internal host
		// and path details; log them but keep them out of the response body.
		if code >= http.StatusInternalServerError {
			message = "failed to process the email request; see the notifications logs for details"
		}
		return ctx.JSON(code, &model.ErrorResponse{Message: message})
	}

	return ctx.JSON(http.StatusOK, &model.SuccessResponse{Success: true})
}
