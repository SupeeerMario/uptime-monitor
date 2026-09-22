# Uptime Monitor API

A concurrent HTTP uptime monitor written in Go, Gin, pgx and Postgres. Register a URL with a
check interval, and it probes that URL on schedule, records every result — status code, latency,
transport error — and keeps the history in Postgres behind a small JSON API.

The interesting part is not the CRUD. It is that **one hung endpoint cannot stall the others**,
and there is a test that fails if that stops being true.

## Features

- **Register monitors over HTTP** — URL, check interval and expected status, validated on the way in.
- **Scheduler on a ticker** — asks Postgres which monitors are due and dispatches them.
- **Worker pool** — `PROBER_COUNT` goroutines share one buffered jobs channel, each holding its own `http.Client` with a 10s timeout.
- **Every probe is a row** — successes, non-2xx responses and connection failures all land in `checks`.
- **Due-monitor selection in SQL** — `last_checked_at + interval <= now()`, so the database decides what is due.
- **In-flight tracking** — a monitor already being probed is skipped on later ticks instead of queued twice.
- **Uptime computed in SQL** — one grouped left join, no N+1 and no denormalised counter to keep honest.
- **Paginated check history** — newest first, straight off the covering index.
- **Graceful shutdown** — on SIGINT/SIGTERM the scheduler stops, workers drain, results are written, and only then does the pool close.
- **One-command environment** — Docker Compose brings up Postgres, runs migrations, and starts the app with live reload.

## Architecture

```mermaid
flowchart TD
    client([Client]) -->|"HTTP JSON"| gin["Gin router<br/>internal/api"]
    gin --> store["Store<br/>internal/store"]

    sched["Scheduler<br/>ticker"] -->|"ListDueMonitors"| store
    sched -->|"DueMonitor"| jobs[["jobs channel"]]
    jobs --> worker["HTTP worker pool<br/>internal/prober"]
    worker -->|"GET url, timed"| target([Monitored endpoint])
    worker -->|"Result"| results[["results channel"]]
    results --> writer["Writer goroutine<br/>cmd/pulse"]
    writer -->|"SaveCheck"| store
    writer -->|"monitor id"| schedDone[["schedDone channel"]]
    schedDone -->|"clear in-flight"| sched

    store --> pg[("Postgres<br/>monitors, checks")]

    subgraph compose["Docker Compose"]
        gin
        sched
        worker
        writer
        store
        pg
        migrate["migrate/migrate<br/>runs migrations"]
    end

    migrate --> pg
```

### The check pipeline

1. The **scheduler** ticks on its interval and queries `monitors` for rows whose
   `last_checked_at + interval_seconds` has passed, or that were never checked.
2. Each due monitor is sent on the **jobs channel**, unless it is already in flight.
3. An **HTTP worker** takes the job, issues a `GET` with a 10s client timeout, and times the
   request, producing a `Result` with a nullable status code and a nullable error string.
4. The result goes on the **results channel**.
5. The **writer goroutine** calls `SaveCheck`, which inserts the `checks` row and stamps
   `monitors.last_checked_at` in a single statement, then signals the monitor id back on
   **schedDone** so the scheduler clears it from the in-flight set.

The job send happens *inside* the scheduler's `select`, not before it. That matters: with a
blocking send outside the select, a full jobs channel makes the scheduler deaf to `schedDone`
and `ctx.Done()` — and a worker waiting to report completion deadlocks against it. The Go
runtime never notices, because the HTTP server is still alive.

### Why an in-flight set

Postgres only knows what *finished*. `last_checked_at` is written at completion, so a probe that
is currently running looks identical to one that never started — and gets dispatched again on the
next tick. The scheduler keeps an in-memory set of ids it has handed out and clears each one when
the writer reports it done. It lives in a single goroutine, so it needs no mutex.

Writing `last_checked_at` at completion rather than dispatch is deliberate: the column stays
honest, and it costs no extra round trip because the writer is already touching that row. A crash
with probes in flight leaves those monitors looking overdue, so they get re-checked. That is the
cheapest failure mode available.

