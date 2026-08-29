# Configurable SMTP relay support for notifications

Date: 2026-08-28

## Problem

`notifications` can only talk to one kind of SMTP relay. `mailer/email.go` hardcodes
the port to 25 and the HELO name to `notifications`, and offers no way to configure
authentication or TLS. The only configurable setting is `email.smtpHost`. That is
enough for the local exim relay the service was built against, and nothing else.

Deployments that need to send through an authenticated or TLS-only relay have no
option today. Routing those messages through `portal-conductor` was considered and
rejected: `portal-conductor` is not present in every deployment, so depending on it
would make outbound email fail in exactly the environments that need this feature.

## Approach

`portal-conductor`'s `emailsvc` package already solves the transport half of this
problem — it dials with implicit TLS or STARTTLS, authenticates, and sends a real
HELO name. But it cannot replace `notifications`' sender wholesale, because it has
no support for attachments, no `Cc`, and no plain-text alternative generated from an
HTML body. All three are in use today.

`notifications` builds its messages with `gomail.v2`, which handles exactly those
three things. gomail's `Dialer` can also authenticate and do implicit TLS, but its
STARTTLS is opportunistic only (`smtp.go:83`): it upgrades when the relay advertises
STARTTLS and silently stays in cleartext when it does not, with no way to require it.
A `useTLS` setting backed by that dialer would not mean what it says.

So: **port `emailsvc`'s transport, keep gomail's message building.** `connect()` from
`emailsvc.go:107-140` moves into `notifications/mailer`, and the resulting live
`*smtp.Client` is handed to gomail through a `gomail.SendFunc`. The message
construction path in `EmailClient.Send` does not change at all, so attachments, `Cc`,
`Bcc`, and the html2text alternative carry no regression risk, while `useTLS` and
`useSSL` fail closed and mean precisely what they say.

## Configuration

New settings sit alongside the existing `email.smtpHost` and `email.fromAddress`.
They stay flat rather than nested under an `email.smtp` section, because the
configuration file template is shared across several services; nesting can come later.

| Key | Type | Default | Meaning |
|---|---|---|---|
| `email.smtpHost` | string | *(existing, required)* | Relay hostname |
| `email.smtpPort` | int | `25` | Relay port |
| `email.smtpUser` | string | `""` | SASL username; authentication is skipped entirely when empty |
| `email.smtpPassword` | string | `""` | SASL password |
| `email.smtpUseTLS` | bool | `false` | Require STARTTLS after connecting; fail when the relay does not offer it |
| `email.smtpUseSSL` | bool | `false` | Use implicit TLS from the first byte, as on port 465 |
| `email.smtpLocalName` | string | `os.Hostname()` | Name sent in the SMTP HELO/EHLO |
| `email.smtpInsecureSkipVerify` | bool | `false` | Skip verification of the relay's certificate |
| `email.smtpCACertFile` | string | `""` | PEM bundle to trust in place of the system roots |

Every new setting is optional, so `requiredConfigKeys` in `main.go` is unchanged. The
transport defaults reproduce the service's current behavior: port 25, no authentication,
no TLS.

`email.smtpLocalName` is the one default that changes existing behavior. It follows
`portal-conductor` in defaulting to `os.Hostname()` rather than keeping the hardcoded
`notifications`, because a bare service name is never a fully qualified domain name,
while a hostname sometimes is — on a VM or a container with its hostname set, this
yields a HELO name that a receiving MTA can actually resolve, and receivers score an
unresolvable HELO against the sender. Under Kubernetes it resolves to the pod name,
which is no better than `notifications` but no worse either, so the default degrades
gracefully instead of being uniformly wrong. Deployments that need a specific FQDN set
the setting explicitly.

The change is safe for the existing local exim deployment: as the comment being removed
from `mailer/email.go` records, that relay authorizes senders by source address rather
than by HELO name, so the only visible difference is which name appears in its mail logs.

When `os.Hostname()` fails or returns an empty string, the fallback is `notifications`,
the value in use today. `emailsvc` falls back to `localhost` (`emailsvc.go:147-152`),
which is the exact string this feature is trying to avoid sending.

### Authentication mechanism

`emailsvc` authenticates with `smtp.PlainAuth` only. This service negotiates instead,
the way gomail's dialer does (`smtp.go:96-107`): `CRAM-MD5` when the relay advertises it,
otherwise `LOGIN` when the relay advertises `LOGIN` but not `PLAIN`, otherwise `PLAIN`.
Supporting the mechanism the relay actually offers is the point of this work, so the
narrower `PLAIN`-only behavior is not carried over.

Go's `smtp.PlainAuth` refuses to send credentials over an unencrypted connection to any
host other than `localhost`. Configuring `email.smtpUser` without `email.smtpUseTLS` or
`email.smtpUseSSL` will therefore fail at send time against a remote relay. This is not
rejected at startup — a relay reached over a trusted local link is a legitimate
configuration — but the README notes it, since the resulting error is otherwise puzzling.

### Startup validation

These combinations are rejected when the service starts, alongside the existing
`validateConfig` check, so that a misconfigured deployment fails immediately rather
than at the first delivery attempt:

