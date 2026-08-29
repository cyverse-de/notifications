package mailer

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestNewDialerRejectsContradictorySettings checks that misconfiguration is reported when the
// service starts rather than at the first delivery attempt, the way validateConfig already
// reports missing settings.
func TestNewDialerRejectsContradictorySettings(t *testing.T) {
	tests := []struct {
		name     string
		settings SMTPSettings
		wantIn   string
	}{
		{
			name:     "both TLS modes",
			settings: SMTPSettings{Host: "relay.example.org", Port: 25, UseTLS: true, UseSSL: true},
			wantIn:   "mutually exclusive",
		},
		{
			name:     "user without password",
			settings: SMTPSettings{Host: "relay.example.org", Port: 25, User: "someone"},
			wantIn:   "must be set together",
		},
		{
			name:     "password without user",
			settings: SMTPSettings{Host: "relay.example.org", Port: 25, Password: "secret"},
			wantIn:   "must be set together",
		},
		{
			name: "skip verify with a CA file",
			settings: SMTPSettings{
				Host: "relay.example.org", Port: 25, InsecureSkipVerify: true, CACertFile: "/dev/null",
			},
			wantIn: "contradictory",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dialer, err := NewDialer(tt.settings)
			if err == nil {
				t.Fatalf("expected an error, got a dialer: %+v", dialer)
			}
			if !strings.Contains(err.Error(), tt.wantIn) {
				t.Errorf("expected the error to mention %q, got: %s", tt.wantIn, err)
			}
		})
	}
}

// TestNewDialerAcceptsValidSettings checks the combinations a real deployment uses.
func TestNewDialerAcceptsValidSettings(t *testing.T) {
	tests := []struct {
		name     string
		settings SMTPSettings
	}{
		{"defaults", SMTPSettings{Host: "relay.example.org", Port: 25}},
		{"starttls with credentials", SMTPSettings{
			Host: "relay.example.org", Port: 587, User: "u", Password: "p", UseTLS: true,
		}},
		{"implicit TLS", SMTPSettings{Host: "relay.example.org", Port: 465, UseSSL: true}},
		{"skip verify", SMTPSettings{Host: "relay.example.org", Port: 25, UseTLS: true, InsecureSkipVerify: true}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := NewDialer(tt.settings); err != nil {
				t.Fatalf("expected the settings to be accepted, got: %s", err)
			}
		})
	}
}

// TestNewDialerRejectsUnusableCACertFile checks that a bad trust anchor is caught at startup.
// The happy path is covered end-to-end against a real TLS relay in the TLS tests.
func TestNewDialerRejectsUnusableCACertFile(t *testing.T) {
	garbage := filepath.Join(t.TempDir(), "garbage.pem")
	if err := os.WriteFile(garbage, []byte("this is not a certificate\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name   string
		path   string
		wantIn string
	}{
		{"missing file", filepath.Join(t.TempDir(), "absent.pem"), "reading"},
		{"no PEM in file", garbage, "contains no PEM certificates"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewDialer(SMTPSettings{Host: "relay.example.org", Port: 25, CACertFile: tt.path})
			if err == nil {
				t.Fatal("expected an error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantIn) {
				t.Errorf("expected the error to mention %q, got: %s", tt.wantIn, err)
			}
		})
	}
}

// TestLocalHostnameFallsBackToServiceName checks that a failed hostname lookup does not fall
// back to "localhost". Sending "localhost" in the HELO is a strong spam signal, which is the
// thing the hostname default exists to avoid.
func TestLocalHostnameFallsBackToServiceName(t *testing.T) {
	tests := []struct {
		name   string
		stub   func() (string, error)
		expect string
	}{
		{"lookup fails", func() (string, error) { return "", errors.New("nope") }, defaultLocalName},
		{"lookup returns empty", func() (string, error) { return "", nil }, defaultLocalName},
		{"lookup succeeds", func() (string, error) { return "mail.example.org", nil }, "mail.example.org"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			original := osHostname
			t.Cleanup(func() { osHostname = original })
			osHostname = tt.stub

			if got := localHostname(); got != tt.expect {
				t.Errorf("expected %q, got %q", tt.expect, got)
			}
		})
	}
}

// TestNewDialerLocalNameDefault checks that the configured HELO name wins over the hostname
// and that the hostname is used when no name is configured.
func TestNewDialerLocalNameDefault(t *testing.T) {
	original := osHostname
	t.Cleanup(func() { osHostname = original })
	osHostname = func() (string, error) { return "pod-abc123", nil }

	dialer, err := NewDialer(SMTPSettings{Host: "relay.example.org", Port: 25})
	if err != nil {
		t.Fatal(err)
	}
	if dialer.localName != "pod-abc123" {
		t.Errorf("expected the hostname to be the default HELO name, got %q", dialer.localName)
	}

	dialer, err = NewDialer(SMTPSettings{Host: "relay.example.org", Port: 25, LocalName: "mail.example.org"})
	if err != nil {
		t.Fatal(err)
	}
	if dialer.localName != "mail.example.org" {
		t.Errorf("expected the configured HELO name to win, got %q", dialer.localName)
	}
}
