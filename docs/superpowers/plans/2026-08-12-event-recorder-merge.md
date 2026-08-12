# event-recorder → notifications Merge Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Absorb the `event-recorder` service into `notifications` so the DE runs one deployment instead of two, deleting the duplicated code between them.

**Architecture:** The recording logic moves into the `notifications` repo as a `recorder/` package that runs as an AMQP consumer goroutine alongside the existing Echo server. The `event_listener` queue and its `events.*.update.*` binding are preserved exactly, so callers and message flow are unchanged. The one intentional behavior change is that the outbound email and UI messages are published after the database transaction commits rather than before.

**Tech Stack:** Go 1.26, `cyverse-de/messaging/v12` (on `rabbitmq/amqp091-go`), Echo v4, squirrel, `lib/pq`, testify, `DATA-DOG/go-sqlmock`.

**Spec:** `docs/superpowers/specs/2026-08-12-event-recorder-merge-design.md`

## Global Constraints

- Go directive: `go 1.26` (forced by `messaging/v12`).
- `github.com/cyverse-de/messaging/v12 v12.0.2` — not v9. The AMQP delivery type is `github.com/rabbitmq/amqp091-go`.
- `github.com/cyverse-de/go-mod/otelutils v0.0.6` — matches what event-recorder already runs.
- Tests are table-driven (project Go guideline). The event-recorder tests being ported are currently four near-identical functions; consolidate them.
- Typed errors only — `errors.As` against `RecoverableError` / `UnrecoverableError`. Never string-match error text.
- Thread `context.Context` through every DB call; use the `*Context` method variants.
- Every DB function takes a `*sql.Tx`.
- `outgoing_json` is a wire contract consumed by `db/listings.go:136`. Its shape must not change.
- Run `gofmt`, `goimports`, and `golangci-lint run` before each commit.
- Do not modify `api/v2`. Do not modify the `requests`, `apps`, `data-info`, `terrain`, or `app-exposer` repos.

## Repos and Branches

| PR | Repo | Branch | Tasks |
| --- | --- | --- | --- |
| 1 | notifications | `bump-messaging-v12` | 1 |
| 2 | notifications | `merge-event-recorder` (stacked on PR 1) | 2–8 |
| 3 | deployments | `remove-event-recorder` | 9 |

## File Structure

**PR 1 — dependency bump (notifications repo)**

- Modify: `go.mod`, `go.sum`
- Modify: `main.go:12`, `api/main.go:7`, `api/v1/main.go:7`, `api/v2/main.go:7`, `api/v1/notification_request.go:9` — import path `messaging/v9` → `messaging/v12`

**PR 2 — the merge (notifications repo)**

- Create: `recorder/recorder.go` — the recording logic (from event-recorder `handlers/legacy.go`)
- Create: `recorder/errors.go` — `RecoverableError`, `UnrecoverableError` (from `handlers/errors.go`)
- Create: `recorder/clients.go` — `DatabaseClient` / `MessagingClient` interfaces and the default DB implementation (from `handlers/main.go`)
- Create: `recorder/consumer.go` — queue binding, routing-key parsing, ack/nack, error dispatch, support email (from `handlerset/main.go`)
- Create: `recorder/recorder_test.go`, `recorder/errors_test.go`, `recorder/testdata/outgoing_json_golden.json`
- Create: `db/notifications.go` — `SaveNotification`, `SaveOutgoingNotification`, `CountUnreadNotifications`
- Create: `db/notification_types.go` — `RegisterNotificationType`
- Create: `db/notification_types_test.go`
- Modify: `common/main.go` — add `Notification` struct and `FixTimestampInMap`
- Modify: `db/users.go` — add `AddUser`
- Modify: `api/v1/notification_request.go` — delete the local `fixTimestamp`, call `common.FixTimestampInMap`
- Modify: `main.go` — build the consumer client, register the consumer, start listening

**PR 3 — cleanup (deployments repo)**

- Delete: `ansible/roles/services/event-recorder/` (9 files)
- Modify: `ansible/deploy_it.yml:30-31`, `ansible/kubernetes.yml:272`, `ansible/build_it.yml:96-100`, `ansible/roles/common/defaults/main.yml:1023`, `ansible/example/inventory/group_vars/all.yaml:1957-1958`

---

### Task 1: Bump notifications to messaging v12

**Files:**
- Modify: `go.mod`, `go.sum`
- Modify: `main.go:12`, `api/main.go:7`, `api/v1/main.go:7`, `api/v2/main.go:7`, `api/v1/notification_request.go:9`

**Interfaces:**
- Consumes: nothing.
- Produces: a `notifications` module on `go 1.26` and `messaging/v12`, so later tasks can move event-recorder code in without changing its imports.

**Why this is safe:** the publish-side types are wire-identical between v9 and v12 — `messagetypes.go` differs only in `interface{}` vs `any` and the `model` import version, with identical JSON tags. `NewClient`, `SetupPublishing`, `PublishContextOpts`, and `JSONPublishingOpts` all have identical signatures. Only the import path changes.

- [ ] **Step 1: Create the branch**

```bash
cd /home/johnw/work/src/github.com/cyverse-de/notifications
git checkout -b bump-messaging-v12
```

- [ ] **Step 2: Record the current test baseline**

```bash
go build ./... && go test ./... 2>&1 | tail -20
```

Expected: builds and passes on v9. Save this output — Step 7 must match it.

- [ ] **Step 3: Bump the go directive and the direct dependencies**

```bash
go mod edit -go=1.26
go mod edit -droprequire=github.com/cyverse-de/messaging/v9
go mod edit -require=github.com/cyverse-de/messaging/v12@v12.0.2
go mod edit -require=github.com/cyverse-de/go-mod/otelutils@v0.0.6
```

- [ ] **Step 4: Update the five import sites**

In each of `main.go`, `api/main.go`, `api/v1/main.go`, `api/v2/main.go`, and `api/v1/notification_request.go`, change:

```go
"github.com/cyverse-de/messaging/v9"
```

to:

```go
"github.com/cyverse-de/messaging/v12"
```

The package name stays `messaging`, so no other edits are needed in these files.

- [ ] **Step 5: Resolve the module graph**

```bash
go mod tidy
```

