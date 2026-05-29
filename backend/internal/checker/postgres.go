package checker

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"

	"lalalandshow/backend/internal/ticket"
)

var ErrStaffNotFound = errors.New("staff account not found")

type AccountSeed struct {
	Username string
	Password string
	Role     string
}

type PostgresStore struct {
	db *pgxpool.Pool
}

func NewPostgresStore(db *pgxpool.Pool) *PostgresStore {
	return &PostgresStore{db: db}
}

func SeedAccounts(ctx context.Context, db *pgxpool.Pool, accounts []AccountSeed) error {
	for _, account := range accounts {
		if account.Username == "" || account.Password == "" {
			continue
		}
		if account.Role != "admin" && account.Role != "checker" {
			return fmt.Errorf("seed staff account %s: invalid role %q", account.Username, account.Role)
		}
		hash, err := bcrypt.GenerateFromPassword([]byte(account.Password), bcrypt.DefaultCost)
		if err != nil {
			return fmt.Errorf("hash staff password for %s: %w", account.Username, err)
		}
		if _, err := db.Exec(ctx, `
INSERT INTO staff_accounts (username, password_hash, role)
VALUES ($1, $2, $3)
ON CONFLICT (username) DO UPDATE
SET password_hash = EXCLUDED.password_hash,
    role = EXCLUDED.role`,
			account.Username,
			string(hash),
			account.Role,
		); err != nil {
			return fmt.Errorf("upsert staff account %s: %w", account.Username, err)
		}
	}
	return nil
}

func (s *PostgresStore) FindStaffAccount(ctx context.Context, username string) (StaffAccount, error) {
	row := s.db.QueryRow(ctx, `
SELECT username, password_hash, role
FROM staff_accounts
WHERE username = $1`, username)
	var account StaffAccount
	if err := row.Scan(&account.Username, &account.PasswordHash, &account.Role); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return StaffAccount{}, ErrInvalidCredentials
		}
		return StaffAccount{}, fmt.Errorf("find staff account: %w", err)
	}
	return account, nil
}

func (s *PostgresStore) GetBooking(ctx context.Context, studentID string) (Booking, error) {
	row := s.db.QueryRow(ctx, `
WITH checkins AS (
  SELECT student_id, max(scanned_at) AS last_checkin_at, count(*) AS checkin_count
  FROM checkin_events
  WHERE student_id = $1
    AND result = 'approved'
  GROUP BY student_id
)
SELECT st.student_id,
       st.class_code,
       st.seat_no_in_class,
       st.display_name,
       b.seat_id,
       se.zone,
       se.row_no,
       se.col_no,
       b.ticket_version,
       b.booked_at,
       c.last_checkin_at,
       COALESCE(c.checkin_count, 0)
FROM seat_bookings b
JOIN students st ON st.student_id = b.student_id
JOIN seats se ON se.seat_id = b.seat_id
LEFT JOIN checkins c ON c.student_id = st.student_id
WHERE b.student_id = $1`, studentID)

	booking, err := scanBooking(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Booking{}, ticket.ErrTicketNotFound
		}
		return Booking{}, fmt.Errorf("get checker booking: %w", err)
	}
	return booking, nil
}

