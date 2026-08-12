package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cyverse-de/notifications/mailer"
	"github.com/labstack/echo/v4"
)

// fakeSender records sent messages, or fails every send when err is set.
type fakeSender struct {
	sent []*mailer.FormattedEmailRequest
	err  error
}

func (f *fakeSender) Send(_ context.Context, req *mailer.FormattedEmailRequest) error {
	if f.err != nil {
		return f.err
	}
	f.sent = append(f.sent, req)
	return nil
}

func TestEmailRequestHandler(t *testing.T) {
	// Template paths resolve against the working directory, which is the package directory
	// when tests run; the templates the image ships live at the repo root.
	t.Chdir("..")

	validBody := `{"template":"blank","subject":"s","to":"user@example.org","values":{"contents":"x"}}`

	tests := []struct {
		name         string
		body         string
		senderErr    error
		wantStatus   int
		wantBodyPart string
		rejectPart   string
	}{
		{
			name:         "valid request",
			body:         validBody,
			wantStatus:   http.StatusOK,
			wantBodyPart: `"success":true`,
		},
		{
			name:       "invalid JSON",
			body:       `{not json`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "unknown template",
			body:       `{"template":"no_such_template","subject":"s","to":"user@example.org","values":{}}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			// Regression test: 500 bodies must stay generic, because SMTP dial errors carry
			// internal host details.
			name:         "failing sender",
			body:         validBody,
			senderErr:    errors.New("dial tcp 10.0.0.1:25: connection refused"),
			wantStatus:   http.StatusInternalServerError,
			wantBodyPart: "see the notifications logs",
			rejectPart:   "10.0.0.1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := echo.New()
			sender := &fakeSender{err: tt.senderErr}
			a := API{
				Echo:   e,
				Mailer: mailer.NewEmailProcessor(sender, mailer.DESettings{Base: "https://de.example.org"}, "noreply@example.org"),
			}

			req := httptest.NewRequest(http.MethodPost, "/mail", strings.NewReader(tt.body))
			rec := httptest.NewRecorder()
			ctx := e.NewContext(req, rec)

			if err := a.EmailRequestHandler(ctx); err != nil {
				t.Fatalf("handler returned an error: %s", err)
			}

			if rec.Code != tt.wantStatus {
				t.Fatalf("expected status %d, got %d (%s)", tt.wantStatus, rec.Code, rec.Body.String())
			}
			if tt.wantBodyPart != "" && !strings.Contains(rec.Body.String(), tt.wantBodyPart) {
				t.Errorf("expected body to contain %q, got %q", tt.wantBodyPart, rec.Body.String())
			}
			if tt.rejectPart != "" && strings.Contains(rec.Body.String(), tt.rejectPart) {
				t.Errorf("expected body to not leak %q, got %q", tt.rejectPart, rec.Body.String())
			}
		})
	}
}
