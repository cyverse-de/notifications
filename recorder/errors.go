package recorder

import "fmt"

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
