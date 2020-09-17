package v2

import (
	"fmt"
	"net/http"

	"github.com/cyverse-de/notifications/db"
	"github.com/cyverse-de/notifications/model"
	"github.com/cyverse-de/notifications/query"
	"github.com/labstack/echo"
)

// MarkMessageSeenHandler updates a message in the database to indicate that the user has already seen it.
func (a *API) MarkMessageSeenHandler(ctx echo.Context) error {

	// Extract and validate the notification ID.
	id, err := query.ValidatedPathParam(ctx, "id", "uuid_rfc4122")
	if err != nil {
		return ctx.JSON(http.StatusBadRequest, model.ErrorResponse{
			Message: "invalid notification ID",
		})
	}

	// This is used later if we can't find the notification to update.
	notificationDesc := fmt.Sprintf("notification ID %s", id)

	// Extract and validate the user query parameter.
	user, err := query.ValidatedQueryParam(ctx, "user", "required")
	if err != nil {
		return ctx.JSON(http.StatusBadRequest, model.ErrorResponse{
			Message: "missing required query parameter: user",
		})
	}

	// Begin a database transaction
	tx, err := a.DB.Begin()
	if err != nil {
		a.Echo.Logger.Error(err)
		return err
	}
	defer tx.Rollback()

	// Look up the user ID.
	userID, err := db.GetUserID(tx, user)
	if err != nil {
		a.Echo.Logger.Error(err)
		return err
	}

	// The notification can't be directed to the user if the user isn't in the database.
	if userID == "" {
		return ctx.JSON(http.StatusNotFound, model.NotFound(notificationDesc))
	}

	// Update the notification.
	count, err := db.MarkMessageAsSeen(tx, userID, id)
	if err != nil {
		a.Echo.Logger.Error(err)
		return err
	}

	// Return a 404 no notifications were updated.
	if count == 0 {
		return ctx.JSON(http.StatusNotFound, model.NotFound(notificationDesc))
	}

	// Commit the transaction.
	err = tx.Commit()
	if err != nil {
		a.Echo.Logger.Error(err)
		return err
	}

	return nil
}

// DeleteMessageHandler updates a message in the database to indicate that it has been deleted.
func (a *API) DeleteMessageHandler(ctx echo.Context) error {

	// Extract and validate the notification ID.
	id, err := query.ValidatedPathParam(ctx, "id", "uuid_rfc4122")
	if err != nil {
		return ctx.JSON(http.StatusBadRequest, model.ErrorResponse{
			Message: "invalid notification ID",
		})
	}

	// This is used later if we can't find the notification to update.
	notificationDesc := fmt.Sprintf("notification ID %s", id)

	// Extract and validate the user query parameter.
	user, err := query.ValidatedQueryParam(ctx, "user", "required")
	if err != nil {
		return ctx.JSON(http.StatusBadRequest, model.ErrorResponse{
			Message: "missing required query parameter: user",
		})
	}

	// Begin a database transaction
	tx, err := a.DB.Begin()
	if err != nil {
		a.Echo.Logger.Error(err)
		return err
	}
	defer tx.Rollback()

	// Look up the user ID.
	userID, err := db.GetUserID(tx, user)
	if err != nil {
		a.Echo.Logger.Error(err)
		return err
	}

	// The notification can't be directed to the user if the user isn't in the database.
	if userID == "" {
		return ctx.JSON(http.StatusNotFound, model.NotFound(notificationDesc))
	}

	// Delete the notification.
	count, err := db.DeleteMessage(tx, userID, id)
	if err != nil {
		a.Echo.Logger.Error(err)
		return err
	}

	// Return a 404 no notifications were updated.
	if count == 0 {
		return ctx.JSON(http.StatusNotFound, model.NotFound(notificationDesc))
	}

	// Commit the transaction.
	err = tx.Commit()
	if err != nil {
		a.Echo.Logger.Error(err)
		return err
	}

	return nil
}