Expected: `go.opentelemetry.io/otel*` resolves to v1.43.0, `github.com/cyverse-de/model/v10` replaces `model/v6`, and `github.com/streadway/amqp` disappears in favor of `github.com/rabbitmq/amqp091-go`.

- [ ] **Step 6: Fix otelecho if the build breaks**

```bash
go build ./...
```

If `otelecho v0.49.0` fails against otel 1.43, bump it:

```bash
go get go.opentelemetry.io/contrib/instrumentation/github.com/labstack/echo/otelecho@latest
go mod tidy && go build ./...
```

Expected: clean build. If `cyverse-de/echo-middleware/v2` also fails, bump it the same way. Do not change any application code to work around a version mismatch — bump the dependency instead.

- [ ] **Step 7: Verify tests still pass**

```bash
go test ./... 2>&1 | tail -20
golangci-lint run
```

Expected: same pass/fail set as the Step 2 baseline, and a clean lint run.

- [ ] **Step 8: Verify tracing config is unaffected**

`otelutils` v0.0.6 still honors `OTEL_TRACES_EXPORTER`, and still reads `OTEL_EXPORTER_JAEGER_ENDPOINT` when that value is `jaeger` — it just sends OTLP gRPC to it instead of Jaeger protocol. The deployments template hardcodes `OTEL_TRACES_EXPORTER: none` for this service (`ansible/roles/services/notifications/templates/k8s/notifications.yml.j2:55-56`), so no traces are exported either way. Confirm by reading that template; no code change is needed.

- [ ] **Step 9: Commit and push**

```bash
git add go.mod go.sum main.go api/main.go api/v1/main.go api/v2/main.go api/v1/notification_request.go
git commit -m "build: bump to messaging v12 and Go 1.26

Required so the event-recorder consumer code can move into this repo.
messaging v9 and v12 are built on different AMQP clients (streadway/amqp
vs rabbitmq/amqp091-go), so the consumer cannot straddle them. The
publish-side types are wire-identical between the two versions.

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
git push -u origin bump-messaging-v12
gh pr create --draft --title "Bump to messaging v12 and Go 1.26" --body "$(cat <<'EOF'
Prep for merging `event-recorder` into this service. No behavior change.

`messaging` v9 and v12 are built on different AMQP clients (`streadway/amqp`,
unmaintained since 2021, vs `rabbitmq/amqp091-go`). The event-recorder consumer
code needs the latter, and the code cannot straddle both. The publish-side types
are wire-identical between the versions — `messagetypes.go` differs only in
`interface{}` vs `any` and the `model` import version, with identical JSON tags —
so the existing `api/v1` publish call is unaffected.

Also bumps `go-mod/otelutils` to v0.0.6, matching what event-recorder already
runs. That version routes the `jaeger` exporter setting through OTLP gRPC, but
this service's deployment hardcodes `OTEL_TRACES_EXPORTER: none`, so tracing
behavior is unchanged.

Should be verified in QA before the merge PR lands on top of it.

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"
```

---

### Task 2: Capture the outgoing_json golden fixture

**Files:**
- Create (temporarily, in the event-recorder repo): `handlers/golden_capture_test.go`
- Create: `notifications/recorder/testdata/outgoing_json_golden.json`

**Interfaces:**
- Consumes: nothing.
- Produces: `recorder/testdata/outgoing_json_golden.json`, the pinned serialization of `messaging.NotificationMessage` that Task 5's test asserts against.

**Why:** `db/listings.go:136` selects `n.outgoing_json` and `formatNotification` unmarshals it straight back to API clients. The merge must not change that shape. Capture it from the current event-recorder code *before* that code is deleted.

- [ ] **Step 1: Start the merge branch**

```bash
cd /home/johnw/work/src/github.com/cyverse-de/notifications
git checkout -b merge-event-recorder   # stacked on bump-messaging-v12
mkdir -p recorder/testdata
```

- [ ] **Step 2: Write a throwaway capture test in the event-recorder repo**

Create `/home/johnw/work/src/github.com/cyverse-de/event-recorder/handlers/golden_capture_test.go`:

```go
package handlers

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	amqp "github.com/rabbitmq/amqp091-go"
)

// TestCaptureGoldenOutgoingJSON writes the current outgoing notification
// serialization to a file so the merged service can assert against it.
// This test is temporary and is not committed.
func TestCaptureGoldenOutgoingJSON(t *testing.T) {
	requestBody, err := json.Marshal(getLegacyNotificationRequest())
	if err != nil {
		t.Fatalf("unable to marshal the notification request: %s", err.Error())
	}
	delivery := amqp.Delivery{Body: requestBody, RoutingKey: FakeRoutingKey}

	databaseClient := NewMockDatabaseClient(42)
	handler := NewLegacy(databaseClient, NewMockMessagingClient())

	if err := handler.HandleMessage(context.Background(), "analysis", delivery); err != nil {
		t.Fatalf("handler returned an error: %s", err.Error())
	}

	encoded, err := json.MarshalIndent(databaseClient.savedOutgoingMessage, "", "  ")
	if err != nil {
		t.Fatalf("unable to marshal the outgoing message: %s", err.Error())
	}
	if err := os.WriteFile("/tmp/outgoing_json_golden.json", encoded, 0o600); err != nil {
		t.Fatalf("unable to write the golden file: %s", err.Error())
	}
}
```

- [ ] **Step 3: Run it to produce the fixture**

```bash
cd /home/johnw/work/src/github.com/cyverse-de/event-recorder
go test ./handlers/ -run TestCaptureGoldenOutgoingJSON -v
```

Expected: PASS, and `/tmp/outgoing_json_golden.json` exists.

- [ ] **Step 4: Copy the fixture into the notifications repo and remove the throwaway test**

```bash
cp /tmp/outgoing_json_golden.json \
  /home/johnw/work/src/github.com/cyverse-de/notifications/recorder/testdata/outgoing_json_golden.json
rm /home/johnw/work/src/github.com/cyverse-de/event-recorder/handlers/golden_capture_test.go
```

- [ ] **Step 5: Sanity-check the fixture**

Read `recorder/testdata/outgoing_json_golden.json`. It must contain the keys `deleted`, `email`, `email_template`, `message`, `payload`, `seen`, `subject`, `type`, `user`, with `message` holding `id`, `timestamp`, and `text`. The `timestamp` inside `message` must be a string of digits (milliseconds since epoch), and `type` must be `analysis`.