### Shutdown

A signal cancels the scheduler and prober contexts. The scheduler closes the jobs channel on its
way out; each worker finishes the probe it already holds, reports the result, and returns. A
`WaitGroup` tracks them, the results channel closes once they are gone, and `main` waits on a
done channel from the writer before closing the pgx pool — so every result that was produced
gets written, not dropped.

Jobs still sitting in the channel unstarted are abandoned, which costs nothing: `last_checked_at`
was never stamped for them, so they simply look overdue on the next boot.

A probe that is still mid-request at shutdown is a different case. See
[Known limitations](#known-limitations).

## Tech stack

| Layer | Choice |
| --- | --- |
| Language | Go 1.26 |
| HTTP framework | [Gin](https://github.com/gin-gonic/gin) |
| Database | Postgres 17 |
| Driver | [pgx/v5](https://github.com/jackc/pgx) with `pgxpool` |
| Migrations | [golang-migrate](https://github.com/golang-migrate/migrate) |
| Concurrency | goroutines, buffered channels, `context`, `sync.WaitGroup` |
| Live reload | [air](https://github.com/air-verse/air) |
| Runtime | Docker Compose |

## Quick start

```bash
git clone https://github.com/supeeermario/uptime-monitor.git
cd uptime-monitor
cp .env.example .env
docker compose up --build
```

Compose waits for Postgres to pass its healthcheck, runs the migrations to completion, and then
starts the API on `http://localhost:8000`. Postgres is published on host port `5433`.

```bash
curl localhost:8000/healthz
# {"message":"Uptime monitor is running","status":"healthy"}
```

## Configuration

Every value is read once at startup by `internal/config`. A missing or invalid one is fatal —
nothing downstream calls `os.Getenv`.

| Key | Example | Notes |
| --- | --- | --- |
| `DB_NAME` | `uptime-monitor` | Database created by the Postgres container |
| `DB_USERNAME` | `uptime_monitor` | Postgres role |
| `DB_PASSWORD` | `change_me` | Postgres password |
| `DB_PORT` | `5432` | Postgres port *inside* the Compose network. The published host port is `5433`. |
| `DB_URL` | `postgres://…@localhost:5433/…` | Only used when running the binary on the host; inside Compose, `go-app` is handed a `DB_URL` built from the values above |
| `PORT` | `8000` | HTTP listen port. Must be a positive integer. |
| `PROBER_COUNT` | `5` | Size of the probe worker pool. Must be a positive integer. |

`PROBER_COUNT` is the number of endpoints that can be checked simultaneously, and therefore the
number of simultaneous hangs the system absorbs before healthy monitors start queueing.

Compose has no `env_file:` directive — `.env` is read for `${VAR}` interpolation only.

## API

| Method | Path | Purpose |
| --- | --- | --- |
| `GET` | `/healthz` | Liveness check |
| `POST` | `/monitors` | Register a monitor |
| `GET` | `/monitors` | List monitors with uptime |
| `GET` | `/monitors/:id/checks` | Paginated probe history, newest first |
| `DELETE` | `/monitors/:id` | Delete a monitor and its checks |
| `GET` | `/duemonitors` | **Debug route.** Monitors currently due for a probe |

### Create a monitor

```bash
curl -i -X POST localhost:8000/monitors \
  -H 'Content-Type: application/json' \
  -d '{"url":"https://example.com","interval_seconds":30,"expected_status":200}'
```

```http
HTTP/1.1 201 Created
Location: /monitors/1
```

`url` must be a valid URL, `interval_seconds` greater than zero, and `expected_status` between
100 and 599; anything else returns `422` with the validation error. The same rules exist as
`CHECK` constraints in the schema, so a bad row is impossible even if the handler is bypassed.

### List monitors

```bash
curl localhost:8000/monitors
```

```json
[
  {
    "id": 1,
    "url": "https://example.com",
    "interval_seconds": 30,
    "expected_status": 200,
    "last_checked_at": "2026-09-20T12:00:05Z",
    "created_at": "2026-09-20T11:59:30Z",
    "uptime_percent": 99.4
  }
]
```

`uptime_percent` is computed in SQL — a left join from `monitors` onto `checks`, grouped by
monitor id, counting checks whose `error` is null and whose status matches the monitor's
`expected_status`, over the total check count.

Three details that are easy to get wrong here:

- `count(checks.id)`, not `count(*)`, as the denominator. A left join gives a never-probed
  monitor one all-null row, and `count(*)` would score it **0%** instead of unknown.
- `NULLIF` on the denominator, so zero checks divides by null rather than raising.
- `LEFT` join, not inner, or never-probed monitors disappear from the list entirely.

A monitor with no checks yet returns `null`, not `0`.

### Check history

```bash
curl 'localhost:8000/monitors/1/checks?limit=3&offset=0'
```

```json
[
  {
    "id": 1180,
    "monitor_id": 1,
    "status_code": 200,
    "total_latency_ms": 143,
    "error": null,
    "created_at": "2026-09-20T12:00:05Z"
  }
]
```

Ordered `created_at DESC`, which is exactly the `(monitor_id, created_at DESC)` index — the
query seeks and reads, it never sorts.

`limit` defaults to 50 and is silently capped at 200. A non-numeric or sub-1 `limit`, or a
negative `offset`, returns `400` *before* the query runs — a negative `LIMIT` is a Postgres
error, so without that guard a bad request would surface as a 500.

An unknown monitor id returns `200 []`, not `404`. One round trip instead of two, and the client
got the id from `/monitors` in the first place.

`status_code` and `error` are both nullable and mutually exclusive: a refused connection records
an error with no status, a completed request records a status with no error.

### Delete a monitor

```bash
curl -i -X DELETE localhost:8000/monitors/1
# HTTP/1.1 204 No Content
```

The `checks` foreign key cascades, so a monitor's history goes with it. An unknown id returns
`404`.

## Tests

```bash
go test -race ./...
```

Run these **on the host, not in the container** — `-race` requires cgo, and the Alpine image has
no C compiler.

### What is tested, and why that one thing

`internal/scheduler/scheduler_test.go` asserts the property the whole design exists for: **a
jammed endpoint does not stall the pipeline.**

It stands up two `httptest` servers — one that sleeps for 2 seconds and never answers during the
test, one that answers instantly and increments a counter. A fake `DueLister` feeds the scheduler
four monitors, one pointed at the jammed server and three at the healthy one. The real scheduler
and the real workers run, ticking every 50ms for a second, and the test then asserts the healthy
counter cleared 10.

Anything above 3 proves the healthy monitors were dispatched *more than once each*. If the jam
had blocked the pipeline, they would have been dispatched once, never released from the in-flight
set, and never seen again.

**The test is proven to fail.** Dropping the worker pool from 3 to 1 takes the counter to 0 — the
single worker is owned by the jam for its full 2 seconds. An assertion that cannot fail is
decoration, so that check is the point.

No database, no network, about 3 seconds under `-race`. The runtime is dominated by
`httptest.Server.Close()`, which blocks until in-flight requests finish.

### The interface that made it testable

`scheduler.Scheduler` originally held a concrete `*store.Store`, so testing it meant running
Postgres. It now takes a `DueLister` — a one-method interface declared in the **consuming**
package. `*store.Store` satisfies it implicitly and has no idea it exists; `store` never imports
`scheduler`, and the dependency arrow points one way.

`SaveCheck` needed no such treatment: the scheduler never calls it. The writer loop in `main`
does, and the test writes its own writer loop.

The tick interval moved out of the scheduler's body into a parameter for the same reason — the
test ticks in milliseconds, `main` passes 5 seconds. It is deliberately *not* an env var like
`PROBER_COUNT`, because it never varies per environment.

## Database schema

```sql
CREATE TABLE monitors(
    id BIGSERIAL PRIMARY KEY,
    url TEXT NOT NULL,
    interval_seconds INT CHECK(interval_seconds > 0) NOT NULL,
    expected_status INT CHECK(expected_status BETWEEN 100 AND 599) NOT NULL,
    last_checked_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE checks (
    id BIGSERIAL PRIMARY KEY,
    monitor_id BIGINT NOT NULL,
    status_code INT CHECK(status_code BETWEEN 100 AND 599) NULL,
    total_latency_ms BIGINT NULL,
    error TEXT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT fk_checks_monitor
        FOREIGN KEY (monitor_id)
        REFERENCES monitors(id)
        ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS checks_monitor ON checks(monitor_id, created_at DESC);
```

- `last_checked_at` is nullable. Null means never checked, which makes it due on the next tick —
  no sentinel date, no special case in the query.
- Durations are `BIGINT` milliseconds, not `INTERVAL`. `avg()` works on both, so that was never
  the argument — the cost is the trip through Go. `BIGINT` scans straight to `int64`; `INTERVAL`
  hands pgx a months/days/microseconds triple needing hand-written conversion on every insert and
  every read. The unit lives in the column name, so `143` is never ambiguous.
- `timestamptz` everywhere an event happened — an instant, not a wall clock reading.
- The index is `(monitor_id, created_at DESC)`: exact-match column first, sort column second.

## Project layout

```
cmd/pulse/main.go          wiring: pool, router, scheduler, workers, writer, shutdown
internal/api/              Gin handlers and request binding
internal/config/           environment loading and validation
internal/scheduler/        ticker, due-monitor dispatch, in-flight set, DueLister
internal/prober/           HTTP worker pool, timing, Result
internal/store/            pgx queries: monitors, due monitors, checks
migrations/                golang-migrate up/down SQL
compose.yaml               app, Postgres, migration runner
```

`main.go` is wiring only: read config, open the pool, construct, start, wait, shut down. Package
names describe what they own — there is no `models`, `utils`, `common` or `helpers`.

## Running locally without Docker

With Postgres reachable and the migrations applied:

```bash
export DB_URL='postgres://uptime_monitor:change_me@localhost:5433/uptime-monitor?sslmode=disable'
export PORT=8000
export PROBER_COUNT=5
go run ./cmd/pulse
```

## Known limitations

Stated plainly, because each one has an answer.

- **An in-flight probe cannot be cancelled.** The worker builds its request with
  `context.Background()` rather than its own context, so shutdown cannot abort a request already
  on the wire — it is bounded only by the 10s client timeout. Passing the worker's context into
  the request is the fix.
- **`GET /duemonitors` is a debug route.** It exposes the scheduler's due query over HTTP. It was
  useful while building the scheduler and is left in deliberately; it would not ship.
- **Pagination is limit/offset, not keyset.** History is read newest-first and rarely paged deep,
  so the cost is deep-page scans and a possible duplicate row across pages while the scheduler is
  writing. Keyset on `(created_at, id)` is the next step if either becomes real.
- **`ListMonitors` has no `LIMIT`.** Correct at ten monitors, wrong at a hundred thousand.
- **The worker pool itself has no test.** The scheduler test proves the *system* keeps flowing;
  it does not assert that the pool never exceeds `PROBER_COUNT`.
- **`config.Load` calls `log.Fatal` inside a library package**, which makes it awkward to test.
  Returning an error and letting `main` decide to exit is the more defensible shape.

## What I'd do next

- **Per-probe HTTP detail** — TTFB, redirect chain and TLS expiry. The columns existed in an
  early schema and were removed rather than left permanently null; they come back with the code
  that fills them.
- **Server-Sent Events** — push check results to a browser dashboard. One-way traffic, so SSE
  over WebSocket: plain HTTP, no library, and the browser reconnects by itself. A slow client
  gets dropped, never blocked — one bad connection must not backpressure the writer and stall the
  pipeline.
- **Alerting** — notify on N consecutive failures rather than on every single one.
- **Readiness endpoint** — `/healthz` is liveness only and deliberately does not touch the
  database. Liveness answers "should you restart me?", and restarting never fixes a dead
  database; a DB check there turns a 30-second Postgres blip into every instance being killed at
  once. Dependency checks belong in a separate `/readyz`.
