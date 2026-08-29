# Configurable SMTP Relay Support Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let the `notifications` service send mail through authenticated and TLS-only SMTP relays, not just the unauthenticated local exim relay it is hardcoded for today.

**Architecture:** Port the SMTP *transport* from `portal-conductor/emailsvc` into `notifications/mailer` as a new `Dialer` type, and hand the live `*smtp.Client` to `gomail` through a `gomail.SendFunc`. `gomail` keeps doing message construction — attachments, `Cc`, `Bcc` stripping, `Date`, and the html2text plain-text alternative — so none of that changes. Owning the transport is what makes `email.smtpUseTLS` able to fail closed, which gomail's opportunistic STARTTLS cannot do.

**Tech Stack:** Go 1.26, `net/smtp`, `crypto/tls`, `gopkg.in/gomail.v2`, Viper via `github.com/cyverse-de/configurate`, stdlib `testing`.

**Spec:** `docs/superpowers/specs/2026-08-28-smtp-relay-support-design.md`

## Global Constraints

- Every new configuration setting is optional. `requiredConfigKeys` in `main.go` is **not** modified.
- Configuration keys stay flat (`email.smtpUseTLS`), not nested (`email.smtp.useTLS`). The configuration file template is shared across several services.
- Defaults reproduce current behavior: port `25`, no authentication, no TLS. The one exception is the HELO name, which changes from the hardcoded `notifications` to `os.Hostname()`.
- When `os.Hostname()` fails or returns empty, fall back to `notifications` — **never** to `localhost`, which is the spam signal this work exists to avoid.
- Transport failures stay plain errors, never `*HTTPError`, so `ErrorCode` reports them as 500. Only request-validation failures are 4xx.
- Do not add dependencies. Everything needed is in the standard library or already in `go.mod`.
- After every task: `go build ./... && go test ./...` must pass from `/Users/sarahr/src/de/notifications`.

## File Structure

| File | Responsibility |
|---|---|
| `mailer/smtp.go` *(new)* | Transport only: `SMTPSettings`, `Dialer`, connection, TLS, authentication, hostname and Message-ID helpers. |
| `mailer/smtp_test.go` *(new)* | The `fakeSMTPServer` harness and all transport tests. |
| `mailer/email.go` *(modified)* | Message construction, unchanged except for using the `Dialer` and setting `Message-ID`. |
| `mailer/email_test.go` *(modified)* | One call site updated for the new `NewEmailClient` signature. |
| `main.go` *(modified)* | Reads the new settings and builds the `Dialer`. |
| `README.md` *(modified)* | Documents the new settings. |

---

### Task 1: Settings, validation, and the hostname helper

Pure configuration handling with no network involved. `mailer/smtp.go` is created here but nothing consumes it yet, so the build stays green.

**Files:**
- Create: `mailer/smtp.go`
- Create: `mailer/smtp_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces: `type SMTPSettings struct{...}`, `func NewDialer(settings SMTPSettings) (*Dialer, error)`, `func localHostname() string`, and the package-level `var osHostname = os.Hostname` test seam. Task 2 adds methods to `Dialer`; Task 5 calls `NewDialer` from `main.go`.

- [ ] **Step 1: Write the failing tests**

Create `mailer/smtp_test.go`:

```go
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
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./mailer/ -run 'TestNewDialer|TestLocalHostname' -v`
Expected: FAIL — the package does not compile, with `undefined: SMTPSettings`, `undefined: NewDialer`, `undefined: localHostname`, `undefined: osHostname`, `undefined: defaultLocalName`.

- [ ] **Step 3: Write the implementation**

Create `mailer/smtp.go`:

```go
package mailer

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"os"
)

// defaultLocalName is the HELO name used when the machine hostname isn't available. It's the
// name this service sent before the HELO name became configurable.
const defaultLocalName = "notifications"

// osHostname is a seam so that tests can exercise the hostname lookup failing.
var osHostname = os.Hostname

// SMTPSettings describes how to reach the SMTP relay. The zero value plus a host describes the
// unauthenticated cleartext relay this service talked to before these settings existed.
type SMTPSettings struct {
	Host               string
	Port               int
	User               string
	Password           string
	LocalName          string
	CACertFile         string
	UseTLS             bool
	UseSSL             bool
	InsecureSkipVerify bool
}

// Dialer opens connections to the configured SMTP relay. Everything that can fail on bad
// configuration rather than on a bad connection is resolved once, in NewDialer, so that a
// misconfigured deployment fails at startup instead of at the first delivery attempt.
type Dialer struct {
	settings  SMTPSettings
	localName string
	tlsConfig *tls.Config
}

// NewDialer validates the relay settings and resolves everything that doesn't depend on a
// connection: the HELO name and the TLS configuration, including any configured trust anchor.
func NewDialer(settings SMTPSettings) (*Dialer, error) {
	if err := settings.validate(); err != nil {
		return nil, err
	}

	tlsConfig, err := settings.buildTLSConfig()
	if err != nil {
		return nil, err
	}

	localName := settings.LocalName
	if localName == "" {
		localName = localHostname()
	}

	return &Dialer{settings: settings, localName: localName, tlsConfig: tlsConfig}, nil
}