func (s *PostgresStore) InsertCheckIn(ctx context.Context, input CheckInInput) (CheckInEvent, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return CheckInEvent{}, fmt.Errorf("begin check-in transaction: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, input.StudentID); err != nil {
		return CheckInEvent{}, fmt.Errorf("lock check-in student: %w", err)
	}

	var seen bool
	if err := tx.QueryRow(ctx, `
SELECT EXISTS (
  SELECT 1
  FROM checkin_events
  WHERE student_id = $1
    AND result = 'approved'
)`, input.StudentID).Scan(&seen); err != nil {
		return CheckInEvent{}, fmt.Errorf("read prior check-ins: %w", err)
	}

	row := tx.QueryRow(ctx, `
INSERT INTO checkin_events (
  student_id,
  seat_id,
  ticket_version,
  method,
  result,
  is_first_scan,
  scanner_account,
  ip_addr
)
VALUES ($1, $2, $3, $4, $5, $6, $7, NULLIF($8, '')::inet)
RETURNING id,
          student_id,
          seat_id,
          ticket_version,
          method,
          result,
          is_first_scan,
          scanner_account,
          scanned_at`,
		input.StudentID,
		input.SeatID,
		input.TicketVersion,
		input.Method,
		input.Result,
		!seen,
		input.Scanner,
		input.IPAddr,
	)
	event, err := scanCheckInEvent(row)
	if err != nil {
		return CheckInEvent{}, fmt.Errorf("insert check-in event: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return CheckInEvent{}, fmt.Errorf("commit check-in transaction: %w", err)
	}
	return event, nil
}

func (s *PostgresStore) Stats(ctx context.Context) (Stats, error) {
	row := s.db.QueryRow(ctx, `
SELECT
  (SELECT count(DISTINCT student_id) FROM checkin_events WHERE result = 'approved') AS checked_in,
  (SELECT count(*) FROM seat_bookings WHERE student_id IS NOT NULL) AS total,
  (SELECT count(*) FROM checkin_events WHERE result = 'approved' AND scanned_at >= now() - interval '1 minute') AS last_minute_rate`)
	var stats Stats
	if err := row.Scan(&stats.CheckedIn, &stats.Total, &stats.LastMinuteRate); err != nil {
		return Stats{}, fmt.Errorf("read checker stats: %w", err)
	}
	return stats, nil
}

func (s *PostgresStore) Roster(ctx context.Context) ([]RosterRow, error) {
	rows, err := s.db.Query(ctx, `
WITH approved AS (
  SELECT student_id,
         min(scanned_at) AS first_scanned_at,
         max(scanned_at) AS last_scanned_at,
         count(*) AS scan_count
  FROM checkin_events
  WHERE result = 'approved'
  GROUP BY student_id
)
SELECT st.student_id,
       COALESCE(st.class_code, ''),
       st.seat_no_in_class,
       COALESCE(st.display_name, ''),
       b.seat_id,
       se.zone,
       se.row_no,
       se.col_no,
       b.ticket_version,
       a.first_scanned_at,
       a.last_scanned_at,
       COALESCE(a.scan_count, 0)
FROM seat_bookings b
JOIN students st ON st.student_id = b.student_id
JOIN seats se ON se.seat_id = b.seat_id
LEFT JOIN approved a ON a.student_id = st.student_id
ORDER BY se.zone, se.row_no, se.col_no, st.student_id`)
	if err != nil {
		return nil, fmt.Errorf("query checker roster: %w", err)
	}
	defer rows.Close()

	roster := []RosterRow{}
	for rows.Next() {
		var (
			row            RosterRow
			seatNoInClass  sql.NullInt64
			firstScannedAt sql.NullTime
			lastScannedAt  sql.NullTime
		)
		if err := rows.Scan(
			&row.StudentID,
			&row.ClassCode,
			&seatNoInClass,
			&row.DisplayName,
			&row.SeatID,
			&row.Zone,
			&row.RowNo,
			&row.ColNo,
			&row.TicketVersion,
			&firstScannedAt,
			&lastScannedAt,
			&row.ScanCount,
		); err != nil {
			return nil, fmt.Errorf("scan checker roster: %w", err)
		}
		if seatNoInClass.Valid {
			value := int(seatNoInClass.Int64)
			row.SeatNoInClass = &value
		}
		if firstScannedAt.Valid {
			row.FirstScannedAt = &firstScannedAt.Time
		}
		if lastScannedAt.Valid {
			row.LastScannedAt = &lastScannedAt.Time
		}
		row.CheckedIn = row.ScanCount > 0
		roster = append(roster, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate checker roster: %w", err)
	}
	return roster, nil
}

type bookingScanner interface {
	Scan(dest ...any) error
}

func scanBooking(row bookingScanner) (Booking, error) {
	var (
		booking       Booking
		classCode     sql.NullString
		seatNoInClass sql.NullInt64
		displayName   sql.NullString
		lastCheckInAt sql.NullTime
	)
	if err := row.Scan(
		&booking.StudentID,
		&classCode,
		&seatNoInClass,
		&displayName,
		&booking.SeatID,
		&booking.Zone,
		&booking.RowNo,
		&booking.ColNo,
		&booking.TicketVersion,
		&booking.BookedAt,
		&lastCheckInAt,
		&booking.CheckInCount,
	); err != nil {
		return Booking{}, err
	}
	if classCode.Valid {
		booking.ClassCode = &classCode.String
	}
	if seatNoInClass.Valid {
		value := int(seatNoInClass.Int64)
		booking.SeatNoInClass = &value
	}
	if displayName.Valid {
		booking.DisplayName = &displayName.String
	}
	if lastCheckInAt.Valid {
		booking.LastCheckInAt = &lastCheckInAt.Time
	}
	booking.CheckedInBefore = booking.CheckInCount > 0
	return booking, nil
}

func scanCheckInEvent(row bookingScanner) (CheckInEvent, error) {
	var (
		event         CheckInEvent
		seatID        sql.NullInt64
		ticketVersion sql.NullInt64
		scanner       sql.NullString
	)
	if err := row.Scan(
		&event.ID,
		&event.StudentID,
		&seatID,
		&ticketVersion,
		&event.Method,
		&event.Result,
		&event.IsFirstScan,
		&scanner,
		&event.ScannedAt,
	); err != nil {
		return CheckInEvent{}, err
	}
	if seatID.Valid {
		value := seatID.Int64
		event.SeatID = &value
	}
	if ticketVersion.Valid {
		value := int(ticketVersion.Int64)
		event.TicketVersion = &value
	}
	if scanner.Valid {
		event.Scanner = scanner.String
	}
	return event, nil
}
