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