// validate reports settings combinations that can't be honored, so that they're caught before
// anything tries to send mail.
func (s SMTPSettings) validate() error {
	if s.UseTLS && s.UseSSL {
		return errors.New(
			"email.smtpUseTLS and email.smtpUseSSL are mutually exclusive: STARTTLS upgrades a " +
				"cleartext connection, while implicit TLS starts encrypted",
		)
	}
	if (s.User == "") != (s.Password == "") {
		return errors.New("email.smtpUser and email.smtpPassword must be set together")
	}
	if s.InsecureSkipVerify && s.CACertFile != "" {
		return errors.New(
			"email.smtpInsecureSkipVerify and email.smtpCACertFile are contradictory: skipping " +
				"verification would ignore the configured certificate authority",
		)
	}
	return nil
}

// buildTLSConfig returns the TLS configuration to use for both implicit TLS and STARTTLS.
func (s SMTPSettings) buildTLSConfig() (*tls.Config, error) {
	config := &tls.Config{
		ServerName:         s.Host,
		InsecureSkipVerify: s.InsecureSkipVerify, //nolint:gosec // opt-in, for relays with private CAs
	}

	if s.CACertFile == "" {
		return config, nil
	}

	pemBytes, err := os.ReadFile(s.CACertFile)
	if err != nil {
		return nil, fmt.Errorf("reading email.smtpCACertFile %s: %w", s.CACertFile, err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(pemBytes) {
		return nil, fmt.Errorf("email.smtpCACertFile %s contains no PEM certificates", s.CACertFile)
	}
	config.RootCAs = pool

	return config, nil
}

// localHostname returns the machine hostname, for use as the HELO name and as the fallback
// domain in generated Message-IDs. Receiving MTAs score an unresolvable HELO name against the
// sender, so a hostname is preferable to a bare service name. The fallback is the service name
// rather than "localhost" because "localhost" is the value that raises the score.
func localHostname() string {
	if hostname, err := osHostname(); err == nil && hostname != "" {
		return hostname
	}
	return defaultLocalName
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./mailer/ -run 'TestNewDialer|TestLocalHostname' -v`
Expected: PASS, all subtests.

Then confirm nothing else broke: `go build ./... && go test ./...`

- [ ] **Step 5: Commit**

```bash
git add mailer/smtp.go mailer/smtp_test.go
git commit -m "Add validated SMTP relay settings to the mailer package

Resolves the HELO name and TLS configuration once, at startup, so that a
misconfigured deployment fails immediately rather than at the first delivery."
```

---

### Task 2: The fake relay and the cleartext connection

Adds the test harness every later task reuses, plus `connect` and `send` for the unencrypted case. Still nothing calls the `Dialer` in production code.

**Files:**
- Modify: `mailer/smtp.go`
- Modify: `mailer/smtp_test.go`

**Interfaces:**
- Consumes: `SMTPSettings`, `NewDialer`, `Dialer.localName`, `Dialer.tlsConfig` from Task 1.
- Produces: `func (d *Dialer) connect() (*smtp.Client, error)` and `func (d *Dialer) send(from string, to []string, msg io.WriterTo) error` — the latter has exactly the signature `gomail.SendFunc` expects, which is how Task 5 wires it in. Also produces the test helpers `newFakeSMTPServer(t *testing.T, extensions ...string) *fakeSMTPServer`, `(*fakeSMTPServer).port() int`, and its `commands`/`data` channels.

- [ ] **Step 1: Write the failing test**

Append to `mailer/smtp_test.go` (and add `bufio`, `fmt`, `io`, and `net` to its imports):

```go
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
```

Add `slices` and `time` to the test file's imports as well.

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./mailer/ -run TestSend -v`
Expected: FAIL — `d.send undefined (type *Dialer has no field or method send)`.

- [ ] **Step 3: Write the implementation**

Append to `mailer/smtp.go`, adding `io`, `net`, `net/smtp`, `strconv`, and `time` to its imports:

```go
// dialTimeout bounds the TCP connect and, for implicit TLS, the handshake. A relay that hasn't
// answered in this long isn't going to.
const dialTimeout = 30 * time.Second

// send delivers one already-built message. Its signature is what gomail.SendFunc expects, so
// that gomail keeps doing message construction while this package owns the connection.
func (d *Dialer) send(from string, to []string, msg io.WriterTo) error {
	client, err := d.connect()
	if err != nil {
		return err
	}
	defer client.Close() //nolint:errcheck

	if err := d.authenticate(client); err != nil {
		return err
	}

	if err := client.Mail(from); err != nil {
		return fmt.Errorf("sender %s rejected: %w", from, err)
	}
	for _, recipient := range to {
		if err := client.Rcpt(recipient); err != nil {
			return fmt.Errorf("recipient %s rejected: %w", recipient, err)
		}
	}

	w, err := client.Data()
	if err != nil {
		return err
	}
	if _, err := msg.WriteTo(w); err != nil {
		_ = w.Close()
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}

	return client.Quit()
}

// connect opens a connection to the relay and greets it. Implicit TLS and STARTTLS are added
// in a later step; this handles the cleartext case.
func (d *Dialer) connect() (*smtp.Client, error) {
	address := net.JoinHostPort(d.settings.Host, strconv.Itoa(d.settings.Port))

	conn, err := net.DialTimeout("tcp", address, dialTimeout)
	if err != nil {
		return nil, fmt.Errorf("connecting to SMTP server %s: %w", address, err)
	}

	client, err := smtp.NewClient(conn, d.settings.Host)
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("SMTP greeting from %s failed: %w", address, err)
	}

	return d.hello(client, address)
}

// hello sends EHLO with the configured local name. net/smtp defaults to "localhost", which
// lands in the receiving MTA's Received header and raises the message's spam score.
func (d *Dialer) hello(client *smtp.Client, address string) (*smtp.Client, error) {
	if err := client.Hello(d.localName); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("SMTP HELO with %s failed: %w", address, err)
	}
	return client, nil
}

// authenticate is a no-op until Task 4 adds credential support; sending without credentials is
// the existing behavior.
func (d *Dialer) authenticate(_ *smtp.Client) error {
	return nil
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./mailer/ -v`
Expected: PASS, including the Task 1 tests and the pre-existing `TestSendRejectsInvalidRequests`.

- [ ] **Step 5: Commit**

```bash
git add mailer/smtp.go mailer/smtp_test.go
git commit -m "Add the SMTP dialer's cleartext connection path

Includes the fake relay harness ported from portal-conductor's emailsvc
tests, extended with a configurable ESMTP capability list."
```

---

### Task 3: TLS modes

Implicit TLS, and STARTTLS that fails closed. This is the behavior gomail's dialer cannot provide and the reason the transport is being ported at all.

**Files:**
- Modify: `mailer/smtp.go`
- Modify: `mailer/smtp_test.go`

**Interfaces:**
- Consumes: `Dialer.connect`, `Dialer.hello`, `Dialer.tlsConfig`, `newFakeSMTPServer`, `(*fakeSMTPServer).port`, `testMessage` from Tasks 1 and 2.
- Produces: `func newFakeTLSSMTPServer(t *testing.T, extensions ...string) *fakeSMTPServer` and `func testTLSCert(t *testing.T) (tls.Certificate, []byte)`, used by Task 4's authentication-over-TLS test. Adds a `tlsConfig` field to `fakeSMTPServer`.

- [ ] **Step 1: Extend the fake relay to speak TLS, then write the failing tests**

Three changes to the harness in `mailer/smtp_test.go`.

**First**, replace the `fakeSMTPServer` struct, `newFakeSMTPServer`, `port`, and `serve` with
versions that support both TLS modes and more than one connection — `TestSendTrustsTheConfiguredCA`
below connects twice:

```go
// fakeSMTPServer accepts SMTP sessions on a local port and records the commands and the message
// bodies it receives. Ported from portal-conductor's emailsvc tests and extended with a
// configurable ESMTP capability list so that tests can offer or withhold STARTTLS and AUTH.
type fakeSMTPServer struct {
	listener   net.Listener
	extensions []string
	tlsConfig  *tls.Config
	certPEM    []byte
	commands   chan string
	data       chan string
}

// newFakeSMTPServer starts a cleartext relay advertising the given ESMTP extensions. It can
// still be upgraded in place by a STARTTLS command.
func newFakeSMTPServer(t *testing.T, extensions ...string) *fakeSMTPServer {
	t.Helper()
	return startFakeSMTPServer(t, false, extensions...)
}

// newFakeTLSSMTPServer starts a relay that speaks TLS from the first byte, as a relay on the
// implicit-TLS port does.
func newFakeTLSSMTPServer(t *testing.T, extensions ...string) *fakeSMTPServer {
	t.Helper()
	return startFakeSMTPServer(t, true, extensions...)
}

func startFakeSMTPServer(t *testing.T, implicitTLS bool, extensions ...string) *fakeSMTPServer {
	t.Helper()

	cert, certPEM := testTLSCert(t)
	tlsConfig := &tls.Config{Certificates: []tls.Certificate{cert}}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	if implicitTLS {
		listener = tls.NewListener(listener, tlsConfig)
	}
	t.Cleanup(func() { _ = listener.Close() })

	server := &fakeSMTPServer{
		listener:   listener,
		extensions: extensions,
		tlsConfig:  tlsConfig,
		certPEM:    certPEM,
		commands:   make(chan string, 64),
		data:       make(chan string, 1),
	}
	go server.serve()

	return server
}

// port returns the port the relay listens on, read back through the address rather than by a
// type assertion, because the implicit-TLS listener wraps the TCP one.
func (s *fakeSMTPServer) port() int {
	_, port, err := net.SplitHostPort(s.listener.Addr().String())
	if err != nil {
		panic(err)
	}
	number, err := strconv.Atoi(port)
	if err != nil {
		panic(err)
	}
	return number
}

// serve handles sessions until the listener closes. More than one session is needed by the test
// that checks a relay is rejected without a trust anchor and accepted with one.
func (s *fakeSMTPServer) serve() {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			return
		}
		s.session(conn)
		_ = conn.Close()
	}
}
```

Note that `serve` no longer closes the connection with a `defer`, because it now handles more
than one; `session` returns before each `Close`.

**Second**, add a `STARTTLS` case to the `switch` in `session`, between the `HELO` and `AUTH`
cases. `session` assigns to its own `conn` parameter, and both the `write` closure and the
reader are rebuilt from it, so the rest of the session runs over the upgraded connection:

```go
		case "STARTTLS":
			write("220 ready to start TLS")
			tlsConn := tls.Server(conn, s.tlsConfig)
			if err := tlsConn.Handshake(); err != nil {
				return
			}
			conn = tlsConn
			reader = bufio.NewReader(conn)
```

**Third**, add the certificate generator:

```go
// testTLSCert generates a self-signed certificate valid for 127.0.0.1 and returns it along with
// its PEM encoding, which doubles as the trust anchor fixture for email.smtpCACertFile. ECDSA
// rather than RSA because a key is generated for every test that touches TLS.
func testTLSCert(t *testing.T) (tls.Certificate, []byte) {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	template := x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "127.0.0.1"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
		IsCA:                  true,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}

	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		t.Fatal(err)
	}

	return cert, certPEM
}
```

Add `crypto/ecdsa`, `crypto/elliptic`, `crypto/rand`, `crypto/tls`, `crypto/x509`,
`crypto/x509/pkix`, `encoding/pem`, `math/big`, and `strconv` to the test file's imports.

Then the tests:

```go
// TestSendRequiresSTARTTLSWhenConfigured is the behavior that justifies owning the transport:
// gomail's dialer upgrades opportunistically and silently stays in cleartext when the relay
// doesn't offer STARTTLS. With email.smtpUseTLS set, that has to be an error.
func TestSendRequiresSTARTTLSWhenConfigured(t *testing.T) {
	server := newFakeSMTPServer(t) // advertises no extensions
	dialer, err := NewDialer(SMTPSettings{
		Host: "127.0.0.1", Port: server.port(), UseTLS: true, InsecureSkipVerify: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	err = dialer.send("noreply@example.org", []string{"someone@example.org"}, testMessage("body\r\n"))
	if err == nil {
		t.Fatal("expected an error when the relay does not advertise STARTTLS, got nil")
	}
	if !strings.Contains(err.Error(), "STARTTLS") {
		t.Errorf("expected the error to mention STARTTLS, got: %s", err)
	}

	// Nothing may be delivered over the cleartext connection.
	select {
	case body := <-server.data:
		t.Errorf("a message was delivered in cleartext:\n%s", body)
	default:
	}
}

// TestSendUpgradesWithSTARTTLS checks the STARTTLS path against a relay that offers it.
func TestSendUpgradesWithSTARTTLS(t *testing.T) {
	server := newFakeSMTPServer(t, "STARTTLS")
	dialer, err := NewDialer(SMTPSettings{
		Host: "127.0.0.1", Port: server.port(), UseTLS: true, InsecureSkipVerify: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	err = dialer.send("noreply@example.org", []string{"someone@example.org"}, testMessage("body\r\n"))
	if err != nil {
		t.Fatalf("send failed: %s", err)
	}

	commands := server.collectCommands(t)
	if !slices.Contains(commands, "STARTTLS") {
		t.Errorf("expected a STARTTLS command; got: %v", commands)
	}
	if body := <-server.data; !strings.Contains(body, "body") {
		t.Errorf("delivered message missing its body:\n%s", body)
	}
}

// TestSendOverImplicitTLS checks email.smtpUseSSL against a relay that is encrypted from the
// first byte.
func TestSendOverImplicitTLS(t *testing.T) {
	server := newFakeTLSSMTPServer(t)
	dialer, err := NewDialer(SMTPSettings{
		Host: "127.0.0.1", Port: server.port(), UseSSL: true, InsecureSkipVerify: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	err = dialer.send("noreply@example.org", []string{"someone@example.org"}, testMessage("body\r\n"))
	if err != nil {
		t.Fatalf("send failed: %s", err)
	}
	if body := <-server.data; !strings.Contains(body, "body") {
		t.Errorf("delivered message missing its body:\n%s", body)
	}
}

// TestSendTrustsTheConfiguredCA checks that email.smtpCACertFile is what makes a relay with a
// private certificate authority verifiable, and that verification is genuinely on: the same
// relay must be rejected without the setting.
func TestSendTrustsTheConfiguredCA(t *testing.T) {
	server := newFakeTLSSMTPServer(t)

	caFile := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(caFile, server.certPEM, 0o600); err != nil {
		t.Fatal(err)
	}

	trusting, err := NewDialer(SMTPSettings{
		Host: "127.0.0.1", Port: server.port(), UseSSL: true, CACertFile: caFile,
	})
	if err != nil {
		t.Fatal(err)
	}
	err = trusting.send("noreply@example.org", []string{"someone@example.org"}, testMessage("body\r\n"))
	if err != nil {
		t.Fatalf("send with the configured CA failed: %s", err)
	}
	<-server.data

	untrusting, err := NewDialer(SMTPSettings{Host: "127.0.0.1", Port: server.port(), UseSSL: true})
	if err != nil {
		t.Fatal(err)
	}
	err = untrusting.send("noreply@example.org", []string{"someone@example.org"}, testMessage("body\r\n"))
	if err == nil {
		t.Fatal("expected the self-signed relay to be rejected without the configured CA")
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./mailer/ -run 'TestSendRequiresSTARTTLS|TestSendUpgrades|TestSendOverImplicitTLS|TestSendTrusts' -v`
Expected: FAIL — `TestSendRequiresSTARTTLSWhenConfigured` fails because `connect` ignores `UseTLS` and delivers in cleartext; the implicit-TLS tests fail on the TLS handshake because `connect` dials in the clear.

- [ ] **Step 3: Write the implementation**

Replace `connect` in `mailer/smtp.go` with the full version. `crypto/tls` is already imported
from Task 1:

```go
// connect opens a connection to the relay, greets it, and puts it in the configured TLS state.
// A relay that won't offer STARTTLS when email.smtpUseTLS is set is an error rather than a
// silent downgrade: the setting exists to guarantee the connection is encrypted.
func (d *Dialer) connect() (*smtp.Client, error) {
	address := net.JoinHostPort(d.settings.Host, strconv.Itoa(d.settings.Port))

	if d.settings.UseSSL {
		conn, err := tls.DialWithDialer(&net.Dialer{Timeout: dialTimeout}, "tcp", address, d.tlsConfig)
		if err != nil {
			return nil, fmt.Errorf("connecting to SMTP server %s over TLS: %w", address, err)
		}
		client, err := smtp.NewClient(conn, d.settings.Host)
		if err != nil {
			_ = conn.Close()
			return nil, fmt.Errorf("SMTP greeting from %s failed: %w", address, err)
		}
		return d.hello(client, address)
	}

	conn, err := net.DialTimeout("tcp", address, dialTimeout)
	if err != nil {
		return nil, fmt.Errorf("connecting to SMTP server %s: %w", address, err)
	}

	client, err := smtp.NewClient(conn, d.settings.Host)
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("SMTP greeting from %s failed: %w", address, err)
	}
	if client, err = d.hello(client, address); err != nil {
		return nil, err
	}

	if d.settings.UseTLS {
		if ok, _ := client.Extension("STARTTLS"); !ok {
			_ = client.Close()
			return nil, fmt.Errorf(
				"SMTP server %s does not advertise STARTTLS, but email.smtpUseTLS is set", address,
			)
		}
		if err := client.StartTLS(d.tlsConfig); err != nil {
			_ = client.Close()
			return nil, fmt.Errorf("STARTTLS with %s failed: %w", address, err)
		}
	}

	return client, nil
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./mailer/ -v`
Expected: PASS, all tests including the earlier tasks'.

- [ ] **Step 5: Commit**

```bash
git add mailer/smtp.go mailer/smtp_test.go
git commit -m "Support implicit TLS and required STARTTLS in the SMTP dialer

email.smtpUseTLS fails closed when the relay does not advertise STARTTLS,
which gomail's opportunistic dialer cannot express."
```

---

### Task 4: Authentication

**Files:**
- Modify: `mailer/smtp.go`
- Modify: `mailer/smtp_test.go`

**Interfaces:**
- Consumes: `Dialer.authenticate` (the no-op stub from Task 2), `newFakeSMTPServer`, `newFakeTLSSMTPServer`, `testMessage`.
- Produces: `func (d *Dialer) authMechanism(mechanisms string) smtp.Auth` and `type loginAuth struct{...}` implementing `smtp.Auth`. Nothing outside the package uses either.

- [ ] **Step 1: Write the failing tests**

Append to `mailer/smtp_test.go`:

```go
// TestAuthMechanismSelection checks that the strongest mechanism the relay advertises is the
// one chosen. Selection is tested directly rather than end to end because driving three
// challenge-response exchanges through the fake relay would test the fake, not the choice.
func TestAuthMechanismSelection(t *testing.T) {
	tests := []struct {
		name       string
		advertised string
		expect     string
	}{
		{"prefers CRAM-MD5", "CRAM-MD5 LOGIN PLAIN", "CRAM-MD5"},
		{"LOGIN when PLAIN is absent", "LOGIN", "LOGIN"},
		{"PLAIN when both are offered", "LOGIN PLAIN", "PLAIN"},
		{"PLAIN by default", "PLAIN", "PLAIN"},
		{"PLAIN when nothing is recognized", "XOAUTH2", "PLAIN"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dialer, err := NewDialer(SMTPSettings{
				Host: "relay.example.org", Port: 587, User: "someone", Password: "secret", UseTLS: true,
			})
			if err != nil {
				t.Fatal(err)
			}

			auth := dialer.authMechanism(tt.advertised)
			proto, _, err := auth.Start(&smtp.ServerInfo{
				Name: "relay.example.org",
				TLS:  true,
				Auth: strings.Fields(tt.advertised),
			})
			if err != nil {
				t.Fatalf("starting authentication failed: %s", err)
			}
			if proto != tt.expect {
				t.Errorf("expected the %s mechanism, got %s", tt.expect, proto)
			}
		})
	}
}

// TestSendAuthenticatesOverTLS checks that configured credentials actually reach the relay.
// It runs over implicit TLS because Go's PlainAuth refuses to send credentials over an
// unencrypted connection to a remote host.
func TestSendAuthenticatesOverTLS(t *testing.T) {
	server := newFakeTLSSMTPServer(t, "AUTH PLAIN")
	dialer, err := NewDialer(SMTPSettings{
		Host: "127.0.0.1", Port: server.port(), UseSSL: true,
		User: "someone", Password: "secret", InsecureSkipVerify: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	err = dialer.send("noreply@example.org", []string{"someone@example.org"}, testMessage("body\r\n"))
	if err != nil {
		t.Fatalf("send failed: %s", err)
	}

	commands := server.collectCommands(t)
	index := slices.IndexFunc(commands, func(c string) bool { return strings.HasPrefix(c, "AUTH PLAIN ") })
	if index < 0 {
		t.Fatalf("expected an AUTH PLAIN command; got: %v", commands)
	}

	decoded, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(commands[index], "AUTH PLAIN "))
	if err != nil {
		t.Fatalf("the AUTH argument was not base64: %s", err)
	}
	if want := "\x00someone\x00secret"; string(decoded) != want {
		t.Errorf("expected the credentials %q, got %q", want, decoded)
	}
}

// TestSendReportsARelayThatCannotAuthenticate checks that credentials configured against a
// relay with no AUTH support are reported clearly instead of being silently dropped.
func TestSendReportsARelayThatCannotAuthenticate(t *testing.T) {
	server := newFakeTLSSMTPServer(t) // advertises no AUTH
	dialer, err := NewDialer(SMTPSettings{
		Host: "127.0.0.1", Port: server.port(), UseSSL: true,
		User: "someone", Password: "secret", InsecureSkipVerify: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	err = dialer.send("noreply@example.org", []string{"someone@example.org"}, testMessage("body\r\n"))
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "email.smtpUser") {
		t.Errorf("expected the error to name the setting at fault, got: %s", err)
	}
}
```

Add `encoding/base64` and `net/smtp` to the test file's imports.

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./mailer/ -run 'TestAuth|TestSendAuthenticates|TestSendReportsARelay' -v`
Expected: FAIL — `d.authMechanism undefined`, and once that compiles, the two send tests fail because `authenticate` is still the no-op stub.

- [ ] **Step 3: Write the implementation**

Replace the `authenticate` stub in `mailer/smtp.go` and add the LOGIN mechanism, adding `bytes` and `strings` to its imports:

```go
// authenticate presents the configured credentials, if any. A relay with no AUTH support and
// credentials configured is an error: silently sending unauthenticated would fail later in a
// way that points nowhere near the actual mistake.
func (d *Dialer) authenticate(client *smtp.Client) error {
	if d.settings.User == "" {
		return nil
	}

	ok, mechanisms := client.Extension("AUTH")
	if !ok {
		return fmt.Errorf(
			"SMTP server %s does not support authentication, but email.smtpUser is set",
			d.settings.Host,
		)
	}

	if err := client.Auth(d.authMechanism(mechanisms)); err != nil {
		return fmt.Errorf(
			"SMTP authentication failed (bad credentials, or the server requires a different mechanism?): %w",
			err,
		)
	}

	return nil
}

// authMechanism picks the strongest mechanism the relay advertises. The order follows gomail's,
// which this service used before it owned the transport, so relays that only offer LOGIN keep
// working. Note that Go's PlainAuth refuses to send credentials over an unencrypted connection
// to any host but localhost, so PLAIN effectively requires one of the TLS settings.
func (d *Dialer) authMechanism(mechanisms string) smtp.Auth {
	switch {
	case strings.Contains(mechanisms, "CRAM-MD5"):
		return smtp.CRAMMD5Auth(d.settings.User, d.settings.Password)
	case strings.Contains(mechanisms, "LOGIN") && !strings.Contains(mechanisms, "PLAIN"):
		return &loginAuth{username: d.settings.User, password: d.settings.Password, host: d.settings.Host}
	default:
		return smtp.PlainAuth("", d.settings.User, d.settings.Password, d.settings.Host)
	}
}

// loginAuth implements the non-standard LOGIN mechanism, which some relays offer in place of
// PLAIN. net/smtp has no implementation of it; this follows gomail's (auth.go).
type loginAuth struct {
	username string
	password string
	host     string
}

func (a *loginAuth) Start(server *smtp.ServerInfo) (string, []byte, error) {
	if !server.TLS && !slices.Contains(server.Auth, "LOGIN") {
		return "", nil, errors.New("refusing to send credentials over an unencrypted connection")
	}
	if server.Name != a.host {
		return "", nil, fmt.Errorf("expected to be talking to %s, not %s", a.host, server.Name)
	}
	return "LOGIN", nil, nil
}

func (a *loginAuth) Next(fromServer []byte, more bool) ([]byte, error) {
	if !more {
		return nil, nil
	}

	switch {
	case bytes.Equal(fromServer, []byte("Username:")):
		return []byte(a.username), nil
	case bytes.Equal(fromServer, []byte("Password:")):
		return []byte(a.password), nil
	default:
		return nil, fmt.Errorf("unexpected server challenge: %s", fromServer)
	}
}
```

Add `slices` to `mailer/smtp.go`'s imports as well.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./mailer/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add mailer/smtp.go mailer/smtp_test.go
git commit -m "Authenticate to SMTP relays that require credentials

Negotiates CRAM-MD5, LOGIN, or PLAIN based on what the relay advertises,
matching the behavior gomail's dialer provided before the transport moved."
```

---

### Task 5: Wire the dialer in, add Message-ID, and document the settings

The integration task. After this the feature is reachable from configuration.

**Files:**
- Modify: `mailer/email.go:1-140`
- Modify: `mailer/email_test.go:30`
- Modify: `mailer/smtp.go`
- Modify: `mailer/smtp_test.go`
- Modify: `main.go:249-253`
- Modify: `README.md`

**Interfaces:**
- Consumes: `NewDialer`, `Dialer.send`, `localHostname` from Tasks 1–4.
- Produces: `func NewEmailClient(dialer *Dialer, from string) *EmailClient` — a signature change from `NewEmailClient(smtpHost string, from string)` — and `func generateMessageID(from string) string`.

- [ ] **Step 1: Write the failing tests**

Append to `mailer/smtp_test.go`:

```go
// TestGenerateMessageID checks the Message-ID's domain. Its absence raises spam scores when no
// MTA in the delivery path supplies one, and a domain that doesn't match the sender is itself
// a spam signal.
func TestGenerateMessageID(t *testing.T) {
	original := osHostname
	t.Cleanup(func() { osHostname = original })
	osHostname = func() (string, error) { return "mail.example.org", nil }

	tests := []struct {
		name   string
		from   string
		suffix string
	}{
		{"bare address", "noreply@cyverse.org", "@cyverse.org>"},
		{"address with a display name", "DE <noreply@cyverse.org>", "@cyverse.org>"},
		{"no domain falls back to the hostname", "noreply", "@mail.example.org>"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id := generateMessageID(tt.from)
			if !strings.HasPrefix(id, "<") || !strings.HasSuffix(id, tt.suffix) {
				t.Errorf("expected an ID of the form <...%s, got %q", tt.suffix, id)
			}
			if id == generateMessageID(tt.from) {
				t.Error("expected consecutive Message-IDs to differ")
			}
		})
	}
}

// TestEmailClientSendDeliversThroughTheDialer checks the integration: EmailClient builds the
// message with gomail and the dialer puts it on the wire, with the headers a relay expects.
func TestEmailClientSendDeliversThroughTheDialer(t *testing.T) {
	server := newFakeSMTPServer(t)
	dialer, err := NewDialer(SMTPSettings{Host: "127.0.0.1", Port: server.port()})
	if err != nil {
		t.Fatal(err)
	}
	client := NewEmailClient(dialer, "noreply@cyverse.org")

	request := &FormattedEmailRequest{
		To:       []string{"to@example.org"},
		Cc:       []string{"cc@example.org"},
		Bcc:      []string{"bcc@example.org"},
		Subject:  "Analysis complete",
		MIMEType: HTMLMIMEType,
		Body:     "<html><body><p>Your analysis finished.</p></body></html>",
		Attachments: []Attachment{
			{Filename: "report.txt", Data: base64.StdEncoding.EncodeToString([]byte("report contents"))},
		},
	}
	if err := client.Send(context.Background(), request); err != nil {
		t.Fatalf("send failed: %s", err)
	}

	commands := server.collectCommands(t)
	for _, recipient := range []string{"to@example.org", "cc@example.org", "bcc@example.org"} {
		want := "RCPT TO:<" + recipient + ">"
		if !slices.Contains(commands, want) {
			t.Errorf("expected %q; got: %v", want, commands)
		}
	}

	message := <-server.data
	for _, want := range []string{
		"Subject: Analysis complete",
		"To: to@example.org",
		"Cc: cc@example.org",
		"Message-ID: <",
		"Date: ",
		"text/plain",
		"text/html",
		"report.txt",
	} {
		if !strings.Contains(message, want) {
			t.Errorf("delivered message missing %q:\n%s", want, message)
		}
	}
	// Bcc recipients get the message, but must not be named in its headers.
	if strings.Contains(message, "Bcc:") {
		t.Errorf("the Bcc header leaked into the delivered message:\n%s", message)
	}
	// The HTML body must arrive with a generated plain-text alternative.
	if !strings.Contains(message, "Your analysis finished.") {
		t.Errorf("delivered message missing the plain-text alternative:\n%s", message)
	}
}
```

Add `context` to the test file's imports.

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./mailer/ -run 'TestGenerateMessageID|TestEmailClientSend' -v`
Expected: FAIL — `undefined: generateMessageID`, and `NewEmailClient(dialer, ...)` does not match the current `NewEmailClient(smtpHost string, from string)`.

- [ ] **Step 3: Write the implementation**

First, append `generateMessageID` to `mailer/smtp.go`, adding `crypto/rand`, `encoding/binary`, and `encoding/hex` to its imports:

```go
// generateMessageID returns a unique RFC 5322 Message-ID using the sender's domain, or the
// local hostname when the From address has none. gomail supplies a Date header but not this
// one, and its absence raises spam scores when no MTA in the delivery path adds it.
func generateMessageID(from string) string {
	domain := localHostname()
	if at := strings.LastIndex(from, "@"); at != -1 && at < len(from)-1 {
		domain = strings.TrimRight(from[at+1:], ">")
	}

	random := make([]byte, 8)
	if _, err := rand.Read(random); err != nil {
		binary.BigEndian.PutUint64(random, uint64(time.Now().UnixNano()))
	}

	return fmt.Sprintf("<%d.%s@%s>", time.Now().UnixNano(), hex.EncodeToString(random), domain)
}
```

Then change `mailer/email.go`. Delete the `smtpPort` and `smtpLocalName` constants at lines 16-22 — both are settings now — and replace the client struct, constructor, and the tail of `Send`:

```go
// EmailClient is a client used to send email messages to an SMTP server.
type EmailClient struct {
	dialer      *Dialer
	fromAddress string
}

// NewEmailClient creates a new email client that delivers through the given dialer.
func NewEmailClient(dialer *Dialer, from string) *EmailClient {
	return &EmailClient{
		dialer:      dialer,
		fromAddress: from,
	}
}
```

In `Send`, add the `Message-ID` header next to the other headers, just after the `From` header is set:

```go
	m := gomail.NewMessage()
	fromAddress := r.GetFromAddress(req)
	m.SetHeader("From", fromAddress)
	m.SetHeader("Message-ID", generateMessageID(fromAddress))
	m.SetHeader("mailed-by", "cyverse.org")
```

and replace the delivery at the end of `Send`:

```go
	// gomail builds the message; this package owns the connection, so that the TLS and
	// authentication settings mean what they say.
	if err := gomail.Send(gomail.SendFunc(r.dialer.send), m); err != nil {
		log.Error(err)
		return err
	}

	return nil
```

Remove the now-unused `net/http` import only if nothing else in the file uses it — `Validate` does, so leave it.

Next, update the one existing call site in `mailer/email_test.go`. Replace:

```go
	client := NewEmailClient("smtp.example.org", "noreply@example.org")
```

with:

```go
	dialer, err := NewDialer(SMTPSettings{Host: "smtp.example.org", Port: 25})
	if err != nil {
		t.Fatal(err)
	}
	client := NewEmailClient(dialer, "noreply@example.org")
```

Finally, wire it in `main.go`. Replace the `mailer.NewEmailClient(...)` argument at line 253 and add the dialer construction above the processor:

```go
	// Build the outbound email processor, absorbed from the retired de-mailer service. Both
	// the /mail endpoint and the email_requests consumer below drive it.
	fromAddress := cfg.GetString("email.fromAddress")

	// The relay settings are all optional; their defaults describe the unauthenticated
	// cleartext relay this service talked to before they existed.
	cfg.SetDefault("email.smtpPort", 25)
	smtpDialer, err := mailer.NewDialer(mailer.SMTPSettings{
		Host:               cfg.GetString("email.smtpHost"),
		Port:               cfg.GetInt("email.smtpPort"),
		User:               cfg.GetString("email.smtpUser"),
		Password:           cfg.GetString("email.smtpPassword"),
		LocalName:          cfg.GetString("email.smtpLocalName"),
		CACertFile:         cfg.GetString("email.smtpCACertFile"),
		UseTLS:             cfg.GetBool("email.smtpUseTLS"),
		UseSSL:             cfg.GetBool("email.smtpUseSSL"),
		InsecureSkipVerify: cfg.GetBool("email.smtpInsecureSkipVerify"),
	})
	if err != nil {
		e.Logger.Fatalf("invalid SMTP configuration: %s", err.Error())
	}

	emailProcessor := mailer.NewEmailProcessor(
		mailer.NewEmailClient(smtpDialer, fromAddress),
		mailer.DESettings{
```

The rest of the `NewEmailProcessor` call is unchanged.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go build ./... && go test ./... -v`
Expected: PASS across every package, including the pre-existing `TestSendRejectsInvalidRequests` and `main_test.go`'s configuration tests, which are unaffected because `requiredConfigKeys` did not change.

- [ ] **Step 5: Document the settings**

Add this section to `README.md`, after the existing configuration discussion:

```markdown
## SMTP Relay Configuration

`notifications` sends outbound email directly to an SMTP relay. Only `email.smtpHost` is
required; every other setting below is optional, and the defaults describe an unauthenticated
relay listening in the clear on port 25, which is how the service behaved before these settings
existed.

| Setting | Default | Description |
|---|---|---|
| `email.smtpHost` | *(required)* | The relay's hostname. |
| `email.smtpPort` | `25` | The relay's port. |
| `email.smtpUser` | *(none)* | Username for SMTP authentication. Authentication is skipped when this is empty. |
| `email.smtpPassword` | *(none)* | Password for SMTP authentication. Must be set if `email.smtpUser` is. |
| `email.smtpUseTLS` | `false` | Require STARTTLS. Delivery fails if the relay does not offer it. |
| `email.smtpUseSSL` | `false` | Use TLS from the first byte, as relays on port 465 do. |
| `email.smtpLocalName` | the machine hostname | The name sent in the SMTP HELO. |
| `email.smtpInsecureSkipVerify` | `false` | Skip verification of the relay's certificate. |
| `email.smtpCACertFile` | *(none)* | Path to a PEM bundle to trust instead of the system roots. |

`email.smtpUseTLS` and `email.smtpUseSSL` are mutually exclusive: STARTTLS upgrades a cleartext
connection, while implicit TLS is encrypted from the start. Use `email.smtpUseTLS` on ports 25
and 587, and `email.smtpUseSSL` on port 465.

Setting `email.smtpUser` without either TLS setting will fail against a remote relay. Go refuses
to send credentials over an unencrypted connection to any host other than `localhost`, so an
authenticated relay needs `email.smtpUseTLS` or `email.smtpUseSSL` as well.

For a relay with a certificate signed by a private authority, point `email.smtpCACertFile` at the
authority's PEM bundle. `email.smtpInsecureSkipVerify` disables verification entirely and is a
last resort; it cannot be combined with `email.smtpCACertFile`.

Contradictory settings are rejected when the service starts, not at the first delivery attempt.
```

- [ ] **Step 6: Commit**

```bash
git add mailer/smtp.go mailer/smtp_test.go mailer/email.go mailer/email_test.go main.go README.md
git commit -m "Send outbound email through the configurable SMTP dialer

Adds the email.smtp* settings, so deployments can reach authenticated and
TLS-only relays rather than only an unauthenticated local relay. Also sets
a Message-ID, which gomail does not supply and whose absence raises spam
scores when no MTA in the delivery path adds one."
```

---

## Verification

From `/Users/sarahr/src/de/notifications`:

```bash
go build ./...
go test ./... -race
go vet ./...
golangci-lint run   # if installed
```

All four must be clean. `-race` matters here: the fake relay runs its session in a goroutine and the tests read its channels from the test goroutine.
