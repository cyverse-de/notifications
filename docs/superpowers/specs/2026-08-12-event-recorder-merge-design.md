# Merging event-recorder into the notifications service

## Goal

Reduce the number of deployed DE services by absorbing `event-recorder` into
`notifications`. The two are already two halves of one system split across an
AMQP queue, and they duplicate code. Merging them removes a deployment and
deletes the duplication.

Net result: 2 deployments become 1. The only intentional behavior change is the
publish ordering fix described below; everything else is a lift-and-shift.

## Background

The current notification flow:

1. Callers (`app-exposer`, `apps`, `data-info`, `terrain`, `requests`) POST to
   `http://notifications/v1/notification`.
2. `api/v1/notification_request.go:146` publishes the request to the `de`
   exchange with routing key `events.notification.update.<type>`.
3. `event-recorder` consumes it from the durable `event_listener` queue
   (`handlerset/main.go:19-20`), writes it to the `notifications` database, and
   publishes `notification.<user>` for the DE UI.
4. The `notifications` service serves those rows back out through its listing
   API (`db/listings.go:136`).

The AMQP hop leaves the `notifications` service and comes back into a service
that shares its database. Across the entire `cyverse-de` source tree there is
exactly one publisher and one consumer of that routing key.

`notification-agent` is not involved. It is no longer deployed — it has no role
under `deployments/ansible/roles/services/` — and the legacy `notification_agent`
config key resolves to `baseurls_notifications` (`http://notifications/v1`).
`app-exposer`'s hardcoded `http://notification-agent` fallback
(`cmd/app-exposer/expiration.go:87`) is stale and only fires if config is absent.

### Existing duplication between the two repos

True duplication, safe to collapse to one copy:

| Duplicated item | In event-recorder | In notifications |
| --- | --- | --- |
| `fixTimestamp` | `handlers/legacy.go:82` | `api/v1/notification_request.go:18` |
| `common/times.go` | byte-identical | byte-identical |
| `AMQPSettings`, `ValidateEmailAddress` | `common/main.go` | `common/main.go` |

**Name collisions that are not duplicates.** Two `db` functions share a name and
a signature across the repos but have different semantics. They must both
survive, under distinct names — merging them would insert empty-string foreign
keys.

| Name | event-recorder | notifications |
| --- | --- | --- |
| `GetUserID` | get-or-create; calls `AddUser` on `sql.ErrNoRows` (`db/users.go:41`) | read-only; returns `""` for an unknown user with no error (`db/users.go:13`) |
| `GetNotificationTypeID` | selects `id::text`; errors when absent (`db/notification_types.go:15`) | selects `id`; returns `""` when absent with no error (`db/misc.go:14`) |

`SaveNotification` uses both results as foreign keys and needs the strict,
creating variants. The lenient variants have six call sites across
`api/v1/updates.go` and `api/v2/updates.go` that rely on the `""` behavior. The
incoming versions are therefore renamed to `GetOrCreateUserID` and
`RequireNotificationTypeID`, which also names them for what they actually do.

Both services already read the same config file (`jobservices.yml`), the same
`notifications.db.uri`, and the same `amqp.*` settings. Their deployment
templates (`jobservices.yml.j2`) are byte-identical, so the merge requires no
configuration changes.

## Non-goals

**Collapsing the AMQP hop.** Calling the recorder in-process from the v1 handler
was considered and rejected. An audit of the five callers found that only
`app-exposer` retries a failed POST (capped exponential backoff,
`expiration/notify.go`). `apps` catches and logs
(`src/apps/clients/notifications.clj:43`), `data-info` and `terrain` propagate
the exception, and `requests` returns the error before `tx.Commit()`
(`api/requests.go:374`) — which would tie an unrelated administrative request
status change to the health of the notifications database. Keeping the queue
preserves fire-and-forget semantics for all five. This can be revisited later.

**Modernizing other dependencies** beyond what the messaging bump forces.

**Fixing `requests`' missing `http.Client` timeout**
(`clients/notificationagent/main.go:16`). Real, pre-existing, and unrelated once
the queue stays.

## Design

### Package layout

The recording logic moves into the `notifications` repo as a `recorder/`
package that runs as a consumer goroutine alongside the existing Echo server.

- `recorder/recorder.go` — from event-recorder's `handlers/legacy.go`
- `recorder/consumer.go` — from `handlerset/main.go`: queue binding,
  routing-key parsing, ack/nack, error dispatch, the unrecoverable-error
  support email
- `recorder/errors.go` — from `handlers/errors.go`: `RecoverableError` and
  `UnrecoverableError`
