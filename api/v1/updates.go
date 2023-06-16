package v1

import (
	"net/http"
	"strings"

	"github.com/cyverse-de/notifications/db"
	"github.com/cyverse-de/notifications/model"
	"github.com/cyverse-de/notifications/query"
	"github.com/labstack/echo/v4"
)

// MarkMessagesAsSeen marks messages whose UUIDs appear in the request body as having been seen.
func (a *API) MarkMessagesAsSeen(c echo.Context) error {
	ctx := c.Request().Context()

	// Extract and validate the user query parameter.
	user, err := query.ValidatedQueryParam(c, "user", "required")
	if err != nil {
		return c.JSON(http.StatusBadRequest, model.ErrorResponse{
			Message: "missing required query parameter: user",
		})
	}

	// Extract and validate the request body.
	uuidList := new(model.UUIDList)
	if err := c.Bind(uuidList); err != nil {
		return c.JSON(http.StatusBadRequest, model.InvalidRequestBody(err))
	}
	if err = c.Validate(uuidList); err != nil {
		return c.JSON(http.StatusBadRequest, model.InvalidRequestBody(err))
	}

	// Start a transaction.
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

	// No notifications can be marked as seen if the user isn't in the database.
	if userID == "" {
		return c.JSON(http.StatusOK, &model.SuccessCount{
			Success: true,
			Count:   0,
		})
	}

	// Mark the notifications as seen.
	count, err := db.MarkMessagesAsSeen(ctx, tx, userID, uuidList.UUIDs)
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

	return c.JSON(http.StatusOK, &model.SuccessCount{
		Success: true,
		Count:   count,
	})
}

// MarkAllMessagesAsSeen marks all messages for the specified user as having been seen.
func (a *API) MarkAllMessagesAsSeen(c echo.Context) error {
	var err error
	ctx := c.Request().Context()

	// Extract and validate the request body.
	usernameWrapper := new(model.UsernameWrapper)
	if err = c.Bind(usernameWrapper); err != nil {
		return c.JSON(http.StatusBadRequest, model.InvalidRequestBody(err))
	}
	if err = c.Validate(usernameWrapper); err != nil {
		return c.JSON(http.StatusBadRequest, model.InvalidRequestBody(err))
	}

	// Start a transaction.
	tx, err := a.DB.Begin()
	if err != nil {
		a.Echo.Logger.Error(err)
		return err
	}
	defer func() {
		err = tx.Rollback()
	}()

	// Obtain the user ID.
	userID, err := db.GetUserID(ctx, tx, usernameWrapper.User)
	if err != nil {
		a.Echo.Logger.Error(err)
		return err
	}

	// There's nothing to do if the user doesn't exist in the database.
	if userID == "" {
		return c.JSON(http.StatusOK, &model.SuccessCount{
			Count:   0,
			Success: true,
		})
	}

	// Delete all messages for the user ID.
	count, err := db.MarkAllMessagesAsSeen(ctx, tx, userID)
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

	return c.JSON(http.StatusOK, &model.SuccessCount{
		Count:   count,
		Success: true,
	})
}

// DeleteMessages deletes messages whose UUIDs appear in the request body, provided the messages are directed to the
// username in the query string.
func (a *API) DeleteMessages(c echo.Context) error {
	var err error
	ctx := c.Request().Context()

	// Extract and validate the user query parameter.
	user, err := query.ValidatedQueryParam(c, "user", "required")
	if err != nil {
		return c.JSON(http.StatusBadRequest, model.ErrorResponse{
			Message: "missing required query parameter: user",
		})
	}

	// Extract and validate the request body.
	uuidList := new(model.UUIDList)
	if err := c.Bind(uuidList); err != nil {
		return c.JSON(http.StatusBadRequest, model.InvalidRequestBody(err))
	}
	if err = c.Validate(uuidList); err != nil {
		return c.JSON(http.StatusBadRequest, model.InvalidRequestBody(err))
	}

	// Start a transaction.
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

	// No notifications can be deleted if the user isn't in the database.
	if userID == "" {
		return c.JSON(http.StatusOK, &model.SuccessCount{
			Success: true,
			Count:   0,
		})
	}

	// Delete the notifications.
	count, err := db.DeleteMessages(ctx, tx, userID, uuidList.UUIDs)
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

	return c.JSON(http.StatusOK, &model.SuccessCount{
		Success: true,
		Count:   count,
	})
}

// DeleteMatchingMessages deletes messages that match the filters passed in the query string.
func (a *API) DeleteMatchingMessages(c echo.Context) error {
	var err error
	var seen *bool
	ctx := c.Request().Context()

	// Extract and validate the user query parameter.
	user, err := query.ValidatedQueryParam(c, "user", "required")
	if err != nil {
		return c.JSON(http.StatusBadRequest, model.ErrorResponse{
			Message: "missing required query parameter: user",
		})
	}

	// Extract and reformat the filter.
	var notificationType string
	filter := strings.ReplaceAll(strings.ToLower(c.QueryParam("filter")), " ", "_")
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
	defer func() {
		err = tx.Rollback()
	}()

	// Look up the user ID.
	userID, err := db.GetUserID(ctx, tx, user)
	if err != nil {
		a.Echo.Logger.Error(err)
		return err
	}

	// No notifications can be deleted if the user isn't in the database.
	if userID == "" {
		return c.JSON(http.StatusOK, &model.SuccessCount{
			Success: true,
			Count:   0,
		})
	}

	// Look up the notification type ID if the notification type was specified.
	var notificationTypeID string
	if notificationType != "" {
		notificationTypeID, err = db.GetNotificationTypeID(ctx, tx, notificationType)
		if err != nil {
			a.Echo.Logger.Error(err)
			return err
		}

		// no notifications can be deleted if the notificaiton type was specified but not found.
		if notificationTypeID == "" {
			return c.JSON(http.StatusOK, &model.SuccessCount{
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
	count, err := db.DeleteMatchingMessages(ctx, tx, userID, params)
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

	return c.JSON(http.StatusOK, &model.SuccessCount{
		Success: true,
		Count:   count,
	})
}
