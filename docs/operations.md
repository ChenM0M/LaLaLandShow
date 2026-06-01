# Operations Notes

Local B0/B1 flow:

1. Copy `deploy/.env.example` to a local `.env` file if needed.
2. Run `docker compose -f deploy/docker-compose.yml up --build`.
3. Check `http://localhost:8080/healthz`.

The backend runs migrations and default seat seeding at startup when `RUN_MIGRATIONS=true` and `SEED_SEATS=true`.

Production CI/CD:

- GitHub Actions CI is defined in `.github/workflows/ci.yml`.
- GitHub Actions image publishing and SSH deployment are defined in `.github/workflows/deploy.yml`.
- Production Compose runs from `deploy/docker-compose.prod.yml`.
- See `docs/deployment.md` for server setup, required GitHub secrets, release flow, and rollback.

Staff accounts are upserted at backend startup:

- `ADMIN_USERS` and `ADMIN_PASSWORDS` are comma-separated lists matched by position.
- `CHECKER_USER` and `CHECKER_PASSWORD` seed the default checker account.
- Passwords are stored in `staff_accounts` as bcrypt hashes.
- Set `APP_ENV=production` in production. With production mode enabled, the backend refuses to start with placeholder, shared, or too-short `JWT_SECRET`, `HMAC_SECRET`, admin password, or checker password values.

Rate limiting:

- Enabled by default with `RATE_LIMIT_ENABLED=true`.
- `RATE_LIMIT_WINDOW` defaults to `1m`.
- `RATE_LIMIT_GENERIC_MAX` defaults to `300` requests per client/window.
- `RATE_LIMIT_BOOKING_MAX` defaults to `1200` booking attempts per client/window so opening rush traffic has a higher ceiling.
- Limits are counted per client, method, and path so normal page APIs do not burn the same bucket as unrelated endpoints.
- When Redis is configured, limiter counters are stored under the `lalaland:rate:` key prefix; without Redis the Fiber in-memory limiter is used for local development.

Small production host tuning:

- For a 2 vCPU / 2 GB host with less than 1 GB free memory during normal operation, keep `DB_MAX_CONNS` conservative. The default is `12`, with `DB_MIN_CONNS=1`, `DB_MAX_CONN_LIFETIME=1h`, and `DB_MAX_CONN_IDLE_TIME=5m`.
- If booking rush testing shows PostgreSQL CPU is low and memory remains stable, raise `DB_MAX_CONNS` gradually, for example 16 then 20. Avoid jumping to large pool sizes on a 2 GB host.
- The seat stream uses SSE and does not hold database connections per browser. Nginx disables buffering for `/api/seats/stream` so realtime updates are not delayed.
- Keep `RATE_LIMIT_BOOKING_MAX` high enough for the opening rush, but do not disable rate limiting in production unless you are doing a controlled local load test.

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
- The JSON export is read inside one PostgreSQL transaction so seats, bookings, check-ins, audit logs, and config come from one database snapshot instead of a mixed point in time.
- `POST /api/admin/backup/plan` validates a backup snapshot and returns a `confirmation_token`.
- `POST /api/admin/backup/restore` requires the exact snapshot plus its confirmation token. Restore replaces business data in one PostgreSQL transaction and records `audit_logs.action = 'admin_restore'`.
- Staff credentials are excluded from JSON backups; keep `ADMIN_USERS`, `ADMIN_PASSWORDS`, `CHECKER_USER`, and `CHECKER_PASSWORD` in deployment secrets.
- Production Compose includes a lightweight `backup` sidecar that runs `deploy/backup.sh` every `BACKUP_INTERVAL_SECONDS` seconds, keeps files in the `backups` volume, and deletes `lalaland-*.sql.gz` files older than `BACKUP_RETENTION_DAYS`.
- The SQL backup script writes to temporary files and only publishes the final `.sql.gz` after `pg_dump` and `gzip` both succeed. Use this for infrastructure-level disaster recovery; use JSON backup/restore for admin-operated app data moves.
- Quick disaster check: run `docker compose -f deploy/docker-compose.prod.yml exec postgres ls -lh /backups` and verify a recent non-empty `lalaland-*.sql.gz` exists.
- Restore a SQL disaster backup only during a maintenance window. Stop the backend first, then restore with a PostgreSQL client command such as `gunzip -c /backups/lalaland-YYYYMMDD-HHMMSS.sql.gz | psql "$DATABASE_URL"`.

B12 load testing:

- `go run ./cmd/loadtest -mode health -requests 50 -concurrency 10` runs a quick health check load smoke.
- `go run ./cmd/loadtest -mode booking -requests 1200 -concurrency 80 -restore-after=true` simulates a booking rush and restores the pre-test snapshot afterward.
- Booking load mode logs each synthetic student in, completes the acting student's class/seat registration profile, and then attempts a booking. Proxy targets may be unregistered or semi-registered; they must complete registration later before receiving an entry QR code.
- For the 2C2G production server, start with `-requests 600 -concurrency 40`, then try `900/60` and `1200/80` only if memory and PostgreSQL latency remain healthy.
- See `docs/load-test-report.md` for the archived local smoke results and full-rush command.
