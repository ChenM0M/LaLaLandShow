CREATE TABLE IF NOT EXISTS students (
  student_id VARCHAR(7) PRIMARY KEY,
  class_code VARCHAR(16),
  seat_no_in_class INT,
  display_name VARCHAR(64),
  pending_notice JSONB,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT students_student_id_format CHECK (student_id ~ '^[0-9]{7}$'),
  CONSTRAINT students_profile_complete CHECK (
    (class_code IS NULL AND seat_no_in_class IS NULL)
    OR (class_code IS NOT NULL AND seat_no_in_class IS NOT NULL)
  )
);

CREATE TABLE IF NOT EXISTS seats (
  seat_id BIGSERIAL PRIMARY KEY,
  zone VARCHAR(16) NOT NULL,
  row_no INT NOT NULL,
  col_no INT NOT NULL,
  is_blocked BOOLEAN NOT NULL DEFAULT false,
  block_reason VARCHAR(64),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT seats_zone_valid CHECK (zone IN ('left', 'middle', 'right')),
  CONSTRAINT seats_position_positive CHECK (row_no > 0 AND col_no > 0),
  UNIQUE (zone, row_no, col_no)
);

CREATE TABLE IF NOT EXISTS seat_bookings (
  seat_id BIGINT PRIMARY KEY REFERENCES seats(seat_id) ON DELETE CASCADE,
  student_id VARCHAR(7) UNIQUE REFERENCES students(student_id),
  custom_label VARCHAR(64),
  booked_by_student_id VARCHAR(7) REFERENCES students(student_id),
  booked_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  ticket_version INT NOT NULL DEFAULT 1,
  CONSTRAINT seat_bookings_owner_xor_label CHECK (
    (student_id IS NOT NULL AND custom_label IS NULL)
    OR (student_id IS NULL AND custom_label IS NOT NULL)
  ),
  CONSTRAINT seat_bookings_ticket_version_positive CHECK (ticket_version > 0)
);

CREATE TABLE IF NOT EXISTS audit_logs (
  id BIGSERIAL PRIMARY KEY,
  actor_type VARCHAR(16) NOT NULL,
  actor_id VARCHAR(64),
  action VARCHAR(32) NOT NULL,
  target_seat_id BIGINT,
  target_student VARCHAR(7),
  before_json JSONB,
  after_json JSONB,
  ip_addr INET,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS checkin_events (
  id BIGSERIAL PRIMARY KEY,
  student_id VARCHAR(7),
  seat_id BIGINT,
  ticket_version INT,
  method VARCHAR(24) NOT NULL,
  result VARCHAR(24) NOT NULL,
  is_first_scan BOOLEAN NOT NULL DEFAULT false,
  scanner_account VARCHAR(64),
  ip_addr INET,
  scanned_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_checkin_events_student_scanned
  ON checkin_events(student_id, scanned_at);

CREATE TABLE IF NOT EXISTS staff_accounts (
  id BIGSERIAL PRIMARY KEY,
  username VARCHAR(64) UNIQUE NOT NULL,
  password_hash TEXT NOT NULL,
  role VARCHAR(16) NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT staff_accounts_role_valid CHECK (role IN ('admin', 'checker'))
);

CREATE TABLE IF NOT EXISTS system_config (
  key VARCHAR(64) PRIMARY KEY,
  value_json JSONB NOT NULL,
  updated_by VARCHAR(64),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
