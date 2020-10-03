package db

import (
	"database/sql"

	"github.com/cyverse-de/notifications/query"
)

// afterIDBoundaryFinder can be used to find boundary IDs when the after-id parameter was specified in the request.
type afterIDBoundaryFinder struct {
	tx     *sql.Tx
	params *V2NotificationListingParameters
}

// newAfterIDBoundaryFinder returns a new afterIDBoundaryFinder instance.
func newAfterIDBoundaryFinder(tx *sql.Tx, params *V2NotificationListingParameters) *afterIDBoundaryFinder {
	return &afterIDBoundaryFinder{
		tx:     tx,
		params: params,
	}
}

// getRunBoundaryIDQueryParams returns the parameters to pass to runBoundaryIDQuery. The only parameters that need
// to vary in this finder are SortOrder and Offset.
func (finder *afterIDBoundaryFinder) getRunBoundaryIDQueryParams(
	sortOrder query.SortOrder,
	offset uint64,
) *runBoundaryIDQueryParams {
	return &runBoundaryIDQueryParams{
		Tx:                  finder.tx,
		ListingParams:       finder.params,
		ComparisonID:        finder.params.AfterID,
		ComparisonTimestamp: finder.params.AfterTimestamp,
		SortOrder:           sortOrder,
		Offset:              offset,
	}
}

// getBeforeID obtains the identifier to return in the `before_id` field of the response body. In this case, we only
// need to return the ID of the message that comes just before the boundary message from the incoming request.
func (finder *afterIDBoundaryFinder) getBeforeID() (string, error) {
	return runBoundaryIDQuery(finder.getRunBoundaryIDQueryParams(query.SortOrderDescending, 1))
}

// getAfterID obtains the identifier to return in the `after_id` field of the response body. There are two cases to
// take into account here. The first case is where no limit is specified. In this case, the newest matching message
// must be in the current page because we're querying in ascending order and applying no limit. The second case is
// where a limit was specified. In this case, we need to run a query to get the appropriate ID, which may still be
// empty.
func (finder *afterIDBoundaryFinder) getAfterID() (string, error) {
	if finder.params.Limit == 0 {
		return "", nil
	}
	return runBoundaryIDQuery(finder.getRunBoundaryIDQueryParams(query.SortOrderAscending, finder.params.Limit))
}

// GetBoundaryIDs obtains the IDs of the messages just beyond the boundaries of the current page.
func (finder *afterIDBoundaryFinder) GetBoundaryIDs() (string, string, error) {

	// Determine the identifier to return in the `before_id` field.
	beforeID, err := finder.getBeforeID()
	if err != nil {
		return "", "", err
	}

	// Determine the identifier to return in the `after_id` field.
	afterID, err := finder.getAfterID()
	if err != nil {
		return "", "", err
	}

	return beforeID, afterID, err
}
