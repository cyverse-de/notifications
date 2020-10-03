package db

import (
	"database/sql"

	"github.com/cyverse-de/notifications/query"
)

// ascendingBoundaryFinder can be used to find boundary IDs when neither the before-id nor the after-id query
// parameters were specified and the sort order is ascending.
type ascendingBoundaryFinder struct {
	tx     *sql.Tx
	params *V2NotificationListingParameters
}

// newAscendingBoundaryFinder returns a new ascendingBoundaryFinder instance.
func newAscendingBoundaryFinder(tx *sql.Tx, params *V2NotificationListingParameters) *ascendingBoundaryFinder {
	return &ascendingBoundaryFinder{
		tx:     tx,
		params: params,
	}
}

// getAfterID obtains the identifier to return in the `after_id` field of the response body.
func (finder *ascendingBoundaryFinder) getAfterID() (string, error) {
	if finder.params.Limit == 0 {
		return "", nil
	}
	params := &runBoundaryIDQueryParams{
		Tx:            finder.tx,
		ListingParams: finder.params,
		SortOrder:     query.SortOrderAscending,
		Offset:        finder.params.Limit,
	}
	return runBoundaryIDQuery(params)
}

// GetBoundaryIDs obtains the IDs of the messages just beyond the boundaries of the current page.
func (finder *ascendingBoundaryFinder) GetBoundaryIDs() (string, string, error) {

	// The identifier to return in the `before_id` field is always empty in this case.
	beforeID := ""

	// Determine the identifier to return in the `after_id` field.
	afterID, err := finder.getAfterID()
	if err != nil {
		return "", "", err
	}

	return beforeID, afterID, err
}
