# Operations Notes

Local B0/B1 flow:

1. Copy `deploy/.env.example` to a local `.env` file if needed.
2. Run `docker compose -f deploy/docker-compose.yml up --build`.
3. Check `http://localhost:8080/healthz`.

The backend runs migrations and default seat seeding at startup when `RUN_MIGRATIONS=true` and `SEED_SEATS=true`.

Staff accounts are upserted at backend startup:

- `ADMIN_USERS` and `ADMIN_PASSWORDS` are comma-separated lists matched by position.
- `CHECKER_USER` and `CHECKER_PASSWORD` seed the default checker account.
- Passwords are stored in `staff_accounts` as bcrypt hashes.

Rate limiting:

- Enabled by default with `RATE_LIMIT_ENABLED=true`.
- `RATE_LIMIT_WINDOW` defaults to `1m`.
- `RATE_LIMIT_GENERIC_MAX` defaults to `300` requests per client/window.
- `RATE_LIMIT_BOOKING_MAX` defaults to `1200` booking attempts per client/window so opening rush traffic has a higher ceiling.
- When Redis is configured, limiter counters are stored under the `lalaland:rate:` key prefix; without Redis the Fiber in-memory limiter is used for local development.

Admin B8 notes:

- Admin API login uses `POST /api/admin/login` and only accepts staff rows with role `admin`.
- Admin seat force assignment should be treated as an irreversible operational action in the UI: call `/api/admin/seats/:id/assign` without `force_release_existing` first, show the `409` conflict details from the current seat view, then retry with `force_release_existing=true` after human confirmation.
- Students affected by an admin seat change receive `students.pending_notice`; the frontend should show it after login or `/api/student/me`, then call `POST /api/student/notice/ack` after the student confirms.
- Public landing/status pages can read `/api/public/config` and `/api/public/window-state` without a token.
- B8 CSV exports are available from `/api/admin/export/students.csv`, `/api/admin/export/bookings.csv`, `/api/admin/export/audit.csv`, and `/api/admin/export/checkin.csv`.

Admin B9 notes:

- Dashboard data is available from `GET /api/admin/dashboard`.
- Counts are computed from PostgreSQL at request time so the dashboard reflects the same source of truth used by booking and check-in flows.
- `last_minute_rate` uses approved check-in events in the trailing minute, matching the checker stats definition.

Admin B10 notes:

- `GET /api/admin/backup/export` creates an application-level JSON backup. It is portable across the current app schema and does not require `pg_dump` inside the backend container.
- `POST /api/admin/backup/plan` validates a backup snapshot and returns a `confirmation_token`.
- `POST /api/admin/backup/restore` requires the exact snapshot plus its confirmation token. Restore replaces business data in one PostgreSQL transaction and records `audit_logs.action = 'admin_restore'`.
- Staff credentials are excluded from JSON backups; keep `ADMIN_USERS`, `ADMIN_PASSWORDS`, `CHECKER_USER`, and `CHECKER_PASSWORD` in deployment secrets.
- Keep the shell `deploy/backup.sh` SQL dump for infrastructure-level disaster recovery; use JSON backup/restore for admin-operated app data moves.
