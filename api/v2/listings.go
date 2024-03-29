package v2

import (
	"fmt"
	"net/http"
	"time"

	"github.com/cyverse-de/notifications/db"
	"github.com/cyverse-de/notifications/model"
	"github.com/cyverse-de/notifications/query"
	"github.com/labstack/echo/v4"
)

// GetMessagesHandler handles requests for listing notification messages.
func (a *API) GetMessagesHandler(c echo.Context) error {
	var err error
	ctx := c.Request().Context()

	// Extract and validate the user query parameter.
	user, err := query.ValidatedQueryParam(c, "user", "required")
	if err != nil {
		return c.JSON(http.StatusBadRequest, model.ErrorResponse{
			Message: "missing required query parameter: user",
		})
	}

	// Extract and validate the limit query parameter.
	defaultLimit := uint64(0)
	limit, err := query.ValidateUIntQueryParam(c, "limit", &defaultLimit)
	if err != nil {
		return c.JSON(http.StatusBadRequest, model.ErrorResponse{
			Message: err.Error(),
		})
	}

	// Extract and validate the seen query parameter.
	defaultSeenValue := false
	seen, err := query.ValidateBooleanQueryParam(c, "seen", &defaultSeenValue)
	if err != nil {
		return c.JSON(http.StatusBadRequest, model.ErrorResponse{
			Message: err.Error(),
		})
	}

	// Extract and validate the sort-dir query parameter.
	defaultSortOrder := query.SortOrderDescending
	sortOrderParam := query.NewSortOrderParam(&defaultSortOrder)
	err = query.ValidateParseableParam(c, "sort-dir", sortOrderParam)
	if err != nil {
		return c.JSON(http.StatusBadRequest, model.ErrorResponse{
			Message: err.Error(),
		})
	}

	// Extract and validate the before-id query parameter.
	beforeID, err := query.ValidateUUIDQueryParam(c, "before-id", false)
	if err != nil {
		return c.JSON(http.StatusBadRequest, model.ErrorResponse{
			Message: err.Error(),
		})
	}

	// Extract and validate the after-id query parameter.
	afterID, err := query.ValidateUUIDQueryParam(c, "after-id", false)
	if err != nil {
		return c.JSON(http.StatusBadRequest, model.ErrorResponse{
			Message: err.Error(),
		})
	}

	// Extract and validate the count-only query parameter.
	defaultCountOnlyValue := false
	countOnly, err := query.ValidateBooleanQueryParam(c, "count-only", &defaultCountOnlyValue)
	if err != nil {
		return c.JSON(http.StatusBadRequest, model.ErrorResponse{
			Message: err.Error(),
		})
	}

	// Extract the subject-search query parameter.
	subjectSearch := c.QueryParam("subject-search")

	// Extract the type query parameter.
	notificationType := c.QueryParam("type")

	// Begin a database transaction
	tx, err := a.DB.Begin()
	if err != nil {
		a.Echo.Logger.Error(err)
		return err
	}
	defer func() {
		err = tx.Rollback()
	}()

	// Determine the before timestamp for the request.
	var beforeTimestamp *time.Time
	if beforeID != "" {
		beforeTimestamp, err = db.GetNotificationTimestamp(ctx, tx, beforeID)
		if err != nil {
			a.Echo.Logger.Error(err)
			return err
		}
		if beforeTimestamp == nil {
			return c.JSON(http.StatusBadRequest, model.ErrorResponse{
				Message: fmt.Sprintf("message %s does not exist", beforeID),
			})
		}
	}

	// Determine the after timestamp for the request.
	var afterTimestamp *time.Time
	if afterID != "" {
		afterTimestamp, err = db.GetNotificationTimestamp(ctx, tx, afterID)
		if err != nil {
			a.Echo.Logger.Error(err)
			return err
		}
		if afterTimestamp == nil {
			return c.JSON(http.StatusBadRequest, model.ErrorResponse{
				Message: fmt.Sprintf("message %s does not exist", afterID),
			})
		}
	}

	// Obtain the listing.
	params := &db.V2NotificationListingParameters{
		User:             user,
		Limit:            limit,
		Seen:             seen,
		SortOrder:        *(sortOrderParam.GetValue().(*query.SortOrder)),
		BeforeID:         beforeID,
		BeforeTimestamp:  beforeTimestamp,
		AfterID:          afterID,
		AfterTimestamp:   afterTimestamp,
		CountOnly:        countOnly,
		SubjectSearch:    subjectSearch,
		NotificationType: notificationType,
	}
	listing, err := db.V2ListNotifications(ctx, tx, params)
	if err != nil {
		a.Echo.Logger.Error(err)
		return err
	}

	return c.JSON(http.StatusOK, listing)
}

// GetMessageHandler handles requests for obtaining information about a single notification.
func (a *API) GetMessageHandler(ctx echo.Context) error {

	// Extract and validate the notification ID.
	id, err := query.ValidatedPathParam(ctx, "id", "uuid_rfc4122")
	if err != nil {
		return ctx.JSON(http.StatusBadRequest, model.ErrorResponse{
			Message: "invalid notification ID",
		})
	}

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
	defer func() {
		err = tx.Rollback()
	}()

	// Get the notification.
	notification, err := db.GetNotification(ctx.Request().Context(), tx, user, id)
	if err != nil {
		a.Echo.Logger.Error(err)
		return err
	}

	// Return a 404 if the notification wasn't found.
	if notification == nil {
		desc := fmt.Sprintf("notification ID %s", id)
		return ctx.JSON(http.StatusNotFound, model.NotFound(desc))
	}

	return ctx.JSON(http.StatusOK, notification)
}
