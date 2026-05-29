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

Admin B8 notes:

- Admin API login uses `POST /api/admin/login` and only accepts staff rows with role `admin`.
- Admin seat force assignment should be treated as an irreversible operational action in the UI: call `/api/admin/seats/:id/assign` without `force_release_existing` first, show the `409` conflict details from the current seat view, then retry with `force_release_existing=true` after human confirmation.
- Students affected by an admin seat change receive `students.pending_notice`; the frontend should show it after login or `/api/student/me`, then call `POST /api/student/notice/ack` after the student confirms.
- Public landing/status pages can read `/api/public/config` and `/api/public/window-state` without a token.
- B8 CSV exports are available from `/api/admin/export/students.csv`, `/api/admin/export/bookings.csv`, `/api/admin/export/audit.csv`, and `/api/admin/export/checkin.csv`.
