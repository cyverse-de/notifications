package v1

import (
	"net/http"

	"github.com/cyverse-de/notifications/db"
	"github.com/cyverse-de/notifications/model"
	"github.com/cyverse-de/notifications/query"
	"github.com/labstack/echo"
)

// GetMessagesHandler handles requests for listing notification messages for a user.
func (a API) GetMessagesHandler(ctx echo.Context) error {
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

	// Extract and validate the offset query parameter.
	defaultOffset := uint64(0)
	offset, err := query.ValidateUIntQueryParam(ctx, "offset", &defaultOffset)
	if err != nil {
		return ctx.JSON(http.StatusBadRequest, model.ErrorResponse{
			Message: err.Error(),
		})
	}

	// Extract and validate the seen query parameter.
	seen, err := query.ValidateBoolPQueryParam(ctx, "seen")
	if err != nil {
		return ctx.JSON(http.StatusBadRequest, model.ErrorResponse{
			Message: err.Error(),
		})
	}

	// Extract and validate the sort_dir query parameter.
	defaultSortOrder := query.SortOrderDescending
	sortOrderParam := query.NewSortOrderParam(&defaultSortOrder)
	err = query.ValidateParseableParam(ctx, "sort_dir", sortOrderParam)
	if err != nil {
		return ctx.JSON(http.StatusBadRequest, model.ErrorResponse{
			Message: err.Error(),
		})
	}

	// Extract and validate the sort_field query parameter.
	defaultSortField := query.V1ListingSortFieldTimestamp
	sortFieldParam := query.NewV1ListingSortFieldParam(&defaultSortField)
	err = query.ValidateParseableParam(ctx, "sort_field", sortFieldParam)
	if err != nil {
		return ctx.JSON(http.StatusBadRequest, model.ErrorResponse{
			Message: err.Error(),
		})
	}

	// Start a transaction.
	tx, err := a.DB.Begin()
	if err != nil {
		a.Echo.Logger.Error(err)
		return err
	}
	defer tx.Rollback()

	// Obtain the listing.
	params := &db.V1NotificationListingParameters{
		User:      user,
		Limit:     limit,
		Offset:    offset,
		Seen:      seen,
		SortOrder: *(sortOrderParam.GetValue().(*query.SortOrder)),
		SortField: *(sortFieldParam.GetValue().(*query.V1ListingSortField)),
	}
	listing, err := db.V1ListNotifications(tx, params)
	if err != nil {
		a.Echo.Logger.Error(err)
		return err
	}

	return ctx.JSON(http.StatusOK, listing)
}