The event-recorder repo must be left with no uncommitted changes — verify with `git -C /home/johnw/work/src/github.com/cyverse-de/event-recorder status --short` returning nothing.

- [ ] **Step 6: Commit**

```bash
cd /home/johnw/work/src/github.com/cyverse-de/notifications
git add recorder/testdata/outgoing_json_golden.json
git commit -m "test: pin the outgoing_json wire shape from event-recorder

Captured from event-recorder's current handler before that code moves, so
the merge can prove it did not change what db/listings.go serves.

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

### Task 3: Move shared helpers into common

**Files:**
- Modify: `common/main.go`
- Modify: `api/v1/notification_request.go:18-47` (delete `fixTimestamp`), `:113-126` (call sites)
- Test: `common/main_test.go` (create)

**Interfaces:**
- Consumes: `common.FixTimestamp` (already exists in `common/times.go`).
- Produces:
  - `common.Notification` struct with fields `ID, NotificationType, User, Subject string`, `Seen, Deleted bool`, `TimeCreated time.Time`, `Message, RoutingKey string`
  - `func common.FixTimestampInMap(m map[string]any, k string) error`

**Why:** `fixTimestamp` is currently duplicated verbatim in `api/v1/notification_request.go:18` and event-recorder's `handlers/legacy.go:82`. Both the API and the recorder need it, so it belongs in `common` per the project guideline on shared utilities.

- [ ] **Step 1: Write the failing test**

Create `common/main_test.go`:

```go
package common

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFixTimestampInMap(t *testing.T) {
	tests := []struct {
		name    string
		input   map[string]any
		key     string
		want    any
		wantErr bool
	}{
		{
			name:  "an absent key is left alone",
			input: map[string]any{},
			key:   "startdate",
			want:  nil,
		},
		{
			name:  "an RFC3339 string becomes milliseconds",
			input: map[string]any{"startdate": "2020-07-07T17:59:59-07:00"},
			key:   "startdate",
			want:  "1594169999000",
		},
		{
			name:  "a numeric value is stringified then converted",
			input: map[string]any{"startdate": float64(1594169999000)},
			key:   "startdate",
			want:  "1594169999000",
		},
		{
			name:    "an unsupported type is an error",
			input:   map[string]any{"startdate": []string{"nope"}},
			key:     "startdate",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := FixTimestampInMap(tt.input, tt.key)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tt.want, tt.input[tt.key])
		})
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

```bash
go test ./common/ -run TestFixTimestampInMap -v
```

Expected: FAIL — `undefined: FixTimestampInMap`.

- [ ] **Step 3: Add the struct and the helper to `common/main.go`**

Add the `time` and `fmt` imports, then append:

```go
// Notification represents a single notification to be recorded in the database.
type Notification struct {
	ID               string
	NotificationType string
	User             string
	Subject          string
	Seen             bool
	Deleted          bool
	TimeCreated      time.Time
	Message          string
	RoutingKey       string
}

// FixTimestampInMap converts a timestamp stored under a key in a map to milliseconds since the
// epoch, leaving the map untouched if the key is absent.
func FixTimestampInMap(m map[string]any, k string) error {
	wrapMsg := fmt.Sprintf("unable to fix the timestamp in key '%s'", k)

	v, present := m[k]
	if !present {
		return nil
	}

	// Only the types the json package can produce need to be handled here.
	var stringValue string
	switch val := v.(type) {
	case string:
		stringValue = val
	case float64:
		stringValue = fmt.Sprintf("%d", int64(val))
	default:
		return fmt.Errorf("%s: %s", wrapMsg, "invalid data type")
	}

	convertedValue, err := FixTimestamp(stringValue)
	if err != nil {
		return errors.Wrap(err, wrapMsg)
	}

	m[k] = convertedValue

	return nil
}
```

- [ ] **Step 4: Run the test to verify it passes**

```bash
go test ./common/ -run TestFixTimestampInMap -v
```

Expected: PASS, all four cases.

- [ ] **Step 5: Delete the duplicate in api/v1 and repoint its call sites**

Delete `fixTimestamp` from `api/v1/notification_request.go` (lines 17–47) and change both call sites to:

```go
	// Ensure that the analysis start date is in the correct format if it's present.
	err = common.FixTimestampInMap(notificationRequest.Payload, "startdate")
	if err != nil {
		span.RecordError(err)
		return ctx.JSON(http.StatusBadRequest, model.InvalidRequestBody(err))
	}

	// Ensure that the analysis end date is in the correct format if it's present.
	err = common.FixTimestampInMap(notificationRequest.Payload, "enddate")
	if err != nil {
		span.RecordError(err)
		return ctx.JSON(http.StatusBadRequest, model.InvalidRequestBody(err))
	}
```

Then run `goimports -w api/v1/notification_request.go` — the `fmt` import may now be unused.

- [ ] **Step 6: Verify the whole package still builds and passes**

```bash
go build ./... && go test ./... && golangci-lint run
```

Expected: clean.

- [ ] **Step 7: Commit**

