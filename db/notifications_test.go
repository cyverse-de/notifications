package db

import (
	"context"
	"testing"

	"github.com/cyverse-de/messaging/v12"
	"github.com/stretchr/testify/assert"
)

func TestSaveOutgoingNotificationRequiresAnID(t *testing.T) {
	tests := []struct {
		name    string
		message map[string]interface{}
	}{
		{
			name:    "a message without an ID is rejected",
			message: map[string]interface{}{"text": "some job status changed"},
		},
		{
			name:    "an ID that isn't a string is rejected",
			message: map[string]interface{}{"id": 42.0},
		},
		{
			name:    "a message with no fields at all is rejected",
			message: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// The transaction is nil because the ID is validated before the database is touched;
			// this delivery must be reported as an error rather than panicking the process.
			err := SaveOutgoingNotification(
				context.Background(),
				nil,
				&messaging.NotificationMessage{Message: tt.message},
			)
			assert.Error(t, err, "an outgoing notification without a usable ID must be reported")
		})
	}
}
