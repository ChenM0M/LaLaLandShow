## Completed

- hard_observed: Plan capability was claimed successfully and `.vibehub/tasks/T-20260529133755-69e7bd66/runs/R-20260529133755-405932d7/context-packs/plan.md` was generated.
- agent_reported: Created an implementation and validation plan prioritizing backend B0-B7 as the first demonstrable milestone.
- agent_reported: Plan incorporates research finding that Fiber v3 requires Go 1.25+, so implementation should use Go 1.26.x unless the user later requires an older toolchain.

## Implementation Plan

### steps

1. Project foundation / B0
   - Create `backend/` Go module using Go 1.26.x and Fiber v3.
   - Add config loading, structured logging, `/healthz`, Dockerfile, and local Compose services for PostgreSQL and Redis.
   - Add `deploy/.env.example` with JWT/HMAC/admin/checker/database/redis settings.

2. Database and seed / B1
   - Add migrations for `students`, `seats`, `seat_bookings`, `audit_logs`, `checkin_events`, `staff_accounts`, and `system_config`.
   - Add seat seed command/function for 385 seats and default 24 blocked seats.
   - Keep booking uniqueness in PostgreSQL: `seat_bookings.seat_id` primary key and `seat_bookings.student_id` unique.

3. Student identity / B2
   - Implement `POST /api/student/login`, `POST /api/student/register`, and `GET /api/student/me`.
   - Validate 7-digit student IDs server-side.
   - Support first login, repeat login, and semi-registered proxy-booked students.
   - Implement JWT middleware and 7-day rolling token behavior.

4. Seat snapshot and SSE / B3
   - Implement `GET /api/seats/snapshot` using PostgreSQL as source of truth.
   - Implement `GET /api/seats/stream` with snapshot-first client contract and Redis Pub/Sub as best-effort fanout.
   - Ensure SSE does not hold database connections per client.

5. Atomic booking / B4
   - Implement `POST /api/seats/book` for 1-3 requested seats.
   - Enforce time window, self-seat requirement, no duplicate students/seats in batch, and all-or-nothing transaction.
   - Use DB constraints plus transaction rollback when affected rows do not match requested rows.
   - Write audit logs and publish SSE updates after commit.

6. Release and change / B5
   - Implement `POST /api/seats/release` and `POST /api/seats/change`.
   - Enforce time-window rules: before lock can change/release, after lock can release only, after show start read-only.
   - Increment `ticket_version` on seat changes where applicable and audit every write.

7. Ticket and QR / B6
   - Implement `GET /api/ticket/me` and later `GET /api/ticket/me/image`.
   - Sign canonical payload containing `student_id`, `seat_id`, `ticket_version`, and `issued_at` with HMAC-SHA256.
   - Verify with constant-time MAC comparison.

8. Checker core / B7
   - Implement checker/admin login seed from environment.
   - Implement `POST /api/checker/verify-qr`, `POST /api/checker/lookup`, `POST /api/checker/checkin`, `GET /api/checker/stats`, and `GET /api/checker/export-roster.csv`.
   - Record first/repeat scans, invalid signatures, manual approvals, and no-booking force approvals in `checkin_events`.

9. Defer after B0-B7
   - B8-B12: full admin CRUD, dashboard, backup/restore, rate limiting, load testing.
   - F0-F9: frontend landing/login/seat map/ticket/checker/admin after backend contracts stabilize.

### validation_plan

- B0: `go test ./...`; `docker compose config`; start Compose; `curl /healthz`.
- B1: run migrations; assert `SELECT count(*) FROM seats = 385`; assert default blocked count is 24; test idempotent seed.
- B2: unit/integration tests for invalid ID, first login, repeat login, register completion, semi-registered completion, JWT middleware.
- B3: integration test snapshot states; SSE smoke test with two clients and a published update; reconnect should refresh snapshot.
- B4: concurrent integration test where multiple requests race for the same seat and only one succeeds; batch conflict must roll back all seats; unique student constraint must prevent double booking.
- B5: time-window tests for release/change permissions; audit log assertions; ticket version behavior assertions.
- B6: QR round-trip test; tampered payload rejected; wrong version rejected; use constant-time compare helper.
- B7: checker lookup/checkin tests for QR, manual, repeat scan, invalid signature, no-booking force approval, admin-as-checker access, and CSV export.
- Cross-cutting: `go vet ./...` if available, migration rollback smoke where supported, and a small k6/vegeta smoke later before frontend integration.

