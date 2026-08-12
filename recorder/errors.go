package recorder

import (
	"fmt"

	"github.com/cyverse-de/notifications/db"
)

// RecoverableError is an error that is explicitly marked as recoverable.
type RecoverableError struct {
	message string
}

// Error returns the error message for a RecoverableError.
func (e RecoverableError) Error() string {
	return e.message
}

// NewRecoverableError returns a new error that is marked as being recoverable.
func NewRecoverableError(formatString string, a ...interface{}) RecoverableError {
	return RecoverableError{message: fmt.Sprintf(formatString, a...)}
}

// UnrecoverableError is an error that we do not expect to be able to recover from.
type UnrecoverableError struct {
	message string
}

// Error returns the error message for an UnrecoverableError.
func (e UnrecoverableError) Error() string {
	return e.message
}

// NewUnrecoverableError returns a new error that is marked as being unrecoverable.
func NewUnrecoverableError(formatString string, a ...interface{}) UnrecoverableError {
	return UnrecoverableError{message: fmt.Sprintf(formatString, a...)}
}

// classifyDatabaseError marks a database failure as unrecoverable only when a retry can't fix it.
// A database failure that might be transient has to stay recoverable, because discarding the
// delivery loses the notification for good.
func classifyDatabaseError(err error, wrapMsg string) error {
	if db.IsPermanentError(err) {
		return NewUnrecoverableError("%s: %s", wrapMsg, err.Error())
	}
	return NewRecoverableError("%s: %s", wrapMsg, err.Error())
}