```bash
git add common/main.go common/main_test.go api/v1/notification_request.go
git commit -m "refactor: move fixTimestamp into common and add the Notification struct

fixTimestamp was duplicated verbatim between api/v1 and event-recorder's
handler. Both the API and the incoming recorder need it.

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

### Task 4: Move the database functions

**Files:**
- Create: `db/notifications.go`, `db/notification_types.go`, `db/notification_types_test.go`
- Modify: `db/users.go` (add `AddUser`)

**Interfaces:**
- Consumes: `common.Notification` (Task 3) and the package-level `psql` squirrel builder already in `db/main.go`.
- Produces:
  - `func db.RegisterNotificationType(ctx context.Context, tx *sql.Tx, notificationType string) error`
  - `func db.SaveNotification(ctx context.Context, tx *sql.Tx, notification *common.Notification) error`
  - `func db.SaveOutgoingNotification(ctx context.Context, tx *sql.Tx, m *messaging.NotificationMessage) error`
  - `func db.CountUnreadNotifications(ctx context.Context, tx *sql.Tx, user string) (int64, error)`
  - `func db.AddUser(ctx context.Context, tx *sql.Tx, user string) (string, error)`
  - `func db.GetOrCreateUserID(ctx context.Context, tx *sql.Tx, user string) (string, error)`
  - `func db.RequireNotificationTypeID(ctx context.Context, tx *sql.Tx, notificationType string) (string, error)`

**Name collisions — read this before touching the code.** event-recorder's `db/users.go:41` and `db/notification_types.go:15` define `GetUserID` and `GetNotificationTypeID`. This repo already has functions with those exact names and signatures (`db/users.go:13`, `db/misc.go:14`) — **but they do different things**:

| Name | event-recorder | this repo |
| --- | --- | --- |
| `GetUserID` | get-or-create; calls `AddUser` on `sql.ErrNoRows` | read-only; returns `""` for an unknown user with no error |
| `GetNotificationTypeID` | selects `id::text`; errors when absent | selects `id`; returns `""` when absent with no error |

`SaveNotification` uses both results as foreign keys, so it needs the strict, creating variants — pointing it at this repo's lenient versions would insert an empty-string `user_id` for any first-time user. This repo's versions cannot change either: six call sites in `api/v1/updates.go` and `api/v2/updates.go` depend on the `""` behavior.

Both versions therefore survive. The incoming ones are renamed to `GetOrCreateUserID` and `RequireNotificationTypeID`, which describe what they actually do.

- [ ] **Step 1: Copy the source files across**

```bash
cd /home/johnw/work/src/github.com/cyverse-de/notifications
ER=/home/johnw/work/src/github.com/cyverse-de/event-recorder
cp $ER/db/notifications.go db/notifications.go
cp $ER/db/notification_types.go db/notification_types.go
cp $ER/db/notification_types_test.go db/notification_types_test.go
```

- [ ] **Step 2: Repoint the imports**

In all three copied files, change `github.com/cyverse-de/event-recorder/common` to `github.com/cyverse-de/notifications/common` and `github.com/cyverse-de/messaging/v12` stays as-is.

- [ ] **Step 3: Rename the colliding notification-type function**

In the new `db/notification_types.go`, rename `GetNotificationTypeID` to `RequireNotificationTypeID` and update its doc comment to say it returns an error when the type is not registered. Update `RegisterNotificationType`'s internal call to match, and rename `TestGetNotificationTypeID` to `TestRequireNotificationTypeID` in `db/notification_types_test.go`.

Do **not** delete it in favor of `db/misc.go:14` — that version returns `""` with no error when the type is absent, which would put an empty string into the `notification_type_id` foreign key.

- [ ] **Step 4: Add AddUser and GetOrCreateUserID to db/users.go**

Append both to `db/users.go`, taken verbatim from event-recorder's `db/users.go:12-70` with only the second function's name changed:

```go
// AddUser adds a user to the `users` table in the notifications database, returning
// the ID assigned to the user.
func AddUser(ctx context.Context, tx *sql.Tx, user string) (string, error) {
	wrapMsg := fmt.Sprintf("unable to add `%s` to the users table", user)

	// Build the query.
	statement, args, err := sq.StatementBuilder.
		PlaceholderFormat(sq.Dollar).
		Insert("users").Columns("username").
		Values(user).
		Suffix("RETURNING id").
		ToSql()
	if err != nil {
		return "", errors.Wrap(err, wrapMsg)
	}

	// Execute the statement.
	var id string
	row := tx.QueryRowContext(ctx, statement, args...)
	err = row.Scan(&id)
	if err != nil {
		return "", errors.Wrap(err, wrapMsg)
	}

	return id, nil
}

// GetOrCreateUserID obtains the user ID for `user`, adding the user to the `users` table
// in the notifications database if necessary. This differs from GetUserID, which reports
// an unknown user as an empty string rather than creating one.
func GetOrCreateUserID(ctx context.Context, tx *sql.Tx, user string) (string, error) {
	wrapMsg := fmt.Sprintf("unable to get the user ID for `%s`", user)

	// Build the query.
	statement, args, err := sq.StatementBuilder.
		PlaceholderFormat(sq.Dollar).
		Select("id").From("users").
		Where(sq.Eq{"username": user}).
		ToSql()
	if err != nil {
		return "", errors.Wrap(err, wrapMsg)
	}

	// Query the database.
	var id string
	row := tx.QueryRowContext(ctx, statement, args...)
	err = row.Scan(&id)

	if err == nil {
		return id, nil
	}

	if errors.Is(err, sql.ErrNoRows) {
		return AddUser(ctx, tx, user)
	}

	return "", errors.Wrap(err, wrapMsg)
}
```

Note the one deliberate change from the original: `err == sql.ErrNoRows` becomes `errors.Is(err, sql.ErrNoRows)`, per the project guideline on typed error checks.

- [ ] **Step 4a: Point SaveNotification at the renamed functions**

In the new `db/notifications.go`, `SaveNotification` currently calls `GetNotificationTypeID` and `GetUserID`. Change those calls to `RequireNotificationTypeID` and `GetOrCreateUserID`. Without this, it silently binds to this repo's lenient versions and writes empty-string foreign keys.

Confirm the insert column list is unchanged: `notification_type_id`, `user_id`, `subject`, `seen`, `deleted`, `time_created`, `incoming_json`, `routing_key`.

- [ ] **Step 5: Add go-sqlmock as a test dependency**

```bash
go get github.com/DATA-DOG/go-sqlmock@v1.5.2
```

Required by the ported `db/notification_types_test.go`.

- [ ] **Step 6: Run the db tests**

```bash
go test ./db/... -v
```

Expected: PASS, including the ported `TestRegisterNotificationType` and `TestRequireNotificationTypeID`.

- [ ] **Step 6a: Prove the two lookup pairs stayed distinct**

```bash
grep -n "func GetUserID\|func GetOrCreateUserID\|func GetNotificationTypeID\|func RequireNotificationTypeID" db/*.go
```

Expected: exactly four results, one per function. If either lenient version disappeared, restore it — `api/v1/updates.go` and `api/v2/updates.go` depend on it.

- [ ] **Step 7: Build and lint**

```bash
go build ./... && golangci-lint run
```

Expected: clean.

- [ ] **Step 8: Commit**

```bash
git add db/
git commit -m "feat: move event-recorder's notification writes into db

Ports SaveNotification, SaveOutgoingNotification, CountUnreadNotifications,
RegisterNotificationType, and AddUser.

event-recorder's GetUserID and GetNotificationTypeID share a name and signature
with functions already here, but not their behavior: theirs create-if-missing
and error-if-missing respectively, while ours return an empty string. Since
SaveNotification uses both as foreign keys and api/*/updates.go depends on the
lenient behavior, both survive — the incoming pair renamed to GetOrCreateUserID
and RequireNotificationTypeID.

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

