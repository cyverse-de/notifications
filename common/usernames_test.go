package common

import "testing"

func TestUserSuffixQualify(t *testing.T) {
	const domain = "iplantcollaborative.org"

	tests := []struct {
		name     string
		username string
		expected string
	}{
		{
			name:     "bare username",
			username: "jdoe",
			expected: "jdoe@iplantcollaborative.org",
		},
		{
			// The DE truncates before appending, so a username that is already an
			// address resolves to the same account every other service resolves it
			// to rather than to a third spelling.
			name:     "username that is already an address",
			username: "stephen.wright@utoronto.ca",
			expected: "stephen.wright@iplantcollaborative.org",
		},
		{
			name:     "username already carrying the DE domain",
			username: "jdoe@iplantcollaborative.org",
			expected: "jdoe@iplantcollaborative.org",
		},
		{
			name:     "username carrying the malformed doubled domain",
			username: "jdoe@@iplantcollaborative.org",
			expected: "jdoe@iplantcollaborative.org",
		},
		{
			name:     "truncation stops at the first at-sign",
			username: "a@b@c",
			expected: "a@iplantcollaborative.org",
		},
		{
			// An iplant-groups subject ID, which terrain sends as the user for team
			// notifications. Degenerate either way; recorded so the behavior is not
			// accidentally relied upon.
			name:     "leading at-sign leaves an empty local part",
			username: "@grouper-0123",
			expected: "@iplantcollaborative.org",
		},
		{
			name:     "empty username",
			username: "",
			expected: "@iplantcollaborative.org",
		},
		{
			name:     "case is preserved",
			username: "JDoe",
			expected: "JDoe@iplantcollaborative.org",
		},
	}

	suffix := NewUserSuffix(domain)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := suffix.Qualify(tt.username)
			if actual != tt.expected {
				t.Errorf("Qualify(%q) = %q, want %q", tt.username, actual, tt.expected)
			}
		})
	}
}

func TestUserSuffixQualifyIsIdempotent(t *testing.T) {
	suffix := NewUserSuffix("iplantcollaborative.org")

	for _, username := range []string{"jdoe", "stephen.wright@utoronto.ca", "jdoe@@iplantcollaborative.org"} {
		once := suffix.Qualify(username)
		twice := suffix.Qualify(once)
		if once != twice {
			t.Errorf("Qualify is not idempotent for %q: %q then %q", username, once, twice)
		}
	}
}

func TestNewUserSuffixNormalizesTheDomain(t *testing.T) {
	// The configuration setting is a bare domain, but tolerate an operator who
	// includes the separator rather than silently producing "jdoe@@domain".
	for _, domain := range []string{"iplantcollaborative.org", "@iplantcollaborative.org"} {
		if actual := NewUserSuffix(domain).Qualify("jdoe"); actual != "jdoe@iplantcollaborative.org" {
			t.Errorf("NewUserSuffix(%q).Qualify(\"jdoe\") = %q", domain, actual)
		}
	}
}
