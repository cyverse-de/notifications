package v1

import (
	"net/http"
	"strings"

	"github.com/cyverse-de/notifications/db"
	"github.com/cyverse-de/notifications/model"
	"github.com/cyverse-de/notifications/query"
	"github.com/labstack/echo/v4"
)

// GetMessagesHandler handles requests for listing notification messages for a user.
func (a API) GetMessagesHandler(c echo.Context) error {
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

	// Extract and validate the offset query parameter.
	defaultOffset := uint64(0)
	offset, err := query.ValidateUIntQueryParam(c, "offset", &defaultOffset)
	if err != nil {
		return c.JSON(http.StatusBadRequest, model.ErrorResponse{
			Message: err.Error(),
		})
	}

	// Extract and validate the seen query parameter.
	seen, err := query.ValidateBoolPQueryParam(c, "seen")
	if err != nil {
		return c.JSON(http.StatusBadRequest, model.ErrorResponse{
			Message: err.Error(),
		})
	}

	// Extract and validate the sort_dir query parameter.
	defaultSortOrder := query.SortOrderDescending
	sortOrderParam := query.NewSortOrderParam(&defaultSortOrder)
	err = query.ValidateParseableParam(c, "sort_dir", sortOrderParam)
	if err != nil {
		return c.JSON(http.StatusBadRequest, model.ErrorResponse{
			Message: err.Error(),
		})
	}

	// Extract and validate the sort_field query parameter.
	defaultSortField := query.V1ListingSortFieldTimestamp
	sortFieldParam := query.NewV1ListingSortFieldParam(&defaultSortField)
	err = query.ValidateParseableParam(c, "sort_field", sortFieldParam)
	if err != nil {
		return c.JSON(http.StatusBadRequest, model.ErrorResponse{
			Message: err.Error(),
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

	// Start a transaction.
	tx, err := a.DB.Begin()
	if err != nil {
		a.Echo.Logger.Error(err)
		return err
	}
	defer tx.Rollback()

	// Obtain the listing.
	params := &db.V1NotificationListingParameters{
		User:             user,
		Limit:            limit,
		Offset:           offset,
		Seen:             seen,
		SortOrder:        *(sortOrderParam.GetValue().(*query.SortOrder)),
		SortField:        *(sortFieldParam.GetValue().(*query.V1ListingSortField)),
		NotificationType: notificationType,
	}
	listing, err := db.V1ListNotifications(ctx, tx, params)
	if err != nil {
		a.Echo.Logger.Error(err)
		return err
	}

	return c.JSON(http.StatusOK, listing)
}

// GetUnseenMessagesHandler handles requests for listing messages that haven't been marked as seen yet for a
// user.
func (a *API) GetUnseenMessagesHandler(c echo.Context) error {
	var err error
	ctx := c.Request().Context()

	// Extract and validate the user query parameter.
	user, err := query.ValidatedQueryParam(c, "user", "required")
	if err != nil {
		return c.JSON(http.StatusBadRequest, model.ErrorResponse{
			Message: "missing required query parameter: user",
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
	seen := false
	params := &db.V1NotificationListingParameters{
		User:             user,
		Limit:            0,
		Offset:           0,
		Seen:             &seen,
		SortOrder:        query.SortOrderAscending,
		SortField:        query.V1ListingSortFieldTimestamp,
		NotificationType: "",
	}
	listing, err := db.V1ListNotifications(ctx, tx, params)
	if err != nil {
		a.Echo.Logger.Error(err)
		return err
	}

	return c.JSON(http.StatusOK, listing)
}

// CountMessagesHandler handles requests for counting notification messages.
func (a *API) CountMessagesHandler(c echo.Context) error {
	var err error
	ctx := c.Request().Context()

	// Extract and validate the user query parameter.
	user, err := query.ValidatedQueryParam(c, "user", "required")
	if err != nil {
		return c.JSON(http.StatusBadRequest, model.ErrorResponse{
			Message: "missing required query parameter: user",
		})
	}

	// Extract and validate the seen query parameter.
	seen, err := query.ValidateBoolPQueryParam(c, "seen")
	if err != nil {
		return c.JSON(http.StatusBadRequest, model.ErrorResponse{
			Message: err.Error(),
		})
	}

	// Extract and reformat the notification type filter.
	notificationType := strings.ReplaceAll(strings.ToLower(c.QueryParam("filter")), " ", "_")

	// Start a transaction.
	tx, err := a.DB.Begin()
	if err != nil {
		a.Echo.Logger.Error(err)
		return err
	}
	defer tx.Rollback()

	// Obtain the counts.
	params := &db.V1NotificationCountingParameters{
		User:             user,
		Seen:             seen,
		NotificationType: notificationType,
	}
	result, err := db.V1CountNotifications(ctx, tx, params)
	if err != nil {
		a.Echo.Logger.Error(err)
		return err
	}

	return c.JSON(http.StatusOK, result)
}
