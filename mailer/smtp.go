package mailer

import (
	"bytes"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/smtp"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"
)

// defaultLocalName is the HELO name used when the machine hostname isn't available.
const defaultLocalName = "notifications"

// osHostname is a seam so that tests can exercise the hostname lookup failing.
var osHostname = os.Hostname

// SMTPSettings describes how to reach the SMTP relay.
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

// dialTimeout bounds the TCP connect and the SMTP greeting. A variable rather than a constant
// so that tests can shrink it.
var dialTimeout = 30 * time.Second

// send delivers one already-built message. The signature is what gomail.SendFunc expects.
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

// connect opens a connection to the relay, greets it, and puts it in the configured TLS state.
// A relay that won't offer STARTTLS when email.smtpUseTLS is set is an error rather than a
// silent downgrade: the setting exists to guarantee the connection is encrypted.
func (d *Dialer) connect() (*smtp.Client, error) {
	address := net.JoinHostPort(d.settings.Host, strconv.Itoa(d.settings.Port))

	conn, err := d.dial(address)
	if err != nil {
		return nil, err
	}

	// Bound the greeting and any handshake. Without this, a relay that accepts the connection
	// and then says nothing blocks the sender forever, which is exactly what reaching an
	// implicit-TLS relay without email.smtpUseSSL looks like from this end.
	if err := conn.SetDeadline(time.Now().Add(dialTimeout)); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("setting a deadline on the connection to %s: %w", address, err)
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

	// The session is established. Clear the deadline so that transferring a large attachment
	// over a slow link isn't cut off part way through the message.
	if err := conn.SetDeadline(time.Time{}); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("clearing the deadline on the connection to %s: %w", address, err)
	}

	return client, nil
}

// dial opens the connection to the relay, encrypted from the first byte when email.smtpUseSSL
// is set, as relays on the implicit-TLS port expect.
func (d *Dialer) dial(address string) (net.Conn, error) {
	if d.settings.UseSSL {
		conn, err := tls.DialWithDialer(&net.Dialer{Timeout: dialTimeout}, "tcp", address, d.tlsConfig)
		if err != nil {
			return nil, fmt.Errorf("connecting to SMTP server %s over TLS: %w", address, err)
		}
		return conn, nil
	}

	conn, err := net.DialTimeout("tcp", address, dialTimeout)
	if err != nil {
		return nil, fmt.Errorf("connecting to SMTP server %s: %w", address, err)
	}
	return conn, nil
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

// authMechanism picks the strongest mechanism the relay advertises, so that a relay offering
// only LOGIN still works. Go's PlainAuth refuses to send credentials over an unencrypted
// connection to any host but localhost, so PLAIN effectively requires one of the TLS settings.
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
