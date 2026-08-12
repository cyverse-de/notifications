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

Flags:

- `--config`, `-c` — path to the config file
- `--port`, `-p` — HTTP listen port (default 8080)
- `--debug`, `-d` — enable debug logging
