# Research Pack: LaLaLandShow Platform

Generated: 2026-05-29
Task: `T-20260529133755-69e7bd66`
Run: `R-20260529133755-405932d7`
Capability: research

## Research Questions

- Can the proposed Go + Fiber + PostgreSQL + Redis + SSE stack support the booking and check-in requirements?
- What updates are needed because the notes were written before the current 2026 toolchain state?
- Which implementation risks must be handled before planning backend B0-B7?

## Hypothesis

The proposed architecture is viable for the project scale: approximately 1200 students competing for 385 seats, one show, one active venue, and simple one-way realtime updates. The main correction is that Fiber v3 now requires Go 1.25+, so the backend baseline should be updated from "Go 1.22+" to "Go 1.26.x preferred, Go 1.25 minimum" if Fiber v3 remains the chosen framework.

## Sources

- hard_observed: Go release history says Go 1.26.3 was released on 2026-05-07 and Go supports each major release until two newer major releases exist. Source: https://go.dev/doc/devel/release
- hard_observed: Fiber official docs cover Fiber v3 and state that Go 1.25 or higher is required. Source: https://docs.gofiber.io/
- hard_observed: Fiber v3 migration notes call out meaningful API changes and again state the Go 1.25+ requirement. Source: https://docs.gofiber.io/blog/whats-new-in-fiber-v3
- hard_observed: PostgreSQL current docs describe `INSERT ... ON CONFLICT` as the mechanism for handling unique/exclusion conflicts on insert. Source: https://www.postgresql.org/docs/current/static/sql-insert.html
- hard_observed: PostgreSQL docs state unique constraints automatically create unique B-tree indexes. Source: https://www.postgresql.org/docs/current/ddl-constraints.html
- hard_observed: PostgreSQL documentation lists version 16 as supported, though current docs are version 18. Source: https://www.postgresql.org/docs/
- hard_observed: Redis Pub/Sub docs state Pub/Sub has at-most-once delivery semantics. Source: https://redis.io/docs/latest/develop/pubsub/
- hard_observed: Redis rate limiter docs describe Redis as suitable for centralized quotas across service instances. Source: https://redis.io/docs/latest/develop/use-cases/rate-limiter/
- hard_observed: Docker Compose docs describe Compose as a tool for defining/running multi-container apps and managing services, networks, and volumes. Source: https://docs.docker.com/compose/
- hard_observed: Docker Compose file reference says the Compose Specification is the current recommended format. Source: https://docs.docker.com/reference/compose-file/
- hard_observed: WHATWG HTML describes SSE/EventSource using `text/event-stream`. Source: https://html.spec.whatwg.org/dev/server-sent-events.html
- hard_observed: Go `crypto/hmac` package docs state HMAC signs a message with a key and recommend `hmac.Equal` to avoid timing side channels. Source: https://pkg.go.dev/crypto/hmac
- hard_observed: pgx README describes pgx as a pure Go PostgreSQL driver/toolkit, with v5 as latest stable major version and support aligned to supported Go/PostgreSQL releases. Source: https://github.com/jackc/pgx/

## Findings

### Backend Toolchain

- agent_reported: Fiber v3 remains a reasonable fit for an Express-like Go API, but the plan must update the Go baseline. The current notes say Go 1.22+, while Fiber v3 now requires Go 1.25+.
- agent_reported: Recommended backend baseline is Go 1.26.x, Fiber v3, pgx v5, PostgreSQL 16 or newer, Redis 7+, and Docker Compose v2/spec format.
- inferred: If the deployment machine cannot support Go 1.25+, switch the framework choice during plan to Echo v4 or Fiber v2 rather than forcing Fiber v3.

### Seat Booking Correctness

- agent_reported: PostgreSQL is the right correctness boundary for "each seat at most one booking" and "each student at most one seat" using primary key/unique constraints.
- agent_reported: The booking endpoint should use one database transaction and fail the full batch if any requested seat/student conflicts. Do not rely on Redis for correctness.
- inferred: A robust B4 design should insert all desired bindings in a transaction, verify affected row count equals requested count, and roll back otherwise. The schema must ensure custom-label admin occupancy remains mutually exclusive with student booking.

### Realtime Seat Map

