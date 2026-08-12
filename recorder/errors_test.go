package recorder

import (
	"testing"

	"github.com/lib/pq"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
)

func TestRecoverableError(t *testing.T) {
	var err error = NewRecoverableError("this is a test %s", "of the Emergency Broadcast System")

	// Verify that we go the expected error message.
	if err.Error() != "this is a test of the Emergency Broadcast System" {
		t.Errorf("unexpected error message: %s", err.Error())
	}

	// Verify that a RecoverableError was actually returned.
	_, ok := err.(RecoverableError)
	if !ok {
		t.Errorf("The error doesn't appear to be a RecoverableError")
	}

	// The type must be distinct from an uncrecoverable error.
	_, ok = err.(UnrecoverableError)
	if ok {
		t.Errorf("The error appears to be an UnrecoverableError")
	}
}

func TestUnrecoverableError(t *testing.T) {
	var err error = NewUnrecoverableError("testing %s %s", "check", "1...2...3")

	// Verify that w get the expected error message.
	if err.Error() != "testing check 1...2...3" {
		t.Errorf("unexpected error message: %s", err.Error())
	}

	// Verify that an UnrecoverableError was actually returned.
	_, ok := err.(UnrecoverableError)
	if !ok {
		t.Errorf("The error doesn't appear to be an UnrecoverableError")
	}

	// The type must be distinct from a RecoverableError
	_, ok = err.(RecoverableError)
	if ok {
		t.Errorf("The error appears to be a RecoverableError")
	}
}

func TestClassifyDatabaseError(t *testing.T) {
	tests := []struct {
		name            string
		err             error
		wantRecoverable bool
	}{
		{
			name:            "a lost connection is recoverable",
			err:             &pq.Error{Code: "08006", Message: "connection failure"},
			wantRecoverable: true,
		},
		{
			name:            "a deadlock is recoverable",
			err:             &pq.Error{Code: "40P01", Message: "deadlock detected"},
			wantRecoverable: true,
		},
		{
			name:            "an error that didn't come from PostgreSQL is recoverable",
			err:             errors.New("driver: bad connection"),
			wantRecoverable: true,
		},
		{
			name:            "a value that is too long for its column is unrecoverable",
			err:             &pq.Error{Code: "22001", Message: "value too long for type character varying(32)"},
			wantRecoverable: false,
		},
		{
			name:            "a constraint violation is unrecoverable",
			err:             &pq.Error{Code: "23505", Message: "duplicate key value violates unique constraint"},
			wantRecoverable: false,
		},
		{
			name:            "an undefined column is unrecoverable",
			err:             &pq.Error{Code: "42703", Message: "column does not exist"},
			wantRecoverable: false,
		},
		{
			name:            "a wrapped permanent error is still unrecoverable",
			err:             errors.Wrap(&pq.Error{Code: "22001", Message: "value too long"}, "unable to save notification"),
			wantRecoverable: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert := assert.New(t)

			err := classifyDatabaseError(tt.err, "unable to save the notification")
			assert.ErrorContains(err, "unable to save the notification", "the error must say what failed")

			var recoverable RecoverableError
			var unrecoverable UnrecoverableError
			if tt.wantRecoverable {
				assert.ErrorAs(err, &recoverable, "a failure that a retry might fix must be requeued")
			} else {
				assert.ErrorAs(err, &unrecoverable, "a failure that a retry can't fix must not be requeued")
			}
		})
	}
}
