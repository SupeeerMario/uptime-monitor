# Uptime Monitor API

A concurrent HTTP uptime monitor written in Go, Gin, pgx and Postgres. Register a URL with a
check interval, and Pulse probes it on schedule, records every result — status code, latency,
transport error — and keeps the history in Postgres behind a small JSON API.

## Features

- **Register monitors over HTTP** — URL, check interval and expected status, validated on the way in.
- **Scheduler on a ticker** — every 5s it asks Postgres which monitors are due and dispatches them.
- **Concurrent probing** — worker goroutines fed by a buffered jobs channel, each probe timed with a shared `http.Client`.
- **Every probe is a row** — successes, non-2xx responses and connection failures all land in `checks`.
- **Due-monitor selection in SQL** — `last_checked_at + interval <= now()`, so the database decides what is due.
- **In-flight tracking** — a monitor already being probed is skipped on later ticks instead of queued twice.
- **Graceful shutdown** — on SIGINT/SIGTERM the scheduler stops, in-flight probes finish, their results are written, and only then does the pool close.
- **One-command environment** — Docker Compose brings up Postgres, runs migrations, and starts the app with live reload.

## Architecture

```mermaid
flowchart TD
    client([Client]) -->|"HTTP JSON"| gin["Gin router<br/>internal/api"]
    gin --> store["Store<br/>internal/store"]

    sched["Scheduler<br/>5s ticker"] -->|"ListDueMonitors"| store
    sched -->|"DueMonitor"| jobs[["jobs channel"]]
    jobs --> worker["HTTP worker<br/>internal/prober"]
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

1. The **scheduler** ticks every 5 seconds and queries `monitors` for rows whose
   `last_checked_at + interval_seconds` has passed (or that were never checked).
2. Each due monitor is sent on the **jobs channel**, unless it is already in flight.
3. An **HTTP worker** takes the job, issues a `GET` with a 10s client timeout, and times the
   request, producing a `Result` with a nullable status code and a nullable error string.
4. The result goes on the **results channel**.
5. The **writer goroutine** calls `SaveCheck`, which inserts the `checks` row and stamps
   `monitors.last_checked_at` in a single statement, then signals the monitor id back on
   **schedDone** so the scheduler clears it from the in-flight set.

### Shutdown

A signal cancels the scheduler and the prober contexts. The scheduler closes the jobs channel on
its way out; the worker stops taking new work while the request already in flight completes on a
background context bounded by the client timeout. A `WaitGroup` tracks the workers, the results
channel closes once they are gone, and `main` waits on a done channel from the writer before
closing the pgx pool, so no probe result is dropped on the way down.

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
```

Create a `.env` in the project root:

```env
DB_NAME=pulse
DB_USERNAME=pulse
DB_PASSWORD=pulse
DB_PORT=5432
DB_URL=postgres://pulse:pulse@db:5432/pulse?sslmode=disable
PORT=8000
```

Then:

```bash
docker compose up --build
```

Compose waits for Postgres to pass its healthcheck, runs the migrations to completion, and then
starts the API on `http://localhost:8000`. Postgres is published on host port `5433`.

```bash
curl localhost:8000/healthz
# {"message":"Uptime monitor is running","status":"healthy"}
```

## Configuration

| Key | Example | Notes |
| --- | --- | --- |
| `DB_NAME` | `pulse` | Database created by the Postgres container |
| `DB_USERNAME` | `pulse` | Postgres role |
| `DB_PASSWORD` | `pulse` | Postgres password |
| `DB_PORT` | `5432` | Port used to build `DB_URL` inside the Compose network |
| `DB_URL` | `postgres://pulse:pulse@db:5432/pulse?sslmode=disable` | Read by the app at startup; required |
| `PORT` | `8000` | HTTP listen port; required, must be numeric |

## API

| Method | Path | Purpose |
| --- | --- | --- |
| `GET` | `/healthz` | Liveness check |
| `POST` | `/monitors` | Register a monitor |
| `GET` | `/monitors` | List all monitors |
| `DELETE` | `/monitors/:id` | Delete a monitor and its checks |
| `GET` | `/duemonitors` | List monitors currently due for a probe |

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

```json
{
  "id": 1,
  "url": "https://example.com",
  "interval_seconds": 30,
  "expected_status": 200
}
```

`url` must be a valid URL, `interval_seconds` greater than zero, and `expected_status` between
100 and 599; anything else returns `422` with the validation error.

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
    "created_at": "2026-09-20T11:59:30Z"
  }
]
```

### Delete a monitor

```bash
curl -i -X DELETE localhost:8000/monitors/1
# HTTP/1.1 204 No Content
```

The `checks` foreign key cascades, so a monitor's history goes with it. An unknown id returns
`404`.

### Watch the results land

```bash
docker compose exec db psql -U pulse -d pulse \
  -c 'SELECT monitor_id, status_code, total_latency_ms, error, created_at
      FROM checks ORDER BY created_at DESC LIMIT 5;'
```

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
    ttfb_ms BIGINT NULL,
    total_latency_ms BIGINT NULL,
    error TEXT NULL,
    redirect_chain TEXT[] NULL,
    tls_expires_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT fk_checks_monitor
        FOREIGN KEY (monitor_id)
        REFERENCES monitors(id)
        ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS checks_monitor ON checks(monitor_id, created_at DESC);
```

`status_code` and `error` are both nullable: a refused connection records an error with no
status, a completed request records a status with no error.

## Project layout

```
cmd/pulse/main.go          wiring: pool, router, scheduler, workers, writer, shutdown
internal/api/              Gin handlers and request binding
internal/config/           environment loading and validation
internal/scheduler/        5s ticker, due-monitor dispatch, in-flight set
internal/prober/           HTTP worker, timing, Result
internal/store/            pgx queries: monitors, due monitors, checks
migrations/                golang-migrate up/down SQL
docker-compose.yml         app, Postgres, migration runner
```

## Running locally without Docker

With a Postgres instance reachable and the migrations applied:

```bash
export DB_URL='postgres://pulse:pulse@localhost:5433/pulse?sslmode=disable'
export PORT=8000
go run ./cmd/pulse
```
