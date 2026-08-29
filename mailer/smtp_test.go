package mailer

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
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

// fakeSMTPServer accepts one SMTP session on a local port and records the commands and the
// message body it receives. Ported from portal-conductor's emailsvc tests and extended with a
// configurable ESMTP capability list so that tests can offer or withhold STARTTLS and AUTH.
type fakeSMTPServer struct {
	listener   net.Listener
	extensions []string
	commands   chan string
	data       chan string
}

// newFakeSMTPServer starts a cleartext relay advertising the given ESMTP extensions.
func newFakeSMTPServer(t *testing.T, extensions ...string) *fakeSMTPServer {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })

	server := &fakeSMTPServer{
		listener:   listener,
		extensions: extensions,
		commands:   make(chan string, 64),
		data:       make(chan string, 1),
	}
	go server.serve()

	return server
}

func (s *fakeSMTPServer) port() int {
	return s.listener.Addr().(*net.TCPAddr).Port
}

func (s *fakeSMTPServer) serve() {
	conn, err := s.listener.Accept()
	if err != nil {
		return
	}
	defer conn.Close() //nolint:errcheck
	s.session(conn)
}

// session speaks just enough SMTP to satisfy net/smtp. conn is reassigned in place when the
// client issues STARTTLS, and the write closure reads it through the variable, so both keep
// using the upgraded connection afterwards.
func (s *fakeSMTPServer) session(conn net.Conn) {
	write := func(format string, args ...any) {
		fmt.Fprintf(conn, format+"\r\n", args...) //nolint:errcheck
	}
	write("220 fake.test ESMTP")

	reader := bufio.NewReader(conn)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return
		}
		line = strings.TrimRight(line, "\r\n")
		s.commands <- line

		switch strings.ToUpper(strings.SplitN(line, " ", 2)[0]) {
		case "EHLO":
			s.writeCapabilities(write)
		case "HELO":
			write("250 fake.test")
		case "AUTH":
			write("235 authenticated")
		case "MAIL", "RCPT":
			write("250 OK")
		case "DATA":
			write("354 go ahead")
			var body strings.Builder
			for {
				dataLine, err := reader.ReadString('\n')
				if err != nil {
					return
				}
				if strings.TrimRight(dataLine, "\r\n") == "." {
					break
				}
				body.WriteString(dataLine)
			}
			s.data <- body.String()
			write("250 accepted")
		case "QUIT":
			write("221 bye")
			return
		default:
			write("250 OK")
		}
	}
}

// writeCapabilities emits the EHLO reply. Every line of a multiline SMTP reply but the last is
// marked with a hyphen instead of a space.
func (s *fakeSMTPServer) writeCapabilities(write func(string, ...any)) {
	if len(s.extensions) == 0 {
		write("250 fake.test")
		return
	}
	write("250-fake.test")
	for _, extension := range s.extensions[:len(s.extensions)-1] {
		write("250-%s", extension)
	}
	write("250 %s", s.extensions[len(s.extensions)-1])
}

// collectCommands drains the recorded commands until QUIT, or until the server stops sending.
func (s *fakeSMTPServer) collectCommands(t *testing.T) []string {
	t.Helper()

	var commands []string
	for {
		select {
		case command := <-s.commands:
			commands = append(commands, command)
			if strings.HasPrefix(strings.ToUpper(command), "QUIT") {
				return commands
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("timed out waiting for QUIT; got: %v", commands)
		}
	}
}

// testMessage is a minimal io.WriterTo standing in for a gomail message.
type testMessage string

func (m testMessage) WriteTo(w io.Writer) (int64, error) {
	n, err := io.WriteString(w, string(m))
	return int64(n), err
}

// TestSendOverCleartext checks the default configuration end to end: the hostname is used in
// the HELO, and the envelope and body reach the relay intact.
func TestSendOverCleartext(t *testing.T) {
	original := osHostname
	t.Cleanup(func() { osHostname = original })
	osHostname = func() (string, error) { return "mail.example.org", nil }

	server := newFakeSMTPServer(t)
	dialer, err := NewDialer(SMTPSettings{Host: "127.0.0.1", Port: server.port()})
	if err != nil {
		t.Fatal(err)
	}

	recipients := []string{"first@example.org", "second@example.org"}
	err = dialer.send("noreply@example.org", recipients, testMessage("Subject: hi\r\n\r\nbody\r\n"))
	if err != nil {
		t.Fatalf("send failed: %s", err)
	}

	commands := server.collectCommands(t)
	expected := []string{
		"EHLO mail.example.org",
		"MAIL FROM:<noreply@example.org>",
		"RCPT TO:<first@example.org>",
		"RCPT TO:<second@example.org>",
		"DATA",
		"QUIT",
	}
	for _, want := range expected {
		if !slices.ContainsFunc(commands, func(got string) bool { return strings.HasPrefix(got, want) }) {
			t.Errorf("expected a command starting with %q; got: %v", want, commands)
		}
	}

	if body := <-server.data; !strings.Contains(body, "Subject: hi") {
		t.Errorf("delivered message missing its subject:\n%s", body)
	}
}

// TestSendUsesConfiguredLocalName checks that an explicitly configured HELO name is used in
// place of the hostname, which is how a deployment satisfies a relay that demands an FQDN.
func TestSendUsesConfiguredLocalName(t *testing.T) {
	server := newFakeSMTPServer(t)
	dialer, err := NewDialer(SMTPSettings{
		Host: "127.0.0.1", Port: server.port(), LocalName: "notifications.example.org",
	})
	if err != nil {
		t.Fatal(err)
	}

	err = dialer.send("noreply@example.org", []string{"someone@example.org"}, testMessage("body\r\n"))
	if err != nil {
		t.Fatalf("send failed: %s", err)
	}

	if ehlo := <-server.commands; ehlo != "EHLO notifications.example.org" {
		t.Errorf("expected the configured HELO name, got %q", ehlo)
	}
}

// TestSendReportsAnUnreachableRelay checks that a connection failure names the relay, since
// that address is the first thing an operator needs when delivery stops.
func TestSendReportsAnUnreachableRelay(t *testing.T) {
	// Port 1 on the loopback interface refuses connections without a listener.
	dialer, err := NewDialer(SMTPSettings{Host: "127.0.0.1", Port: 1})
	if err != nil {
		t.Fatal(err)
	}

	err = dialer.send("noreply@example.org", []string{"someone@example.org"}, testMessage("body\r\n"))
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "127.0.0.1:1") {
		t.Errorf("expected the error to name the relay address, got: %s", err)
	}
	// A relay that is down is not the caller's fault, so it must not be classified as a 4xx.
	if code := ErrorCode(err); code != 500 {
		t.Errorf("expected a transport failure to be a 500, got %d", code)
	}
}
