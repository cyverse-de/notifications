package db

import (
	"errors"
	"slices"

	"github.com/lib/pq"
)

// permanentErrorClasses are the SQLSTATE classes that indicate a statement will keep failing no
// matter how often it's retried: invalid data, constraint violations, and schema or permission
// problems. Failures outside of these classes, such as lost connections, deadlocks, and resource
// exhaustion, may well succeed on a retry.
var permanentErrorClasses = []pq.ErrorClass{"22", "23", "42"}

// IsPermanentError returns true if err is a database error that a retry can't fix. Errors that
// didn't come from PostgreSQL are not treated as permanent because the cause of the failure isn't
// known.
func IsPermanentError(err error) bool {
	var pqErr *pq.Error
	if !errors.As(err, &pqErr) {
		return false
	}
	return slices.Contains(permanentErrorClasses, pqErr.Code.Class())
}
