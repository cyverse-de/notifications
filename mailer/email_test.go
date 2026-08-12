package mailer

import (
	"context"
	"fmt"
	"testing"
)

// TestSendRejectsInvalidRequests checks that a request the sender refuses to build a message from
// is reported as the caller's fault. These all return before the SMTP dial, so no relay is needed.
func TestSendRejectsInvalidRequests(t *testing.T) {
	tests := []struct {
		name string
		req  *FormattedEmailRequest
	}{
		{
			name: "no recipients",
			req:  &FormattedEmailRequest{Subject: "s", Body: "b"},
		},
		{
			name: "no subject",
			req:  &FormattedEmailRequest{To: []string{"user@example.org"}, Body: "b"},
		},
		{
			name: "no body",
			req:  &FormattedEmailRequest{To: []string{"user@example.org"}, Subject: "s"},
		},
	}

	client := NewEmailClient("smtp.example.org", "noreply@example.org")

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := client.Send(context.Background(), tt.req)
			if err == nil {
				t.Fatal("expected an error, got nil")
			}
			if got := ErrorCode(err); got != 400 {
				t.Errorf("expected error code 400, got %d (%s)", got, err)
			}

			// Process wraps the sender's error, so the classification has to survive that.
			wrapped := fmt.Errorf("failed to send email to %s: %w", "user@example.org", err)
			if got := ErrorCode(wrapped); got != 400 {
				t.Errorf("expected wrapped error code 400, got %d (%s)", got, wrapped)
			}
		})
	}
}