### affected_files

- `backend/go.mod`
- `backend/go.sum`
- `backend/cmd/server/main.go`
- `backend/internal/config/`
- `backend/internal/api/`
- `backend/internal/auth/`
- `backend/internal/domain/`
- `backend/internal/repo/`
- `backend/internal/service/`
- `backend/internal/seat/`
- `backend/internal/ticket/`
- `backend/internal/checkin/`
- `backend/internal/admin/`
- `backend/migrations/`
- `backend/Dockerfile`
- `deploy/docker-compose.yml`
- `deploy/.env.example`
- `deploy/backup.sh`
- `docs/api.md`
- `docs/db-schema.md`
- `docs/operations.md`

### risks

- Go baseline mismatch: plan resolves by using Go 1.26.x/Fiber v3; user can override before implementation if necessary.
- Seat geometry is still unspecified; seed implementation should isolate geometry data so it can be corrected without rewriting booking logic.
- Redis Pub/Sub is not durable; client must snapshot on load/reconnect.
- No Git repository means diff/recovery evidence is weaker; initialize Git before code implementation if possible.
- OSS upload may not be needed for backend B0-B7 and should not block first milestone.

### references

- `.vibehub/notes/01-requirements.md`
- `.vibehub/notes/02-implementation.md`
- `.vibehub/research/current/research-pack.md`
- `.vibehub/tasks/T-20260529133755-69e7bd66/runs/R-20260529133755-405932d7/context-packs/plan.md`

## Not Yet Done

- agent_reported: No application code has been created yet.
- agent_reported: Exact 385-seat geometry needs confirmation or a configurable placeholder.
- agent_reported: Frontend framework choice remains open and should wait until backend contracts are stable or MiMo output is available.

## Key Decisions Made

- inferred: Use Go 1.26.x + Fiber v3 for implementation unless user explicitly overrides.
- inferred: Use pgx v5 direct PostgreSQL access.
- inferred: Treat B0-B7 as the first implementation milestone.
- inferred: Keep Redis out of booking correctness and use PostgreSQL as the source of truth.

## Files Changed

- `.vibehub/tasks/T-20260529133755-69e7bd66/runs/R-20260529133755-405932d7/phases/plan.output.md`
- `.vibehub/tasks/T-20260529133755-69e7bd66/runs/R-20260529133755-405932d7/outputs/output.md`
- `.vibehub/notes/status.md`

## Files Reportedly Read

- `.vibehub/tasks/T-20260529133755-69e7bd66/runs/R-20260529133755-405932d7/context-packs/plan.md`
- `.vibehub/tasks/T-20260529133755-69e7bd66/runs/R-20260529133755-405932d7/context-packs/plan.manifest.yaml`
- `.vibehub/research/current/research-pack.md`
- `.vibehub/notes/02-implementation.md`

## Commands Run

- `vibehub claim /Users/chenm0m/LocalRepo/LaLaLandShow plan`
- `sed -n '1,360p' .vibehub/tasks/T-20260529133755-69e7bd66/runs/R-20260529133755-405932d7/context-packs/plan.md`
- `sed -n '1,220p' .vibehub/tasks/T-20260529133755-69e7bd66/runs/R-20260529133755-405932d7/context-packs/plan.manifest.yaml`
- `sed -n '1,260p' .vibehub/research/current/research-pack.md`
- `sed -n '1,260p' .vibehub/notes/02-implementation.md`

## Tests Run

- agent_reported: No application tests were run because this session is still planning and no application code exists yet.

## Context Still Needed

- Exact seat geometry for 385 seats.
- User confirmation only if Go 1.26.x/Fiber v3 is not acceptable.
- Frontend framework decision when frontend work begins.

## Warnings

- hard_observed: The project root is still not a Git repository.

## Next Session Should

- Run `vibehub sync /Users/chenm0m/LocalRepo/LaLaLandShow` to validate plan output.
- If plan validates, claim `implement` and start B0/B1 backend foundation.
