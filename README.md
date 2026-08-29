# notifications

This service provides the RESTful API for the revised notification system, and records the
notification events that API publishes. The recording half used to be the separate
`event-recorder` service.

## Components

- **api** — the HTTP API. `POST /v1/notification` validates an incoming notification request and
  publishes it to the `de` AMQP exchange with the `events.notification.update.<type>` routing key.
  The v1 and v2 listing endpoints serve recorded notifications back to clients.
- **recorder** — consumes those events from the durable `event_listener` queue, records them in
  the `notifications` database, and publishes the outgoing email request and the
  `notification.<user>` message that the DE UI listens for.

The queue sits between the two halves so that a caller's POST returns as soon as the event is
published, rather than waiting on the database write.

## Configuration

Reads a YAML config file (default `/etc/iplant/de/jobservices.yml`):

```yaml
amqp:
  uri: amqp://user:password@rabbit:5672/de
  exchange:
    name: de
    type: topic
notifications:
  db:
    uri: postgresql://user:password@dedb:5432/notifications?sslmode=disable
email:
  request: support@example.org
```

`email.request` receives a message when a delivery is discarded because it could not be recorded.

### SMTP relay

Outbound email goes directly to an SMTP relay. Only `email.smtpHost` is required; every other
setting below is optional, and the defaults describe an unauthenticated relay listening in the
clear on port 25, which is how the service behaved before these settings existed.

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

```yaml
email:
  smtpHost: smtp.example.org
  smtpPort: 587
  smtpUser: notifications@example.org
  smtpPassword: secret
  smtpUseTLS: true
```

`email.smtpUseTLS` and `email.smtpUseSSL` are mutually exclusive: STARTTLS upgrades a cleartext
connection, while implicit TLS is encrypted from the start. Use `email.smtpUseTLS` on ports 25
and 587, and `email.smtpUseSSL` on port 465.

Setting `email.smtpUser` without either TLS setting will fail against a remote relay. Go refuses
to send credentials over an unencrypted connection to any host other than `localhost`, so an
authenticated relay needs `email.smtpUseTLS` or `email.smtpUseSSL` as well.

For a relay whose certificate is signed by a private authority, point `email.smtpCACertFile` at
that authority's PEM bundle. `email.smtpInsecureSkipVerify` disables verification entirely and is
a last resort; it cannot be combined with `email.smtpCACertFile`.

`email.smtpLocalName` defaults to the machine's hostname. Receiving MTAs score an unresolvable
HELO name against the sender, so set this to a fully qualified domain name when sending through a
relay that checks.

Contradictory settings are rejected when the service starts, not at the first delivery attempt.

Flags:

- `--config`, `-c` — path to the config file
- `--port`, `-p` — HTTP listen port (default 8080)
- `--debug`, `-d` — enable debug logging