- `db/notifications.go` — `SaveNotification`, `SaveOutgoingNotification`,
  `CountUnreadNotifications`
- `db/notification_types.go` — `RegisterNotificationType`

True duplicates collapse to the canonical `notifications` versions.
`fixTimestamp` moves to `common/` since both `api/v1` and the recorder need it,
and the `Notification` struct moves into `common/main.go`. event-recorder's
`AddUser` (`db/users.go:14`) has no counterpart and moves in as-is. The two
name-collision functions move in renamed, per the table above.

### AMQP clients

The existing publish client is shared by `api/v1` and the recorder. One
additional client is added for consuming, bound to queue `event_listener` with
key `events.*.update.*` and prefetch 100 — unchanged from today.

**To verify during implementation:** event-recorder constructs its consumer
client with `reconnect=true` (`handlerset/main.go:39`), while `job-status`
deliberately uses `false`, commenting that the messaging library's reconnect
path does not reestablish consumers and that exiting for a Kubernetes restart is
safer. If that limitation still holds in v12, event-recorder has a latent bug
where a reconnect leaves it silently not consuming, and the merged service
should use `false`.

### Publish ordering fix

Today the email request and the outbound UI notification are published *before*
the transaction commits (`handlers/legacy.go:219` and `:250`, with the commit at
`:256`). A commit failure therefore sends the email and then requeues the
message, and the retry sends it again.

The merged recorder commits first, then publishes. A publish failure leaves a
recorded notification with no email or UI ping — recoverable and visible in
logs — rather than a duplicate email. This is the only intentional behavior
change in the merge.

### Wire compatibility

`db/listings.go:136` selects `n.outgoing_json` and `formatNotification`
unmarshals it back to clients, so that column is a wire contract. The same code
builds it from the same inputs, so no listing changes are needed — but the shape
must be pinned by a golden-fixture test captured from the current event-recorder
code before that code is deleted.

The `notifications.incoming_json` and `notifications.routing_key` columns store
the raw incoming JSON and the `events.notification.update.<type>` key. Neither
is served by the v1 listing, but both continue to be written exactly as today.

## Sequencing

Three PRs, each independently reviewable and revertible.

### PR 1 — dependency bump (notifications repo)

Bump `messaging/v9` to `v12`, which requires `go` 1.21 to 1.26 and pulls otel
1.6 to 1.43, rippling through `otelecho v0.49.0` and `echo-middleware/v2`.

This is required because the moved consumer code needs an AMQP client, and v9
and v12 are built on different ones — `streadway/amqp` (unmaintained since
2021) and `rabbitmq/amqp091-go` respectively. `AddConsumer` and `Listen` have
identical signatures in both, but `MessageHandler` takes a `Delivery` from the
respective package, so the code cannot straddle them.

The publish-side types are wire-identical between v9 and v12 —
`messagetypes.go` differs only by `interface{}` vs `any` and the `model` import
version, with identical JSON tags — so `api/v1`'s existing publish call needs no
changes.

No behavior change. Verify in QA before proceeding.

### PR 2 — the merge (notifications repo)

Move the packages, dedupe, wire the consumer into `main.go`, apply the publish
ordering fix, port the tests.

### PR 3 — cleanup

In the `deployments` repo, remove the `event-recorder` role. In the
`event-recorder` repo, the k8s manifest, skaffold config, and GitHub workflow go
away with the repo itself, which is archived on GitHub once the cutover is
confirmed.

## Cutover

1. Deploy the merged `notifications`. During and after rollout, old
   `event-recorder` pods and new `notifications` pods are competing consumers on
   the same queue with the same binding, so no message is processed twice and
   none is lost.
2. Watch `event_listener` queue depth drain to zero.
3. Scale `event-recorder` to zero replicas and confirm notifications still flow
   end to end.
4. Delete the event-recorder deployment artifacts (PR 3).

Reversible at every step: scaling event-recorder back up restores the old
consumer without any change to the merged service.

Replica count stays at 2, matching both services today.

## Testing

Port event-recorder's existing tests, which already mock the database and
messaging clients:

- `handlers/legacy_test.go` — the recording path
- `handlers/errors_test.go` — error classification
- `db/notification_types_test.go` — type registration
- `common/times_test.go` — already duplicated in `notifications`; drop one

Keep them table-driven. Add:

- A golden-fixture test pinning the `outgoing_json` shape, captured from the
  current event-recorder output before deletion.
- Coverage of the new publish ordering: a commit failure must not have published
  the email or the UI message.

## Open questions

None blocking. The `reconnect=true` question under "AMQP clients" is resolved
during implementation, not before.
