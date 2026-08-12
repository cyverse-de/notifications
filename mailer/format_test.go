package mailer

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestFormatMessage(t *testing.T) {
	tests := []struct {
		name       string
		template   string
		values     string // JSON object with the template values
		wantHTML   bool
		wantCode   int // expected HTTP error code; 0 means no error expected
		wantParts  []string
		wantAbsent []string
	}{
		{
			name:     "analysis status change",
			template: "analysis_status_change",
			values: `{
				"analysisname": "my analysis",
				"analysisstatus": "Completed",
				"startdate": "1754800000000",
				"analysisresultsfolder": "/example/home/analyses/foo",
				"analysisid": "11111111-2222-3333-4444-555555555555",
				"user": "someuser"
			}`,
			wantHTML: true,
			wantParts: []string{
				"https://de.example.org/data/ds/example/home/analyses/foo",
				"https://de.example.org/analyses/11111111-2222-3333-4444-555555555555",
				"Analysis launch date:",
				"2025",
			},
		},
		{
			// A publisher that sends the start date as a JSON number used to defeat the
			// string-only extraction and render the epoch as the launch date.
			name:     "analysis status change with a numeric start date",
			template: "analysis_status_change",
			values: `{
				"analysisname": "my analysis",
				"analysisstatus": "Completed",
				"startdate": 1754800000000
			}`,
			wantHTML:   true,
			wantParts:  []string{"Analysis launch date:", "2025"},
			wantAbsent: []string{"1970"},
		},
		{
			name:     "analysis status change with an unusable start date",
			template: "analysis_status_change",
			values: `{
				"analysisname": "my analysis",
				"analysisstatus": "Completed",
				"startdate": "not a timestamp"
			}`,
			wantHTML:   true,
			wantAbsent: []string{"Analysis launch date:", "1970"},
		},
		{
			name:       "analysis status change without a start date",
			template:   "analysis_status_change",
			values:     `{"analysisname": "my analysis", "analysisstatus": "Completed"}`,
			wantHTML:   true,
			wantAbsent: []string{"Analysis launch date:", "1970"},
		},
		{
			name:       "periodic notification with an unusable start date",
			template:   "analysis_periodic_notification",
			values:     `{"analysisname": "my analysis", "analysisstatus": "Running", "startdate": {}}`,
			wantHTML:   true,
			wantAbsent: []string{"Analysis launch date:", "1970"},
		},
		{
			// Regression test: this template shipped with a malformed {{.DEToolsLink} action.
			name:      "tool request completion renders tools link",
			template:  "tool_request_completion",
			values:    `{"toolname": "mytool", "comments": "looks good"}`,
			wantHTML:  true,
			wantParts: []string{`href="https://de.example.org/tools"`, "mytool"},
		},
		{
			name:      "added to team",
			template:  "added_to_team",
			values:    `{"team_name": "myteam"}`,
			wantHTML:  true,
			wantParts: []string{"https://de.example.org/teams/myteam"},
		},
		{
			name:     "added to team without team name",
			template: "added_to_team",
			values:   `{}`,
			wantCode: 400,
		},
		{
			name:     "added to team with non-string team name",
			template: "added_to_team",
			values:   `{"team_name": 42}`,
			wantCode: 400,
		},
		{
			name:      "vice request complete",
			template:  "request_complete",
			values:    `{"request_type": "vice", "request_details": {"concurrent_jobs": 2, "intended_use": "testing"}}`,
			wantHTML:  true,
			wantParts: []string{"testing"},
		},
		{
			// A missing request_type used to panic; it must now fail cleanly rather than
			// silently sending a non-VICE email that omits the VICE details.
			name:     "request complete without request type",
			template: "request_complete",
			values:   `{}`,
			wantCode: 400,
		},
		{
			name:     "request complete with non-string request type",
			template: "request_complete",
			values:   `{"request_type": 7}`,
			wantCode: 400,
		},
		{
			name:     "vice request complete without details",
			template: "request_complete",
			values:   `{"request_type": "vice"}`,
			wantCode: 400,
		},
		{
			name:     "tool request without details",
			template: "tool_request",
			values:   `{}`,
			wantCode: 400,
		},
		{
			name:     "unknown template",
			template: "no_such_template",
			values:   `{}`,
			wantCode: 400,
		},
		{
			// The name is concatenated into a file path, so a traversal has to be rejected
			// before anything outside the template directories can be parsed and mailed.
			name:     "template name escaping the template directory",
			template: "../../etc/passwd",
			values:   `{}`,
			wantCode: 400,
		},
		{
			name:     "template name with a path separator",
			template: "html/blank",
			values:   `{}`,
			wantCode: 400,
		},
		{
			name:      "plain text template",
			template:  "blank",
			values:    `{"contents": "plain text body"}`,
			wantHTML:  false,
			wantParts: []string{"plain text body"},
		},
		{
			// The recorder asks for this template when it discards a delivery, and supplies
			// exactly these keys. The template referred to them in title case, so every
			// field rendered as "<no value>" and the notice arrived with no diagnostics.
			name:     "recorder discard notice",
			template: "notifications_event_discarded",
			values: `{
				"error": "bad payload",
				"routing_key": "events.notification.update.analysis",
				"message_body": "{\"user\":\"someuser\"}"
			}`,
			wantHTML: false,
			wantParts: []string{
				"Error: bad payload",
				"Routing Key: events.notification.update.analysis",
				`{"user":"someuser"}`,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			useRepoTemplates(t)

			payload := make(map[string]any)
			if err := json.Unmarshal([]byte(tt.values), &payload); err != nil {
				t.Fatalf("bad test values: %s", err)
			}
			emailReq := EmailRequest{Template: tt.template}

			output, isHTML, err := FormatMessage(context.Background(), emailReq, payload, testDESettings())

			if tt.wantCode != 0 {
				if err == nil {
					t.Fatalf("expected an error with code %d, got nil", tt.wantCode)
				}
				if got := ErrorCode(err); got != tt.wantCode {
					t.Fatalf("expected error code %d, got %d (%s)", tt.wantCode, got, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %s", err)
			}
			if isHTML != tt.wantHTML {
				t.Errorf("expected isHTML %v, got %v", tt.wantHTML, isHTML)
			}
			rendered := output.String()
			if rendered == "" {
				t.Fatal("expected non-empty output")
			}
			for _, part := range tt.wantParts {
				if !strings.Contains(rendered, part) {
					t.Errorf("expected output to contain %q; output:\n%s", part, rendered)
				}
			}
			for _, part := range tt.wantAbsent {
				if strings.Contains(rendered, part) {
					t.Errorf("expected output not to contain %q; output:\n%s", part, rendered)
				}
			}
		})
	}
}
