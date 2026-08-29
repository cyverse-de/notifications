package mailer

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net"
	"net/smtp"
	"os"
	"strconv"
	"time"
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

// authenticate is a no-op until credential support is added; sending without credentials is
// the existing behavior.
func (d *Dialer) authenticate(_ *smtp.Client) error {
	return nil
}
