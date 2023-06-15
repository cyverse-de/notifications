package v2

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"strings"

	"github.com/cyverse-de/notifications/db"
	"github.com/cyverse-de/notifications/model"
	"github.com/cyverse-de/notifications/query"
	"github.com/labstack/echo/v4"
)

// updateMultipleMessages handles requests from endpoints that update multiple messages in the database.
func (a API) updateMultipleMessages(
	c echo.Context,
	updateFn func(context.Context, *sql.Tx, string, *model.MultipleMessageUpdateRequest) error,
) error {
	ctx := c.Request().Context()

	// Extract and validate the user query parameter.
	user, err := query.ValidatedQueryParam(c, "user", "required")
	if err != nil {
		return c.JSON(http.StatusBadRequest, model.ErrorResponse{
			Message: "missing required query parameter: user",
		})
	}

	// Parse and validate the message body.
	body := new(model.MultipleMessageUpdateRequest)
	err = c.Bind(body)
	if err != nil {
		return c.JSON(http.StatusBadRequest, model.InvalidRequestBody(err))
	}
	err = c.Validate(body)
	if err != nil {
		return c.JSON(http.StatusBadRequest, model.InvalidRequestBody(err))
	}

	// If we're not updating all messages for the user then some message IDs need to be specified.
	if !body.AllNotifications && len(body.IDs) == 0 {
		return c.JSON(http.StatusBadRequest, model.ErrorResponse{
			Message: "either `all_notifications` must be true or `ids` must be specified and not empty",
		})
	}

	// Begin a database transaction.
	tx, err := a.DB.Begin()
	if err != nil {
		a.Echo.Logger.Error(err)
		return err
	}
	defer func() {
		err = tx.Rollback()
	}()

	// Look up the user ID.
	userID, err := db.GetUserID(ctx, tx, user)
	if err != nil {
		a.Echo.Logger.Error(err)
		return err
	}

	// Nothing can be done if the user isn't in the database.
	if userID == "" {
		return c.JSON(http.StatusNotFound, model.ErrorResponse{
			Message: fmt.Sprintf("no messages found for user %s", user),
		})
	}

	// Validate the message IDs that we received if we're not updating all notifications for the user.
	if !body.AllNotifications {
		missingIDs, err := db.FilterMissingIDs(ctx, tx, userID, body.IDs)
		if err != nil {
			a.Echo.Logger.Error(err)
			return err
		}
		if len(missingIDs) > 0 {
			desc := fmt.Sprintf("notification IDs %s", strings.Join(missingIDs, ", "))
			return c.JSON(http.StatusNotFound, model.NotFound(desc))
		}
	}

	// Update the messages.
	err = updateFn(ctx, tx, userID, body)
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
	c echo.Context,
	updateFn func(context.Context, *sql.Tx, string, string) (int, error),
) error {
	ctx := c.Request().Context()

	// Extract and validate the notification ID.
	id, err := query.ValidatedPathParam(c, "id", "uuid_rfc4122")
	if err != nil {
		return c.JSON(http.StatusBadRequest, model.ErrorResponse{
			Message: "invalid notification ID",
		})
	}

	// This is used later if we can't find the notification to update.
	notificationDesc := fmt.Sprintf("notification ID %s", id)

	// Extract and validate the user query parameter.
	user, err := query.ValidatedQueryParam(c, "user", "required")
	if err != nil {
		return c.JSON(http.StatusBadRequest, model.ErrorResponse{
			Message: "missing required query parameter: user",
		})
	}

	// Begin a database transaction
	tx, err := a.DB.Begin()
	if err != nil {
		a.Echo.Logger.Error(err)
		return err
	}
	defer func() {
		err = tx.Rollback()
	}()

	// Look up the user ID.
	userID, err := db.GetUserID(ctx, tx, user)
	if err != nil {
		a.Echo.Logger.Error(err)
		return err
	}

	// The notification can't be directed to the user if the user isn't in the database.
	if userID == "" {
		return c.JSON(http.StatusNotFound, model.NotFound(notificationDesc))
	}

	// Update the notification.
	count, err := updateFn(ctx, tx, userID, id)
	if err != nil {
		a.Echo.Logger.Error(err)
		return err
	}

	// Return a 404 no notifications were updated.
	if count == 0 {
		return c.JSON(http.StatusNotFound, model.NotFound(notificationDesc))
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
func (a *API) MarkMultipleMessagesSeenHandler(c echo.Context) error {
	return a.updateMultipleMessages(c, func(ctx context.Context, tx *sql.Tx, userID string, body *model.MultipleMessageUpdateRequest) error {
		var err error

		if body.AllNotifications {
			_, err = db.MarkAllMessagesAsSeen(ctx, tx, userID)
			if err != nil {
				a.Echo.Logger.Error(err)
				return err
			}
		} else {
			_, err = db.MarkMessagesAsSeen(ctx, tx, userID, body.IDs)
			if err != nil {
				a.Echo.Logger.Error(err)
				return err
			}
		}

		return nil
	})
}

// DeleteMultipleMessagesHandler marks multiple messages in the database as deleted.
func (a *API) DeleteMultipleMessagesHandler(c echo.Context) error {
	return a.updateMultipleMessages(c, func(ctx context.Context, tx *sql.Tx, userID string, body *model.MultipleMessageUpdateRequest) error {
		var err error

		if body.AllNotifications {
			_, err = db.DeleteMatchingMessages(ctx, tx, userID, &db.DeleteMatchingMessagesParameters{})
			if err != nil {
				a.Echo.Logger.Error(err)
				return err
			}
		} else {
			_, err = db.DeleteMessages(ctx, tx, userID, body.IDs)
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
