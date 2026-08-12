package mailer

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// fakeSender records sent messages, or fails every send when err is set.
type fakeSender struct {
	sent []*FormattedEmailRequest
	err  error
}

func (f *fakeSender) Send(_ context.Context, req *FormattedEmailRequest) error {
	if f.err != nil {
		return f.err
	}
	f.sent = append(f.sent, req)
	return nil
}

func testDESettings() DESettings {
	return DESettings{
		Base:        "https://de.example.org",
		Data:        "/data/ds",
		Analyses:    "/analyses",
		Teams:       "/teams",
		Tools:       "/tools",
		Collections: "/collections",
		Apps:        "/apps",
		Admin:       "/admin",
		DOI:         "/doi",
		VICE:        "/vice",
	}
}

// useRepoTemplates points the working directory at the repo root, where the templates the
// image ships live. Template paths are resolved relative to the working directory, which is
// the package directory when tests run.
func useRepoTemplates(t *testing.T) {
	t.Helper()
	t.Chdir("..")
}

func TestProcess(t *testing.T) {
	sendFailure := errors.New("smtp is down")

	tests := []struct {
		name         string
		body         string
		senderErr    error
		wantCode     int   // expected HTTP error code; 0 means no error expected
		wantErrIs    error // expected wrapped error, if any
		wantFrom     string
		wantBodyPart string
	}{
		{
			// The exact shape the recorder publishes to the email_requests queue.
			name:         "recorder shaped payload",
			body:         `{"template":"blank","subject":"test subject","to":"user@example.org","values":{"contents":"hello from amqp"}}`,
			wantFrom:     "noreply@example.org",
			wantBodyPart: "hello from amqp",
		},
		{
			name:     "explicit from address respected",
			body:     `{"template":"blank","subject":"s","to":"user@example.org","fromaddr":"sender@example.org","values":{"contents":"x"}}`,
			wantFrom: "sender@example.org",
		},
		{
			name:     "invalid request body",
			body:     `{not json`,
			wantCode: 400,
		},
		{
			name:     "missing template values",
			body:     `{"template":"blank","subject":"s","to":"user@example.org"}`,
			wantCode: 400,
		},
		{
			name:     "non-object template values",
			body:     `{"template":"blank","subject":"s","to":"user@example.org","values":"not an object"}`,
			wantCode: 400,
		},
		{
			name:     "unknown template",
			body:     `{"template":"no_such_template","subject":"s","to":"user@example.org","values":{}}`,
			wantCode: 400,
		},
		{
			// An omitted recipient used to reach the SMTP dialer as a single empty address,
			// which reported the missing field as a server fault.
			name:     "missing destination address",
			body:     `{"template":"blank","subject":"s","values":{"contents":"x"}}`,
			wantCode: 400,
		},
		{
			name:      "send failure propagates",
			body:      `{"template":"blank","subject":"s","to":"user@example.org","values":{"contents":"x"}}`,
			senderErr: sendFailure,
			wantErrIs: sendFailure,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			useRepoTemplates(t)

			sender := &fakeSender{err: tt.senderErr}
			processor := NewEmailProcessor(sender, testDESettings(), "noreply@example.org")

			err := processor.Process(context.Background(), []byte(tt.body))

			if tt.wantCode != 0 {
				if err == nil {
					t.Fatalf("expected an error with code %d, got nil", tt.wantCode)
				}
				if got := ErrorCode(err); got != tt.wantCode {
					t.Fatalf("expected error code %d, got %d (%s)", tt.wantCode, got, err)
				}
				if len(sender.sent) != 0 {
					t.Fatalf("expected nothing to be sent, got %d messages", len(sender.sent))
				}
				return
			}
			if tt.wantErrIs != nil {
				if !errors.Is(err, tt.wantErrIs) {
					t.Fatalf("expected error wrapping %q, got %v", tt.wantErrIs, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %s", err)
			}
			if len(sender.sent) != 1 {
				t.Fatalf("expected 1 sent message, got %d", len(sender.sent))
			}
			sent := sender.sent[0]
			if tt.wantFrom != "" && sent.From != tt.wantFrom {
				t.Errorf("expected From %q, got %q", tt.wantFrom, sent.From)
			}
			if tt.wantBodyPart != "" && !strings.Contains(sent.Body, tt.wantBodyPart) {
				t.Errorf("expected body to contain %q, got %q", tt.wantBodyPart, sent.Body)
			}
			if len(sent.To) != 1 || sent.To[0] != "user@example.org" {
				t.Errorf("expected To [user@example.org], got %v", sent.To)
			}
		})
	}
}
