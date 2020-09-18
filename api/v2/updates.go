package v2

import (
	"database/sql"
	"fmt"
	"net/http"
	"strings"

	"github.com/cyverse-de/notifications/db"
	"github.com/cyverse-de/notifications/model"
	"github.com/cyverse-de/notifications/query"
	"github.com/labstack/echo"
)

// updateMultipleMessages handles requests from endpoints that update multiple messages in the database.
func (a API) updateMultipleMessages(
	ctx echo.Context,
	updateFn func(*sql.Tx, string, *model.MultipleMessageUpdateRequest) error,
) error {
	// Extract and validate the user query parameter.
	user, err := query.ValidatedQueryParam(ctx, "user", "required")
	if err != nil {
		return ctx.JSON(http.StatusBadRequest, model.ErrorResponse{
			Message: "missing required query parameter: user",
		})
	}

	// Parse and validate the message body.
	body := new(model.MultipleMessageUpdateRequest)
	err = ctx.Bind(body)
	if err != nil {
		return ctx.JSON(http.StatusBadRequest, model.InvalidRequestBody(err))
	}
	err = ctx.Validate(body)
	if err != nil {
		return ctx.JSON(http.StatusBadRequest, model.InvalidRequestBody(err))
	}

	// If we're not updating all messages for the user then some message IDs need to be specified.
	if !body.AllNotifications && len(body.IDs) == 0 {
		return ctx.JSON(http.StatusBadRequest, model.ErrorResponse{
			Message: "either `all_notifications` must be true or `ids` must be specified and not empty",
		})
	}

	// Begin a database transaction.
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

	// Nothing can be done if the user isn't in the database.
	if userID == "" {
		return ctx.JSON(http.StatusNotFound, model.ErrorResponse{
			Message: fmt.Sprintf("no messages found for user %s", user),
		})
	}

	// Validate the message IDs that we received if we're not updating all notifications for the user.
	if !body.AllNotifications {
		missingIDs, err := db.FilterMissingIDs(tx, userID, body.IDs)
		if err != nil {
			a.Echo.Logger.Error(err)
			return err
		}
		if len(missingIDs) > 0 {
			desc := fmt.Sprintf("notification IDs %s", strings.Join(missingIDs, ", "))
			return ctx.JSON(http.StatusNotFound, model.NotFound(desc))
		}
	}

	// Update the messages.
	err = updateFn(tx, userID, body)
	if err != nil {
		a.Echo.Logger.Error(err)
		return err
	}

	// Commit the transaction.
	err = tx.Commit()
	if err != nil {
		a.Echo.Logger.Error(err)
		return err
	}

	return nil
}

// updateSingleMessage handles requests to update a single message in the database.
func (a *API) updateSingleMessage(
	ctx echo.Context,
	updateFn func(*sql.Tx, string, string) (int, error),
) error {

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
	count, err := updateFn(tx, userID, id)
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

// MarkMultipleMessagesSeenHandler updates multiple messages in the databse to indicate that the user has already seen
// them.
func (a *API) MarkMultipleMessagesSeenHandler(ctx echo.Context) error {
	return a.updateMultipleMessages(ctx, func(tx *sql.Tx, userID string, body *model.MultipleMessageUpdateRequest) error {
		var err error

		if body.AllNotifications {
			_, err = db.MarkAllMessagesAsSeen(tx, userID)
			if err != nil {
				a.Echo.Logger.Error(err)
				return err
			}
		} else {
			_, err = db.MarkMessagesAsSeen(tx, userID, body.IDs)
			if err != nil {
				a.Echo.Logger.Error(err)
				return err
			}
		}

		return nil
	})
}

// DeleteMultipleMessagesHandler marks multiple messages in the database as deleted.
func (a *API) DeleteMultipleMessagesHandler(ctx echo.Context) error {
	return a.updateMultipleMessages(ctx, func(tx *sql.Tx, userID string, body *model.MultipleMessageUpdateRequest) error {
		var err error

		if body.AllNotifications {
			_, err = db.DeleteMatchingMessages(tx, userID, &db.DeleteMatchingMessagesParameters{})
			if err != nil {
				a.Echo.Logger.Error(err)
				return err
			}
		} else {
			_, err = db.DeleteMessages(tx, userID, body.IDs)
			if err != nil {
				a.Echo.Logger.Error(err)
				return err
			}
		}

		return nil
	})
}

// MarkMessageSeenHandler updates a message in the database to indicate that the user has already seen it.
func (a *API) MarkMessageSeenHandler(ctx echo.Context) error {
	return a.updateSingleMessage(ctx, db.MarkMessageAsSeen)
}

// DeleteMessageHandler updates a message in the database to indicate that it has been deleted.
func (a *API) DeleteMessageHandler(ctx echo.Context) error {
	return a.updateSingleMessage(ctx, db.DeleteMessage)
}
