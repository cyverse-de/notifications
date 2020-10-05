package v1

import (
	"net/http"
	"strings"

	"github.com/cyverse-de/notifications/db"
	"github.com/cyverse-de/notifications/model"
	"github.com/cyverse-de/notifications/query"
	"github.com/labstack/echo"
)

// MarkMessagesAsSeen marks messages whose UUIDs appear in the request body as having been seen.
func (a *API) MarkMessagesAsSeen(ctx echo.Context) error {

	// Extract and validate the user query parameter.
	user, err := query.ValidatedQueryParam(ctx, "user", "required")
	if err != nil {
		return ctx.JSON(http.StatusBadRequest, model.ErrorResponse{
			Message: "missing required query parameter: user",
		})
	}

	// Extract and validate the request body.
	uuidList := new(model.UUIDList)
	if err := ctx.Bind(uuidList); err != nil {
		return ctx.JSON(http.StatusBadRequest, model.InvalidRequestBody(err))
	}
	if err = ctx.Validate(uuidList); err != nil {
		return ctx.JSON(http.StatusBadRequest, model.InvalidRequestBody(err))
	}

	// Start a transaction.
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

	// No notifications can be marked as seen if the user isn't in the database.
	if userID == "" {
		return ctx.JSON(http.StatusOK, &model.SuccessCount{
			Success: true,
			Count:   0,
		})
	}

	// Mark the notifications as seen.
	count, err := db.MarkMessagesAsSeen(tx, userID, uuidList.UUIDs)
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

	return ctx.JSON(http.StatusOK, &model.SuccessCount{
		Success: true,
		Count:   count,
	})
}

// MarkAllMessagesAsSeen marks all messages for the specified user as having been seen.
func (a *API) MarkAllMessagesAsSeen(ctx echo.Context) error {
	var err error

	// Extract and validate the request body.
	usernameWrapper := new(model.UsernameWrapper)
	if err = ctx.Bind(usernameWrapper); err != nil {
		return ctx.JSON(http.StatusBadRequest, model.InvalidRequestBody(err))
	}
	if err = ctx.Validate(usernameWrapper); err != nil {
		return ctx.JSON(http.StatusBadRequest, model.InvalidRequestBody(err))
	}

	// Start a transaction.
	tx, err := a.DB.Begin()
	if err != nil {
		a.Echo.Logger.Error(err)
		return err
	}
	defer tx.Rollback()

	// Obtain the user ID.
	userID, err := db.GetUserID(tx, usernameWrapper.User)
	if err != nil {
		a.Echo.Logger.Error(err)
		return err
	}

	// There's nothing to do if the user doesn't exist in the database.
	if userID == "" {
		return ctx.JSON(http.StatusOK, &model.SuccessCount{
			Count:   0,
			Success: true,
		})
	}

	// Delete all messages for the user ID.
	count, err := db.MarkAllMessagesAsSeen(tx, userID)
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

	return ctx.JSON(http.StatusOK, &model.SuccessCount{
		Count:   count,
		Success: true,
	})
}

// DeleteMessages deletes messages whose UUIDs appear in the request body, provided the messages are directed to the
// username in the query string.
func (a *API) DeleteMessages(ctx echo.Context) error {
	var err error

	// Extract and validate the user query parameter.
	user, err := query.ValidatedQueryParam(ctx, "user", "required")
	if err != nil {
		return ctx.JSON(http.StatusBadRequest, model.ErrorResponse{
			Message: "missing required query parameter: user",
		})
	}

	// Extract and validate the request body.
	uuidList := new(model.UUIDList)
	if err := ctx.Bind(uuidList); err != nil {
		return ctx.JSON(http.StatusBadRequest, model.InvalidRequestBody(err))
	}
	if err = ctx.Validate(uuidList); err != nil {
		return ctx.JSON(http.StatusBadRequest, model.InvalidRequestBody(err))
	}

	// Start a transaction.
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

	// No notifications can be deleted if the user isn't in the database.
	if userID == "" {
		return ctx.JSON(http.StatusOK, &model.SuccessCount{
			Success: true,
			Count:   0,
		})
	}

	// Delete the notifications.
	count, err := db.DeleteMessages(tx, userID, uuidList.UUIDs)
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

	return ctx.JSON(http.StatusOK, &model.SuccessCount{
		Success: true,
		Count:   count,
	})
}

// DeleteMatchingMessages deletes messages that match the filters passed in the query string.
func (a *API) DeleteMatchingMessages(ctx echo.Context) error {
	var err error
	var seen *bool

	// Extract and validate the user query parameter.
	user, err := query.ValidatedQueryParam(ctx, "user", "required")
	if err != nil {
		return ctx.JSON(http.StatusBadRequest, model.ErrorResponse{
			Message: "missing required query parameter: user",
		})
	}

	// Extract and reformat the filter.
	var notificationType string
	filter := strings.ReplaceAll(strings.ToLower(ctx.QueryParam("filter")), " ", "_")
	if filter == "new" {
		seen = new(bool)
		*seen = false
	} else if filter != "" {
		notificationType = filter
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

	// No notifications can be deleted if the user isn't in the database.
	if userID == "" {
		return ctx.JSON(http.StatusOK, &model.SuccessCount{
			Success: true,
			Count:   0,
		})
	}

	// Look up the notification type ID if the notification type was specified.
	var notificationTypeID string
	if notificationType != "" {
		notificationTypeID, err = db.GetNotificationTypeID(tx, notificationType)
		if err != nil {
			a.Echo.Logger.Error(err)
			return err
		}

		// no notifications can be deleted if the notificaiton type was specified but not found.
		if notificationTypeID == "" {
			return ctx.JSON(http.StatusOK, &model.SuccessCount{
				Success: true,
				Count:   0,
			})
		}
	}

	// Delete matching notifications.
	params := &db.DeleteMatchingMessagesParameters{
		Seen:               seen,
		NotificationTypeID: notificationTypeID,
	}
	count, err := db.DeleteMatchingMessages(tx, userID, params)
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

	return ctx.JSON(http.StatusOK, &model.SuccessCount{
		Success: true,
		Count:   count,
	})
}
