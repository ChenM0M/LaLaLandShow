# Load Test Report

Generated: 2026-05-29

## Scope

- B12 adds a reusable Go load-test command at `backend/cmd/loadtest`.
- Modes:
  - `health`: concurrent `GET /healthz` checks.
  - `booking`: synthetic student login plus `POST /api/seats/book` attempts against currently free seats.
- Booking mode can export a B10 snapshot before the run and restore it afterward with `-restore-after=true`.

## Local Smoke Results

Environment:

- Docker Compose backend on `http://127.0.0.1:8080`.
- PostgreSQL 16 and Redis 7 from `deploy/docker-compose.yml`.
- Existing local database contained prior smoke-test rows.

Health smoke:

```json
{
  "requests": 50,
  "concurrency": 10,
  "errors": 0,
  "statuses": { "200": 50 },
  "latency": {
    "max": "9.709416ms",
    "p50": "603.5us",
    "p95": "9.589333ms",
    "p99": "9.680709ms"
  }
}
```

Booking smoke:

```json
{
  "requests": 12,
  "concurrency": 4,
  "errors": 0,
  "statuses": { "201": 12 },
  "latency": {
    "max": "7.729375ms",
    "p50": "3.926875ms",
    "p95": "7.666167ms",
    "p99": "7.666167ms"
  }
}
```

## Full Rush Scenario

Recommended command for a production-like dry run:

```bash
cd backend
go run ./cmd/loadtest \
  -mode booking \
  -requests 1200 \
  -concurrency 120 \
  -restore-after=true
```

Before full rush testing, raise or disable API rate limits for the test window so the result measures booking capacity rather than limiter behavior:

```bash
RATE_LIMIT_GENERIC_MAX=5000 RATE_LIMIT_BOOKING_MAX=5000 docker compose -f deploy/docker-compose.yml up --build -d backend
```

## Notes

- A `201` booking status means a successful booking.
- A `409` booking status is expected under high contention after a seat or student has already been booked.
- A `429` status means the rate limiter, not booking correctness, became the bottleneck.
- Booking mode restores the pre-test snapshot when `-restore-after=true`; the restore itself appends an `admin_restore` audit row.