- agent_reported: SSE is appropriate because the app needs server-to-client updates only; no bidirectional realtime channel is required.
- hard_observed: Redis Pub/Sub is at-most-once, so it is acceptable only as a freshness accelerator, not as a durable event log.
- agent_reported: Every client should load `/api/seats/snapshot` first, then subscribe to `/api/seats/stream`. On reconnect or missed events, the client should refresh the snapshot.

### Rate Limiting

- agent_reported: Redis-backed rate limiting matches the notes because it can enforce quotas consistently if the service is later scaled beyond one process.
- inferred: The booking endpoint should have a separate, more permissive burst policy than generic API routes so normal opening-time rush traffic is not throttled as abuse.

### Ticket QR Security

- agent_reported: HMAC-SHA256 is an appropriate QR signature mechanism for this use case if the server keeps the signing key secret and compares MACs with constant-time comparison.
- agent_reported: The notes' requirement to include `student_id`, `seat_id`, `ticket_version`, and `issued_at` is important and should be kept. `ticket_version` is the simple invalidation primitive for changed seats.
- inferred: The serialized signed payload should be canonical JSON or a compact delimited format with explicit versioning to avoid ambiguous signature input.

### Deployment

- agent_reported: Docker Compose is appropriate for one app container plus PostgreSQL, Redis, and backup/cron responsibilities on a small server.
- inferred: For a 2C2G host, the plan should keep the backend as a single app process initially, tune PostgreSQL connection pool limits, and avoid running heavyweight observability stacks in v1 unless needed.

## Risks

- risk: The implementation note's "Go 1.22+" conflicts with current Fiber v3 requirements. Mitigation: update plan to Go 1.26.x preferred / 1.25 minimum, or choose Echo v4/Fiber v2 if older Go is mandatory.
- risk: Redis Pub/Sub can drop events for disconnected clients. Mitigation: snapshot-first client model, reconnect refresh, and database as source of truth.
- risk: Booking correctness can fail if logic checks availability before inserting without constraints. Mitigation: enforce `seat_id` primary key and `student_id` unique constraints, perform batch booking in one transaction, and test concurrent contention.
- risk: Admin custom occupancy and student booking can conflict if schema constraints are loose. Mitigation: use `CHECK` constraints and service-level validation for `student_id` xor `custom_label`.
- risk: QR payloads can remain valid after seat changes if versioning is not checked. Mitigation: include and compare `ticket_version` against current DB value on every scan.
- risk: 2C2G capacity may be constrained by PostgreSQL connections and long-lived SSE clients. Mitigation: cap DB pool, do not allocate DB connections per SSE client, and benchmark B12 with realistic concurrency.
- risk: Browser camera scanning varies by mobile browser and HTTPS context. Mitigation: require HTTPS in deployment docs and include manual student-id lookup as a first-class fallback.
- risk: Git evidence is unavailable because the project root is not a Git repository. Mitigation: initialize Git before code implementation or accept reduced VibeHub diff observability.

## Open Questions

- open_question: Should the backend plan officially update Go baseline to Go 1.26.x, or must it preserve the original Go 1.22+ constraint?
- open_question: Should PostgreSQL stay pinned to 16 for production parity, or can the Docker Compose default move to the current supported major version during implementation?
- open_question: Which frontend framework will MiMo produce: Next.js, Vue/Vite, Nuxt, or SvelteKit?
- open_question: What exact 385-seat geometry should be seeded: row/column counts by left/middle/right zone and any aisle gaps?
- open_question: Will production deployment use one domain with path routing or split frontend/backend origins?
- open_question: Will media upload to OSS be implemented in v1, or will admin content initially store external URLs only?

## Recommended Decisions For Plan

- Use Go 1.26.x and Fiber v3 unless the user explicitly requires older Go.
- Use pgx v5 directly rather than `database/sql`, because the app is PostgreSQL-specific and may benefit from PostgreSQL-specific behavior.
- Keep PostgreSQL as the sole booking source of truth; Redis is for rate limiting, session/cache support, and SSE fanout only.
- Implement B0-B7 first as the minimum demonstrable backend chain.
- Add concurrency tests for B4 before building the seat-map UI against it.
- Use snapshot-first SSE semantics and document at-most-once Redis Pub/Sub behavior in code comments/tests.