### Task 5: Move the recorder, with publishes after commit

**Files:**
- Create: `recorder/recorder.go`, `recorder/errors.go`, `recorder/clients.go`
- Create: `recorder/recorder_test.go`, `recorder/errors_test.go`

**Interfaces:**
- Consumes: `common.Notification`, `common.FixTimestampInMap` (Task 3); the `db` functions from Task 4; `recorder/testdata/outgoing_json_golden.json` (Task 2).
- Produces:
  - `type recorder.Recorder` with `func (r *Recorder) Record(ctx context.Context, updateType string, body []byte, routingKey string) error`
  - `func recorder.New(dbc DatabaseClient, messagingClient MessagingClient) *Recorder`
  - `recorder.DatabaseClient` and `recorder.MessagingClient` interfaces (same method sets as event-recorder's `handlers/main.go:23-38`)
  - `func recorder.NewDatabaseClient(db *sql.DB) DatabaseClient` — the default `DatabaseClient`, wrapping the `db` package functions; Task 6 calls this from `main.go`
  - `recorder.RecoverableError`, `recorder.UnrecoverableError`, `recorder.NewRecoverableError`, `recorder.NewUnrecoverableError`

Note that `DatabaseClientImpl`'s methods must call the renamed `db.GetOrCreateUserID` / `db.RequireNotificationTypeID` indirectly — they call `db.SaveNotification` and `db.RegisterNotificationType`, which Task 4 already repointed. No further change is needed here, but verify it after Task 4 lands.

**Signature change from event-recorder:** `HandleMessage(ctx, updateType, delivery amqp.Delivery)` becomes `Record(ctx, updateType, body []byte, routingKey string)`. This keeps `amqp091-go` out of the recording logic so only `consumer.go` imports it, and it makes the recorder directly testable without constructing a delivery.

**The behavior change.** Today the email request (`handlers/legacy.go:219`) and the UI notification (`:250`) are published *before* the commit at `:256`, so a commit failure sends the email and then requeues, sending it again on retry. The new order commits first, then publishes.

This trades one failure mode for another and the plan makes the choice explicit: **after a successful commit, a publish failure is logged and swallowed, not returned.** Returning an error post-commit would requeue the delivery and reprocess it, writing a duplicate notification row. The accepted cost is that a publish outage can leave a recorded notification the user never gets pinged about — visible in logs, and not silently duplicated.

Email *validation* must still happen before the commit, so a bad address is still an `UnrecoverableError` that discards the delivery instead of committing a row first. Split the current `sendEmailRequest` into a `buildEmailRequest` (validates, returns `UnrecoverableError`) called pre-commit, and the publish call made post-commit.

- [ ] **Step 1: Copy the sources across and repoint imports**

```bash
ER=/home/johnw/work/src/github.com/cyverse-de/event-recorder
cp $ER/handlers/errors.go recorder/errors.go
cp $ER/handlers/errors_test.go recorder/errors_test.go
cp $ER/handlers/legacy.go recorder/recorder.go
cp $ER/handlers/main.go recorder/clients.go
cp $ER/handlers/legacy_test.go recorder/recorder_test.go
```

In each, change `package handlers` to `package recorder` and `github.com/cyverse-de/event-recorder/...` imports to `github.com/cyverse-de/notifications/...`. Delete `createMessagingClient` and `InitMessageHandlers` from `recorder/clients.go` — `main.go` wires those in Task 6 — and delete the now-unused `MessageHandler` interface.

- [ ] **Step 2: Rename the type and entry point**

In `recorder/recorder.go`, rename `Legacy` to `Recorder`, `NewLegacy` to `New`, and change the entry point signature:

```go
// Record writes an incoming notification request to the database and publishes the outgoing
// email and UI messages.
func (r *Recorder) Record(ctx context.Context, updateType string, body []byte, routingKey string) error {
```

Replace every `delivery.Body` with `body` and every `delivery.RoutingKey` with `routingKey`, and drop the `amqp091-go` import.

- [ ] **Step 3: Write the failing test for the new ordering**

Add to `recorder/recorder_test.go`. This needs the mock database client to be able to fail its commit, so first add a field to `MockDatabaseClient`:

```go
// CommitErr, when set, makes Commit fail so post-commit behavior can be tested.
CommitErr error
```

and change `Commit` to:

```go
func (c *MockDatabaseClient) Commit(*sql.Tx) error {
	c.CommitCalled = true
	return c.CommitErr
}
```

Then add the test:

```go
func TestPublishesHappenAfterCommit(t *testing.T) {
	assert := assert.New(t)

	body, err := json.Marshal(getLegacyNotificationRequest())
	if err != nil {
		t.Fatalf("unable to marshal the notification request: %s", err.Error())
	}

	databaseClient := NewMockDatabaseClient(42)
	databaseClient.CommitErr = errors.New("commit failed")
	messagingClient := NewMockMessagingClient()
	r := New(databaseClient, messagingClient)

	err = r.Record(context.Background(), "analysis", body, FakeRoutingKey)

	assert.Error(err, "a failed commit must be reported")
	assert.Nil(messagingClient.PublishedEmailRequest,
		"no email may be published when the transaction did not commit")
	assert.Nil(messagingClient.PublishedNotificationMessage,
		"no UI notification may be published when the transaction did not commit")
}
```

- [ ] **Step 4: Run it to verify it fails**

```bash
go test ./recorder/ -run TestPublishesHappenAfterCommit -v
```

Expected: FAIL — both publishes happen before the commit today, so both assertions on `Nil` fail.

- [ ] **Step 5: Reorder Record so publishes follow the commit**

Restructure the body of `Record` to this order:

```go
	// Validate the email request before committing anything, so a bad address discards the
	// delivery instead of leaving a committed row behind.
	var emailRequest *messaging.EmailRequest
	if request.Email {
		emailRequest, err = r.buildEmailRequest(&request)
		if err != nil {
			return err
		}
	}

	// ... RegisterNotificationType, SaveNotification, buildNotificationMessage,
	//     SaveOutgoingNotification, CountUnreadNotifications ...

	if err = r.dbc.Commit(tx); err != nil {
		return NewRecoverableError("unable to commit the database transaction: %s", err.Error())
	}

	// Publishing after the commit means a publish failure cannot requeue the delivery and
	// write a duplicate row. The cost is a notification the user is never pinged about, which
	// is logged rather than retried.
	if emailRequest != nil {
		if err := r.messagingClient.PublishEmailRequestContext(ctx, emailRequest); err != nil {
			log.Errorf("notification %s was recorded but its email request could not be published; "+
				"the AMQP exchange is probably unreachable: %s", storableRequest.ID, err.Error())
		}
	}
	if err := r.messagingClient.PublishNotificationMessageContext(ctx, wrappedNotificationMessage); err != nil {
		log.Errorf("notification %s was recorded but could not be published to the UI; "+
			"the AMQP exchange is probably unreachable: %s", storableRequest.ID, err.Error())
	}

	return nil
```

Split the existing `sendEmailRequest` into `buildEmailRequest`, which performs the same validation and returns `(*messaging.EmailRequest, error)` without publishing.

Add a package-level logger to `recorder/recorder.go` matching this repo's convention — check how `main.go` and `api/` construct theirs and follow suit.

- [ ] **Step 6: Run the test to verify it passes**

```bash
go test ./recorder/ -run TestPublishesHappenAfterCommit -v
```

Expected: PASS.

- [ ] **Step 7: Consolidate the four ported tests into one table-driven test**

The ported `TestNotification`, `TestNotificationWithoutEmail`, `TestNotificationWithoutMessage`, and `TestNotificationWithUpperCaseUpdateType` are near-identical. Replace them with a single table-driven test whose cases are: the full happy path; `email: false`; empty `message` (text falls back to subject); and `updateType: "ANALYSIS"` (lowercased to `analysis`). Each case asserts on the fields the original assertions covered — begin/commit called, registered type, saved notification type/user/routing key, saved outgoing message id and timestamp format, email presence, published notification id/total/timestamp, and payload `startdate` format with `enddate` absent.

- [ ] **Step 8: Add the golden fixture test**

```go
func TestOutgoingJSONShapeIsUnchanged(t *testing.T) {
	body, err := json.Marshal(getLegacyNotificationRequest())
	if err != nil {
		t.Fatalf("unable to marshal the notification request: %s", err.Error())
	}

	databaseClient := NewMockDatabaseClient(42)
	r := New(databaseClient, NewMockMessagingClient())
	if err := r.Record(context.Background(), "analysis", body, FakeRoutingKey); err != nil {
		t.Fatalf("unexpected error: %s", err.Error())
	}

	actual, err := json.MarshalIndent(databaseClient.savedOutgoingMessage, "", "  ")
	if err != nil {
		t.Fatalf("unable to marshal the outgoing message: %s", err.Error())
	}

	expected, err := os.ReadFile("testdata/outgoing_json_golden.json")
	if err != nil {
		t.Fatalf("unable to read the golden file: %s", err.Error())
	}

	assert.JSONEq(t, string(expected), string(actual),
		"outgoing_json is a wire contract read by db/listings.go; its shape must not change")
}
```

- [ ] **Step 9: Run the full recorder suite**

```bash
go test ./recorder/ -v
golangci-lint run
```

Expected: all PASS, clean lint. If the golden test fails, the merge changed the wire shape — fix the code, not the fixture.

- [ ] **Step 10: Commit**

```bash
git add recorder/
git commit -m "feat: move the recording logic in as the recorder package

Record() replaces HandleMessage(), taking the body and routing key instead of
an amqp.Delivery so the AMQP client stays confined to the consumer.

Publishes now happen after the transaction commits. Previously a commit failure
would requeue a delivery whose email had already gone out, sending it twice. A
post-commit publish failure is logged rather than returned, since returning it
would requeue and write a duplicate row. Email validation still runs before the
commit so a bad address discards the delivery instead of committing first.

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

### Task 6: Move the consumer and wire it into main

**Files:**
- Create: `recorder/consumer.go`
- Modify: `main.go`

**Interfaces:**
- Consumes: `recorder.Recorder`, `recorder.RecoverableError`, `recorder.UnrecoverableError` (Task 5).
- Produces:
  - `func recorder.NewConsumer(client *messaging.Client, amqpSettings *common.AMQPSettings, supportEmail string, r *Recorder) *Consumer`
  - `func (c *Consumer) Listen() error`
  - `func (c *Consumer) Close()`
  - `const recorder.QueueName = "event_listener"`, `const recorder.RoutingKey = "events.*.update.*"`

`NewConsumer` takes an already-constructed client rather than dialing its own, so `main.go` owns every connection's lifetime. It therefore has no failure mode of its own and returns no error — unlike event-recorder's `handlerset.New`, which dialed internally.

**Preserve exactly:** queue name `event_listener`, binding key `events.*.update.*`, prefetch 100, the category/update-type parse (components 1 and 3 of the routing key), the ack/nack semantics, and the `notifications_event_discarded` support email on unrecoverable errors. A change to any of these breaks the safe cutover.

- [ ] **Step 1: Copy the consumer across**

```bash
cp /home/johnw/work/src/github.com/cyverse-de/event-recorder/handlerset/main.go recorder/consumer.go
```

Change `package handlerset` to `package recorder`, repoint imports to `github.com/cyverse-de/notifications/...`, rename `HandlerSet` to `Consumer` and `New` to `NewConsumer` (the recorder's `New` already occupies that name), and export `queueName`/`queueKey` as `QueueName`/`RoutingKey`.

- [ ] **Step 2: Replace the handler map with the single recorder**

The `handlerFor map[string]handlers.MessageHandler` indirection existed to dispatch by category, but only `notification` was ever registered. Replace the field with `recorder *Recorder` and the dispatch in `handleMessage` with:

```go
	category, updateType, err := c.parseRoutingKey(delivery.RoutingKey)
	if err != nil {
		log.Errorf("unable to handle message: %s", err.Error())
		c.nack(delivery, false)
		return
	}

	// The binding admits events.*.update.*, but this service only records notifications.
	if category != "notification" {
		log.Infof("no handler for category '%s'; ignoring delivery", category)
		c.ack(delivery)
		return
	}

	err = c.recorder.Record(ctx, updateType, delivery.Body, delivery.RoutingKey)
```

Keep the error switch below it exactly as it is, but convert it from a type switch to `errors.As` checks so it matches the project's typed-error guideline:

```go
	if err != nil {
		var unrecoverable UnrecoverableError
		var recoverable RecoverableError
		switch {
		case errors.As(err, &unrecoverable):
			log.Errorf("discarding message because of an unrecoverable error: %s", err.Error())
			c.sendUnrecoverableErrorEmail(ctx, delivery, unrecoverable)
			c.logDelivery("discarded delivery", delivery)
			c.nack(delivery, false)
		case errors.As(err, &recoverable):
			log.Errorf("requeuing message because of a recoverable error: %s", err.Error())
			c.logDelivery("requeued delivery", delivery)
			c.nack(delivery, true)
		default:
			log.Errorf("requeuing message because of an error that is presumed to be recoverable: %s", err.Error())
			c.logDelivery("requeued delivery", delivery)
			c.nack(delivery, true)
		}
		return
	}
	c.ack(delivery)
```

Note this also fixes the `becuse` typo in the original log message at `handlerset/main.go:135`.

- [ ] **Step 3: Decide the reconnect flag**

event-recorder builds its consumer client with `messaging.NewClient(uri, true)` (`handlerset/main.go:39`), while `job-status` deliberately uses `false`, commenting that the library's reconnect path does not reestablish consumers so a lost connection should exit and let Kubernetes restart the pod.

Read `messaging/v12@v12.0.2/amqp.go` around the reconnect handling and determine whether consumers are reestablished after a reconnect.

- If they are **not**: pass `false` in Step 4 and note in the commit message that event-recorder had a latent bug where a reconnect left it silently not consuming.
- If they **are**: pass `true` in Step 4, matching event-recorder's current behavior.

Record which you found and why in the commit message either way — the next person should not have to re-derive it.

- [ ] **Step 4: Wire the consumer into main.go**

In `main.go`, after the existing messaging client and database are set up, add the following, substituting the boolean determined in Step 3 for `reconnectConsumers`:

```go
	// The recorder consumes notification events published by the v1 API and records them.
	const reconnectConsumers = false // or true — see Step 3

	supportEmail := cfg.GetString("email.request")
	consumerClient, err := messaging.NewClient(amqpSettings.URI, reconnectConsumers)
	if err != nil {
		log.Fatalf("unable to create the consumer messaging client: %s", err.Error())
	}
	defer consumerClient.Close()

	rec := recorder.New(recorder.NewDatabaseClient(db), amqpClient)
	consumer := recorder.NewConsumer(consumerClient, amqpSettings, supportEmail, rec)
	defer consumer.Close()

	if err := consumer.Listen(); err != nil {
		log.Fatalf("unable to start consuming events: %s", err.Error())
	}
```

Inline the constant at its single use site rather than leaving a named constant, if that reads better in context. The already-constructed `amqpClient` is the recorder's publisher — do not create a third client.

`email.request` is already present in this service's deployed config — the `jobservices.yml.j2` templates for `notifications` and `event-recorder` are byte-identical — so no configuration change is required.

- [ ] **Step 5: Verify it builds and the suite passes**

```bash
go build ./... && go test ./... && golangci-lint run
```

Expected: clean.

- [ ] **Step 6: Verify the queue parameters against the running service**

Read `recorder/consumer.go` and confirm against `event-recorder/handlerset/main.go:19-20,167-174` that queue name, binding key, and prefetch are byte-identical. The cutover depends on the merged service joining the *same* queue as the running event-recorder pods.

- [ ] **Step 7: Commit and push, then open the draft PR**

```bash
git add recorder/consumer.go main.go
git commit -m "feat: consume the event_listener queue in-process

Moves event-recorder's handlerset in as recorder.Consumer and starts it
alongside the Echo server. Queue name, binding key, and prefetch are unchanged
so the merged service and any still-running event-recorder pods are competing
consumers on the same queue during cutover.

Drops the category dispatch map, which only ever had one entry, and converts the
error type switch to errors.As.

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
git push -u origin merge-event-recorder
gh pr create --draft --base bump-messaging-v12 --title "Merge event-recorder into notifications" --body "$(cat <<'EOF'
Absorbs the `event-recorder` service into this one, so the DE runs one
deployment instead of two.

Stacked on the messaging v12 bump PR — review that one first. (Replace this
line with the actual `#NNN` reference once PR 1 has a number.)

The two services were already two halves of one system split across an AMQP
queue: this service's v1 API publishes `events.notification.update.<type>`, and
event-recorder was the only consumer, writing to the same `notifications`
database this service reads. They also duplicated `fixTimestamp`,
`common/times.go`, `AMQPSettings`, and `ValidateEmailAddress`.

Two `db` functions looked like duplicates but were not: event-recorder's
`GetUserID` and `GetNotificationTypeID` share a name and signature with ours
while creating-if-missing and erroring-if-missing respectively, where ours
return an empty string. Both survive, the incoming pair renamed to
`GetOrCreateUserID` and `RequireNotificationTypeID`. Collapsing them would have
written empty-string foreign keys for first-time users.

The queue stays. Callers are unaffected — an audit found only `app-exposer`
retries a failed POST, so collapsing the hop would have turned notification
delivery into a hard dependency for `apps`, `data-info`, `terrain`, and
`requests`.

**One intentional behavior change:** the email and UI messages are now published
after the transaction commits. Previously a commit failure requeued a delivery
whose email had already been sent, delivering it twice. A post-commit publish
failure is now logged rather than returned, because returning it would requeue
and write a duplicate row.

`recorder/testdata/outgoing_json_golden.json` pins the `outgoing_json`
serialization captured from event-recorder before the move — that column is a
wire contract read by `db/listings.go:136`.

Cutover is safe to do incrementally: the merged service joins the same
`event_listener` queue with the same binding, so it and any still-running
event-recorder pods are competing consumers. Drain the queue, scale
event-recorder to zero, then land the deployments cleanup.

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"
```

---

### Task 7: Delete the duplicates and verify the whole service

**Files:**
- Modify: whatever Steps 1–2 turn up

**Interfaces:**
- Consumes: everything from Tasks 3–6.
- Produces: no new symbols.

- [ ] **Step 1: Hunt for anything still duplicated**

```bash
grep -rn "func FixTimestamp\|func FixTimestampInMap\|func ValidateEmailAddress\|func fixTimestamp" --include=*.go .
```

Expected: exactly one definition of each, all in `common/`. If `recorder/` redefines any of them, delete the copy and use the `common` version.

```bash
grep -rn "func GetUserID\|func GetOrCreateUserID\|func GetNotificationTypeID\|func RequireNotificationTypeID" --include=*.go .
```

Expected: exactly four, all in `db/`. These are deliberately *not* deduplicated — see Task 4.

- [ ] **Step 2: Confirm no event-recorder import paths survived**

```bash
grep -rn "cyverse-de/event-recorder" --include=*.go . && echo "FOUND — fix these" || echo "clean"
```

Expected: `clean`.

- [ ] **Step 3: Run everything**

```bash
gofmt -l . && goimports -l . && go build ./... && go test ./... && golangci-lint run
```

Expected: no files listed by `gofmt`/`goimports`, clean build, all tests pass, clean lint.

- [ ] **Step 4: Verify the binary starts and rejects bad config**

```bash
go run . --help
```

Expected: usage output including `--config`, `--port`, and `--debug`.

- [ ] **Step 5: Commit any cleanups**

```bash
git add -A
git commit -m "chore: remove leftover duplication after the merge

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
git push
```

---

### Task 8: Update the service README

**Files:**
- Modify: `README.md`

**Interfaces:**
- Consumes: nothing.
- Produces: nothing.

- [ ] **Step 1: Describe both halves**

The README currently reads only "This service provides the RESTful API for the revised notification system." Extend it to say the service also consumes the `event_listener` queue (`events.*.update.*`), records notifications in the `notifications` database, and publishes the outgoing email and UI messages — the work that used to live in `event-recorder`. Keep it to a short paragraph plus a bullet per component, matching the tone of the existing text.

- [ ] **Step 2: Commit and push**

```bash
git add README.md
git commit -m "docs: describe the recorder half of the service

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
git push
```

---

### Task 9: Remove event-recorder from deployments

**Files:**
- Delete: `ansible/roles/services/event-recorder/` (9 files)
- Modify: `ansible/deploy_it.yml:30-31`, `ansible/kubernetes.yml:272`, `ansible/build_it.yml:96-100`, `ansible/roles/common/defaults/main.yml:1023`, `ansible/example/inventory/group_vars/all.yaml:1957-1958`

**Interfaces:**
- Consumes: nothing from earlier tasks.
- Produces: nothing.

**Do not merge this PR until the cutover is complete** — the merged `notifications` is deployed, the `event_listener` queue has drained, and `event-recorder` has been scaled to zero and confirmed idle.

- [ ] **Step 1: Branch**

```bash
cd /home/johnw/work/src/github.com/cyverse-de/deployments
git checkout -b remove-event-recorder
```

- [ ] **Step 2: Find every reference**

```bash
grep -rn "event-recorder\|event_recorder" ansible/ | grep -v "^ansible/roles/services/event-recorder/"
```

Expected, matching the plan header: `deploy_it.yml:30-31`, `kubernetes.yml:272`, `build_it.yml:96-100`, `roles/common/defaults/main.yml:1023`, `example/inventory/group_vars/all.yaml:1957-1958`. If the grep returns anything else, handle it too and note it in the PR body.

- [ ] **Step 3: Remove the role directory**

```bash
git rm -r ansible/roles/services/event-recorder
```

- [ ] **Step 4: Remove the five reference sites**

- `ansible/deploy_it.yml` — delete the `- role: services/event-recorder` entry and its `tags: event-recorder` line.
- `ansible/kubernetes.yml` — delete the `- role: services/event-recorder` line.
- `ansible/build_it.yml` — delete the whole `name: services/event-recorder` block including its `tags`.
- `ansible/roles/common/defaults/main.yml` — delete the `- event-recorder` list entry. Keep the list alphabetized (it already is).
- `ansible/example/inventory/group_vars/all.yaml` — delete the `# event-recorder` comment and the `# event_recorder_replicas: 2` line.

- [ ] **Step 5: Verify nothing references it**

```bash
grep -rn "event-recorder\|event_recorder" ansible/ && echo "FOUND — fix these" || echo "clean"
```

Expected: `clean`.

- [ ] **Step 6: Syntax-check the playbooks**

```bash
ansible-playbook --syntax-check ansible/deploy_it.yml ansible/build_it.yml ansible/kubernetes.yml
```

Expected: no errors. If the syntax check needs an inventory or vault this environment lacks, say so in the PR body rather than skipping silently.

- [ ] **Step 7: Commit, push, and open the draft PR**

```bash
git add -A
git commit -m "chore: remove the event-recorder service

Its logic now lives in the notifications service, which consumes the same
event_listener queue. Merge only after the cutover is confirmed: notifications
deployed, queue drained, event-recorder scaled to zero.

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
git push -u origin remove-event-recorder
gh pr create --draft --title "Remove the event-recorder service" --body "$(cat <<'EOF'
Removes the `event-recorder` role and its five reference sites. Its logic moves
into the `notifications` service, which consumes the same `event_listener` queue
with the same binding.

**Do not merge until the cutover is confirmed:** merged `notifications` deployed,
`event_listener` drained, `event-recorder` scaled to zero and idle. Until then
the running deployment is still doing the work.

Companion to the notifications-repo merge PR.

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"
```

---

## Post-merge, outside this plan

- Archive the `event-recorder` repo on GitHub once PR 3 lands.
- Delete the `event_listener` queue only if RabbitMQ is ever fully drained of it; the merged service keeps using it, so this is *not* a cleanup step — noted here only to prevent someone deleting it by mistake.
- `requests/clients/notificationagent/main.go:16` builds its `http.Client` with no `Timeout`, against the project Go guideline. Pre-existing and unrelated to this merge, but worth a ticket.
- The merged service inherits `notifications`' hardcoded `OTEL_TRACES_EXPORTER: none`, while `event-recorder` read the value from the `configs` secret. In an environment with `jaeger_enabled: true`, recorder spans that used to be exported will stop being exported. If that matters, change `ansible/roles/services/notifications/templates/k8s/notifications.yml.j2:55-56` to read from the secret the way every other service does.
