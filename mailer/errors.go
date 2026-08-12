package mailer

import (
	"errors"
	"fmt"
	"net/http"
)

// HTTPError is an error that carries the HTTP response code to report for it. The mailer
// returns one wherever a failure can be attributed to the request itself, so both the HTTP
// and AMQP transports can tell a bad request from a server-side failure.
type HTTPError struct {
	code    int
	message string
}

// Code returns the HTTP response code corresponding to the error.
func (h *HTTPError) Code() int {
	return h.code
}

// Error returns the message corresponding to the error.
func (h *HTTPError) Error() string {
	return h.message
}

// NewHTTPError returns a new HTTP error with the given status code and (optionally formatted)
// message.
func NewHTTPError(code int, format string, args ...any) *HTTPError {
	return &HTTPError{
		code:    code,
		message: fmt.Sprintf(format, args...),
	}
}

// ErrorCode returns the HTTP response code to report for an error from this package. Anything
// that isn't an *HTTPError is a server-side failure.
func ErrorCode(err error) int {
	var httpError *HTTPError
	if errors.As(err, &httpError) {
		return httpError.Code()
	}
	return http.StatusInternalServerError
}
