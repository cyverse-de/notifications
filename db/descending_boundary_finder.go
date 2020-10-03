package db

import (
	"database/sql"

	"github.com/cyverse-de/notifications/query"
)

// descendingBoundaryFinder can be used to find boundary IDs when neither the before-id nor the after-id query
// parameters were specified and the sort order is descending.
type descendingBoundaryFinder struct {
	tx     *sql.Tx
	params *V2NotificationListingParameters
}

// newDescendingBoundaryFinder returns a new descendingBoundaryFinder instance.
func newDescendingBoundaryFinder(tx *sql.Tx, params *V2NotificationListingParameters) *descendingBoundaryFinder {
	return &descendingBoundaryFinder{
		tx:     tx,
		params: params,
	}
}

// getBeforeID obtains the identifier to return in the `before_id` field of the response body.
func (finder *descendingBoundaryFinder) getBeforeID() (string, error) {
	if finder.params.Limit == 0 {
		return "", nil
	}
	params := &runBoundaryIDQueryParams{
		Tx:            finder.tx,
		ListingParams: finder.params,
		SortOrder:     query.SortOrderDescending,
		Offset:        finder.params.Limit,
	}
	return runBoundaryIDQuery(params)
}

// GetBoundaryIDs obtains the IDs of the messages just beyond the boundaries of the current page.
func (finder *descendingBoundaryFinder) GetBoundaryIDs() (string, string, error) {

	// Determine the identifier to return in the `before_id` field.
	beforeID, err := finder.getBeforeID()
	if err != nil {
		return "", "", err
	}

	// The identifier to return in the `after_id` field is always empty in this case.
	afterID := ""

	return beforeID, afterID, err
}
