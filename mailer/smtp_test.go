package mailer

import (
	"bufio"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/smtp"
	"os"
	"path/filepath"
	"slices"
	"strconv"
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
		case "STARTTLS":
			write("220 ready to start TLS")
			tlsConn := tls.Server(conn, s.tlsConfig)
			if err := tlsConn.Handshake(); err != nil {
				return
			}
			conn = tlsConn
			reader = bufio.NewReader(conn)
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

// TestSendDoesNotBlockOnASilentRelay checks that a relay which accepts the connection and then
// says nothing fails instead of blocking forever. Pointing email.smtpPort at an implicit-TLS
// relay without setting email.smtpUseSSL looks exactly like this from this end: the relay waits
// for a ClientHello while the client waits for a greeting. dialTimeout only covers the TCP
// connect, so without a deadline on the greeting itself the send goroutine is wedged for good.
func TestSendDoesNotBlockOnASilentRelay(t *testing.T) {
	original := dialTimeout
	t.Cleanup(func() { dialTimeout = original })
	dialTimeout = 200 * time.Millisecond

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })

	// Accept the connection and hold it open without ever writing a greeting.
	accepted := make(chan net.Conn, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		accepted <- conn
	}()
	t.Cleanup(func() {
		select {
		case conn := <-accepted:
			_ = conn.Close()
		default:
		}
	})

	dialer, err := NewDialer(SMTPSettings{
		Host: "127.0.0.1", Port: listener.Addr().(*net.TCPAddr).Port,
	})
	if err != nil {
		t.Fatal(err)
	}

	done := make(chan error, 1)
	go func() {
		done <- dialer.send("noreply@example.org", []string{"someone@example.org"}, testMessage("body\r\n"))
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected an error from a relay that never greets, got nil")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("send blocked on a relay that never greets: the SMTP greeting is not bounded by a deadline")
	}
}
