package v2

import (
	"fmt"
	"net/http"
	"time"

	"github.com/cyverse-de/notifications/db"
	"github.com/cyverse-de/notifications/model"
	"github.com/cyverse-de/notifications/query"
	"github.com/labstack/echo"
)

// GetMessagesHandler handles requests for listing notification messages.
func (a *API) GetMessagesHandler(ctx echo.Context) error {
	var err error

	// Extract and validate the user query parameter.
	user, err := query.ValidatedQueryParam(ctx, "user", "required")
	if err != nil {
		return ctx.JSON(http.StatusBadRequest, model.ErrorResponse{
			Message: "missing required query parameter: user",
		})
	}

	// Extract and validate the limit query parameter.
	defaultLimit := uint64(0)
	limit, err := query.ValidateUIntQueryParam(ctx, "limit", &defaultLimit)
	if err != nil {
		return ctx.JSON(http.StatusBadRequest, model.ErrorResponse{
			Message: err.Error(),
		})
	}

	// Extract and validate the seen query parameter.
	defaultSeenValue := false
	seen, err := query.ValidateBooleanQueryParam(ctx, "seen", &defaultSeenValue)
	if err != nil {
		return ctx.JSON(http.StatusBadRequest, model.ErrorResponse{
			Message: err.Error(),
		})
	}

	// Extract and validate the sort-dir query parameter.
	defaultSortOrder := query.SortOrderDescending
	sortOrderParam := query.NewSortOrderParam(&defaultSortOrder)
	err = query.ValidateParseableParam(ctx, "sort-dir", sortOrderParam)
	if err != nil {
		return ctx.JSON(http.StatusBadRequest, model.ErrorResponse{
			Message: err.Error(),
		})
	}

	// Extract and validate the before-id query parameter.
	beforeID, err := query.ValidateUUIDQueryParam(ctx, "before-id", false)
	if err != nil {
		return ctx.JSON(http.StatusBadRequest, model.ErrorResponse{
			Message: err.Error(),
		})
	}

	// Extract and validate the before query parameter.
	var defaultBefore time.Time
	beforeParam := query.NewTimestampParam(&defaultBefore)
	err = query.ValidateParseableParam(ctx, "before", beforeParam)
	if err != nil {
		return ctx.JSON(http.StatusBadRequest, model.ErrorResponse{
			Message: err.Error(),
		})
	}

	// Extract and validate the after-id query parameter.
	afterID, err := query.ValidateUUIDQueryParam(ctx, "after-id", false)
	if err != nil {
		return ctx.JSON(http.StatusBadRequest, model.ErrorResponse{
			Message: err.Error(),
		})
	}

	// Extract and validate the after query parameter.
	var defaultAfter time.Time
	afterParam := query.NewTimestampParam(&defaultAfter)
	err = query.ValidateParseableParam(ctx, "after", afterParam)
	if err != nil {
		return ctx.JSON(http.StatusBadRequest, model.ErrorResponse{
			Message: err.Error(),
		})
	}

	// Begin a database transaction
	tx, err := a.DB.Begin()
	if err != nil {
		a.Echo.Logger.Error(err)
		return err
	}
	defer tx.Rollback()

	// Determine the before timestamp for the request.
	var beforeTimestamp *time.Time
	if beforeID != "" {
		beforeTimestamp, err = db.GetNotificationTimestamp(tx, beforeID)
		if err != nil {
			a.Echo.Logger.Error(err)
			return err
		}
		if beforeTimestamp == nil {
			return ctx.JSON(http.StatusBadRequest, model.ErrorResponse{
				Message: fmt.Sprintf("message %s does not exist", beforeID),
			})
		}
	} else {
		beforeTimestamp = beforeParam.GetValue().(*time.Time)
		if *beforeTimestamp == defaultBefore {
			beforeTimestamp = nil
		}
	}

	// Determine the after timestamp for the request.
	var afterTimestamp *time.Time
	if afterID != "" {
		afterTimestamp, err = db.GetNotificationTimestamp(tx, afterID)
		if err != nil {
			a.Echo.Logger.Error(err)
			return err
		}
		if afterTimestamp == nil {
			return ctx.JSON(http.StatusBadRequest, model.ErrorResponse{
				Message: fmt.Sprintf("message %s does not exist", afterID),
			})
		}
	} else {
		afterTimestamp = afterParam.GetValue().(*time.Time)
		if *afterTimestamp == defaultAfter {
			afterTimestamp = nil
		}
	}

	// Obtain the listing.
	params := &db.V2NotificationListingParameters{
		User:            user,
		Limit:           limit,
		Seen:            seen,
		SortOrder:       *(sortOrderParam.GetValue().(*query.SortOrder)),
		BeforeTimestamp: beforeTimestamp,
		AfterTimestamp:  afterTimestamp,
	}
	listing, err := db.V2ListNotifications(tx, params)
	if err != nil {
		a.Echo.Logger.Error(err)
		return err
	}

	// TODO: add filtering based on the beforeID an afterID parameters.
	return ctx.JSON(http.StatusOK, listing)
}
