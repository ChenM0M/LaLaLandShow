# Database Schema Notes

The initial migration lives in `backend/migrations/0001_initial.up.sql`.

Core correctness constraints:

- `seat_bookings.seat_id` is the primary key, so one seat can have at most one booking.
- `seat_bookings.student_id` is unique, so one student can have at most one seat.
- `seat_bookings` requires exactly one of `student_id` or `custom_label`.
- `students.student_id` must match 110-115 + 1-4 + three digits.
- `students.class_code` must be one of `701-705`, `801-805`, `901-905`, `1001-1005`, `1101-1105`, or `1201-1205` when present.
- `students.seat_no_in_class` must be between `1` and `70` when present.
- Students can exist in a semi-registered state with only `student_id`; `class_code` and `seat_no_in_class` are written together when registration is completed.
- Seat snapshots derive public seat state from PostgreSQL: blocked seats are `locked`, unbooked seats are `free`, the authenticated student's own booking is `mine`, and other student/admin occupied seats are `others`.
- Student booking writes run in one PostgreSQL transaction. The service requires the authenticated actor to have a completed profile, creates missing proxy target students as semi-registered rows, rejects a new batch when the authenticated actor already has an own booking, pre-locks requested seat rows, inserts all requested `seat_bookings` with `ON CONFLICT DO NOTHING`, and rolls back the whole batch unless every requested binding is inserted.
- Proxy booking is creation-only authorization: a student may reserve for a target who does not already have a seat, but once the row exists it is managed by `seat_bookings.student_id` (the target student), not by `booked_by_student_id` (the proxy booker).
- Successful student bookings write `audit_logs.action = 'book'` with `actor_type = 'student'`.
- Student release deletes only the authenticated student's own `seat_bookings` row, writes `audit_logs.action = 'release'`, and leaves any proxy-booked target seats untouched.
- Student rebook releases only the authenticated student's own booking and creates the new request in one PostgreSQL transaction; it must not replace or cancel proxy-booked target seats.
- Student change locks the target seat and current booking in one transaction, moves the booking to the target seat, increments `ticket_version`, and writes `audit_logs.action = 'change_seat'`.
- `system_config.value_json` contains RFC3339 string values for `booking_open_at`, `booking_lock_at`, and `show_start_at`.
- Migration `0002_system_config_defaults` seeds a local/demo window where booking is open, lock is 30 days in the future, and show start is 31 days in the future. Later admin configuration may overwrite these values.
- Booking is allowed only from `booking_open_at` until `booking_lock_at`; changing seats follows the same window; releasing is allowed until `show_start_at`.
- Ticket QR payloads include `student_id`, `seat_id`, `ticket_version`, and `issued_at`; changing seats increments `ticket_version` so old QR tokens can be rejected by check-in verification.
- `staff_accounts` stores bcrypt-hashed checker/admin credentials seeded from environment variables at backend startup.
- Checker login issues staff JWTs. Checker endpoints accept `checker` and `admin` roles.
- QR verification loads the current `seat_bookings` row and rejects signed but stale QR payloads when `seat_id` or `ticket_version` no longer matches.
- Approved entry writes to `checkin_events` with `method` values `qr`, `manual`, or `force_no_booking` and `result = 'approved'`.
- Check-in writes use a per-student PostgreSQL advisory transaction lock before computing `is_first_scan`, so repeated scans for the same student can be distinguished reliably.
- Checker stats derive from `checkin_events` and current `seat_bookings`; `last_minute_rate` is the number of approved scans in the trailing minute.
- Admin dashboard stats derive from `seats`, `seat_bookings`, `students`, `checkin_events`, and recent `audit_logs`; PostgreSQL remains the realtime source of truth.
- Admin login reuses `staff_accounts` but requires role `admin`.
- Admin seat assignment runs in a PostgreSQL transaction, locks the target seat, optionally releases conflicting target/student bookings when `force_release_existing=true`, writes the new `seat_bookings` row, and records `audit_logs.action` as `admin_assign`, `admin_force`, or `admin_release`.
- Admin forced student-seat changes write `students.pending_notice` with an `admin_seat_adjustment` payload; student login and `/api/student/me` expose it, and `/api/student/notice/ack` clears it.
- Admin account bans reuse existing `students.pending_notice` with an `admin_account_ban` payload and restore any previous notice on unban; no database schema change is required.
- Admin permanent locking updates `seats.is_blocked` and `seats.block_reason`; seat snapshots continue to surface blocked seats as `locked`.
- Admin config writes upsert `system_config.value_json` and write `audit_logs.action = 'admin_config'`.
- Admin list/export endpoints read from `students`, `seat_bookings`, `audit_logs`, and `checkin_events`; they do not mutate state.
- Admin JSON restores replace `system_config`, `seats`, `students`, `seat_bookings`, `checkin_events`, and `audit_logs`, reset restored sequences, and then append `audit_logs.action = 'admin_restore'`.