- `email.smtpUseTLS` and `email.smtpUseSSL` both set — they are mutually exclusive.
- `email.smtpUser` set without `email.smtpPassword`, or the reverse.
- `email.smtpInsecureSkipVerify` set together with `email.smtpCACertFile` — the
  combination is contradictory, and silently honoring one would hide the mistake.
- `email.smtpCACertFile` naming a file that cannot be read or that contains no PEM
  certificate.

## Components

### `mailer/smtp.go` (new)

Owns transport, and nothing else.

```go
type SMTPSettings struct {
    Host, User, Password, LocalName, CACertFile string
    Port                                        int
    UseTLS, UseSSL, InsecureSkipVerify          bool
}

func NewDialer(settings SMTPSettings) (*Dialer, error)
func (d *Dialer) connect() (*smtp.Client, error)
func (d *Dialer) send(from string, to []string, msg io.WriterTo) error
```

`NewDialer` applies the startup validation above, reads and parses `CACertFile` when
set, and builds the `*tls.Config` once so that per-message sends do no I/O beyond the
connection itself.

`connect()` is `emailsvc.go:107-140` ported with minimal change: a 30 second dial
timeout, `tls.DialWithDialer` when `UseSSL` is set, `smtp.NewClient`, an explicit
`Hello` with the configured local name, and — when `UseTLS` is set — `StartTLS`, whose
error is returned rather than swallowed.

`send` has the signature `gomail.SendFunc` expects. It connects, authenticates when
credentials are configured, issues `MAIL FROM` and one `RCPT TO` per recipient, writes
the message through `msg.WriteTo`, and quits.

### `mailer/email.go` (modified)

- `NewEmailClient(dialer *Dialer, from string) *EmailClient`. The `smtpPort` and
  `smtpLocalName` constants are deleted; both are now settings.
- `Send` replaces its `gomail.Dialer{...}.DialAndSend(m)` call with
  `gomail.Send(gomail.SendFunc(r.dialer.send), m)`. Every line above that is unchanged.
- `Send` sets one additional header, `Message-ID`, using `generateMessageID` ported from
  `emailsvc.go:230-241`. Its absence raises spam scores when no MTA in the delivery path
  supplies one. `Date` and the stripping of `Bcc` from the wire headers are *not* ported:
  gomail already does both (`writeto.go:25` and `writeto.go:244`). `generateMessageID`
  falls back to a domain when the `From` address has none, and uses the same hostname
  helper as the HELO default so that the two cannot disagree.

### `main.go` (modified)

Builds an `SMTPSettings` from the loaded configuration, calls `NewDialer`, and reports a
failure with `e.Logger.Fatalf` next to the existing configuration validation.

## Error handling

Transport failures are wrapped with the relay address, following `emailsvc`'s messages:
`connecting to SMTP server %s over TLS: %w`, `STARTTLS with %s failed: %w`, and
`SMTP authentication failed (bad credentials or server requires a different mechanism?): %w`.

They stay ordinary errors rather than `*HTTPError`, so `ErrorCode` reports them as 500.
That is correct: an unreachable or misbehaving relay is not the caller's fault. The
existing 400-classified errors from `FormattedEmailRequest.Validate` are untouched, and
the `Process` wrapping that `mailer/email_test.go` already covers keeps working.

Configuration errors surface only at startup, never per-message.

## Testing

The `fakeSMTPServer` from `portal-conductor/emailsvc/smtp_test.go:14-86` ports into
`mailer/smtp_test.go` with two extensions: a configurable multiline EHLO capability list,
so tests can present or withhold `STARTTLS` and `AUTH`, and the option to wrap the
listener in TLS using a self-signed certificate generated within the test. The generated
certificate serves double duty as the fixture for the `smtpCACertFile` and
`smtpInsecureSkipVerify` paths.

Cases:

- Default cleartext send: the EHLO name is the machine hostname and is neither empty nor
  `localhost`; `MAIL FROM`; one `RCPT TO` per address
  across `To`, `Cc`, and `Bcc`; `Message-ID` present in the delivered message; `Bcc`
  absent from the delivered headers; an attachment and an HTML alternative both intact.
- A configured `email.smtpLocalName` appears in the EHLO, overriding the hostname.
- The hostname lookup failing falls back to `notifications` rather than to `localhost`.
- `UseTLS` against a relay that does not advertise STARTTLS returns an error, and no
  message is delivered.
- `UseTLS` against a relay that does advertise it completes over the upgraded connection.
- `UseSSL` against a TLS listener completes.
- Credentials are sent when the relay advertises `AUTH PLAIN` over TLS, and `CRAM-MD5`
  and `LOGIN` are each selected when they are the mechanism the relay advertises.
- A table test over `NewDialer` covering each rejected settings combination.
- `TestSendRejectsInvalidRequests` continues to pass unchanged in substance; only its
  `NewEmailClient` call is updated for the new signature.

## Documentation

The README gains a section listing the new `email.*` settings and describing the two TLS
modes and when each applies.

## Out of scope

- Replacing `gomail.v2`, which is unmaintained but handles message construction correctly.
- Nesting the configuration settings under an `email.smtp` section.
- Any change to the AMQP consumer, the recorder, or the notification APIs.
