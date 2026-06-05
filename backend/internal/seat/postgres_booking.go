package seat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

const uniqueViolationCode = "23505"

func (s *PostgresSnapshotStore) IsStudentBanned(ctx context.Context, studentID string) (bool, error) {
	var banned bool
	if err := s.db.QueryRow(ctx, `
SELECT COALESCE(pending_notice @> '{"type":"admin_account_ban"}'::jsonb, false)
FROM students
WHERE student_id = $1`, studentID).Scan(&banned); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, fmt.Errorf("check student ban: %w", err)
	}
	return banned, nil
}

func (s *PostgresSnapshotStore) BookSeats(ctx context.Context, actorStudentID string, entries []BookEntry) ([]BookedSeat, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin booking transaction: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	if err := ensureBookingAllowed(ctx, tx, time.Now()); err != nil {
		return nil, err
	}

	seatIDs := make([]int64, 0, len(entries))
	studentIDs := make([]string, 0, len(entries))
	ordinals := make([]int32, 0, len(entries))
	for i, entry := range entries {
		seatIDs = append(seatIDs, entry.SeatID)
		studentIDs = append(studentIDs, entry.StudentID)
		ordinals = append(ordinals, int32(i))
	}

	if err := ensureBookableStudentRows(ctx, tx, actorStudentID, studentIDs); err != nil {
		return nil, err
	}

	if err := ensureNoExistingOwnBooking(ctx, tx, actorStudentID); err != nil {
		return nil, err
	}

	if err := lockRequestedSeats(ctx, tx, seatIDs, len(entries)); err != nil {
		return nil, err
	}

	bookings, err := insertSeatBookings(ctx, tx, actorStudentID, seatIDs, studentIDs, ordinals, len(entries))
	if err != nil {
		return nil, err
	}
	for _, booking := range bookings {
		if err := writeBookingAudit(ctx, tx, actorStudentID, booking); err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit booking transaction: %w", err)
	}
	return bookings, nil
}

func (s *PostgresSnapshotStore) ReleaseSeats(ctx context.Context, actorStudentID string) ([]BookedSeat, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin release transaction: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	if err := ensureReleaseAllowed(ctx, tx, time.Now()); err != nil {
		return nil, err
	}

	released, err := deleteOwnBooking(ctx, tx, actorStudentID)
	if err != nil {
		return nil, err
	}
	for _, booking := range released {
		if err := writeReleaseAudit(ctx, tx, actorStudentID, booking); err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit release transaction: %w", err)
	}
	return released, nil
}

func (s *PostgresSnapshotStore) RebookSeats(ctx context.Context, actorStudentID string, entries []BookEntry) (RebookResult, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return RebookResult{}, fmt.Errorf("begin rebook transaction: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	if err := ensureBookingAllowed(ctx, tx, time.Now()); err != nil {
		return RebookResult{}, err
	}

	seatIDs := make([]int64, 0, len(entries))
	studentIDs := make([]string, 0, len(entries))
	ordinals := make([]int32, 0, len(entries))
	for i, entry := range entries {
		seatIDs = append(seatIDs, entry.SeatID)
		studentIDs = append(studentIDs, entry.StudentID)
		ordinals = append(ordinals, int32(i))
	}

	if err := ensureBookableStudentRows(ctx, tx, actorStudentID, studentIDs); err != nil {
		return RebookResult{}, err
	}

	released, err := deleteOwnBooking(ctx, tx, actorStudentID)
	if err != nil {
		return RebookResult{}, err
	}
	for _, booking := range released {
		if err := writeReleaseAudit(ctx, tx, actorStudentID, booking); err != nil {
			return RebookResult{}, err
		}
	}

	if err := lockRequestedSeats(ctx, tx, seatIDs, len(entries)); err != nil {
		return RebookResult{}, err
	}

	bookings, err := insertSeatBookings(ctx, tx, actorStudentID, seatIDs, studentIDs, ordinals, len(entries))
	if err != nil {
		return RebookResult{}, err
	}
	for _, booking := range bookings {
		if err := writeBookingAudit(ctx, tx, actorStudentID, booking); err != nil {
			return RebookResult{}, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return RebookResult{}, fmt.Errorf("commit rebook transaction: %w", err)
	}
	return RebookResult{ReleasedBookings: released, Bookings: bookings}, nil
}

func (s *PostgresSnapshotStore) ChangeSeat(ctx context.Context, actorStudentID string, newSeatID int64) (SeatChange, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return SeatChange{}, fmt.Errorf("begin change transaction: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	if err := ensureChangeAllowed(ctx, tx, time.Now()); err != nil {
		return SeatChange{}, err
	}
	if err := ensureCompleteStudentProfiles(ctx, tx, []string{actorStudentID}, 1); err != nil {
		return SeatChange{}, err
	}
	if err := lockRequestedSeats(ctx, tx, []int64{newSeatID}, 1); err != nil {
		return SeatChange{}, err
	}

	previous, err := getStudentBookingForUpdate(ctx, tx, actorStudentID)
	if err != nil {
		return SeatChange{}, err
	}
	if previous.SeatID == newSeatID {
		return SeatChange{}, ErrSameSeatChange
	}

	booking, err := updateStudentBookingSeat(ctx, tx, actorStudentID, newSeatID)
	if err != nil {
		return SeatChange{}, err
	}
	change := SeatChange{
		Previous: previous,
		Booking:  booking,
	}
	if err := writeChangeAudit(ctx, tx, actorStudentID, change); err != nil {
		return SeatChange{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return SeatChange{}, fmt.Errorf("commit change transaction: %w", err)
	}
	return change, nil
}

func lockRequestedSeats(ctx context.Context, tx pgx.Tx, seatIDs []int64, want int) error {
	rows, err := tx.Query(ctx, `
SELECT seat_id, is_blocked
FROM seats
WHERE seat_id = ANY($1)
FOR UPDATE`, seatIDs)
	if err != nil {
		return fmt.Errorf("lock requested seats: %w", err)
	}
	defer rows.Close()

	found := 0
	for rows.Next() {
		var (
			seatID    int64
			isBlocked bool
		)
		if err := rows.Scan(&seatID, &isBlocked); err != nil {
			return fmt.Errorf("scan locked seat: %w", err)
		}
		found++
		if isBlocked {
			return ErrSeatLocked
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate locked seats: %w", err)
	}
	if found != want {
		return ErrSeatNotFound
	}
	return nil
}

func ensureCompleteStudentProfiles(ctx context.Context, tx pgx.Tx, studentIDs []string, want int) error {
	var count int
	if err := tx.QueryRow(ctx, `
SELECT count(*)
FROM students
WHERE student_id = ANY($1)
  AND class_code IS NOT NULL
  AND class_code ~ '^(7|8|9|10|11|12)0[1-5]$'
  AND seat_no_in_class IS NOT NULL
  AND seat_no_in_class BETWEEN 1 AND 70`, studentIDs).Scan(&count); err != nil {
		return fmt.Errorf("check student registration: %w", err)
	}
	if count != want {
		return ErrIncompleteRegistration
	}
	return nil
}

func ensureBookableStudentRows(ctx context.Context, tx pgx.Tx, actorStudentID string, studentIDs []string) error {
	if _, err := tx.Exec(ctx, `
INSERT INTO students (student_id)
SELECT DISTINCT student_id
FROM unnest($1::varchar[]) AS requested(student_id)
ON CONFLICT (student_id) DO NOTHING`, studentIDs); err != nil {
		return fmt.Errorf("ensure booking student rows: %w", err)
	}

	var actorReady bool
	if err := tx.QueryRow(ctx, `
SELECT class_code IS NOT NULL
   AND class_code ~ '^(7|8|9|10|11|12)0[1-5]$'
   AND seat_no_in_class IS NOT NULL
   AND seat_no_in_class BETWEEN 1 AND 70
FROM students
WHERE student_id = $1`, actorStudentID).Scan(&actorReady); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrIncompleteRegistration
		}
		return fmt.Errorf("check actor registration: %w", err)
	}
	if !actorReady {
		return ErrIncompleteRegistration
	}
	return nil
}

func ensureNoExistingOwnBooking(ctx context.Context, tx pgx.Tx, actorStudentID string) error {
	var count int
	if err := tx.QueryRow(ctx, `
SELECT count(*)
FROM seat_bookings
WHERE student_id = $1`, actorStudentID).Scan(&count); err != nil {
		return fmt.Errorf("check existing own booking: %w", err)
	}
	if count > 0 {
		return ErrBookingConflict
	}
	return nil
}

func insertSeatBookings(
	ctx context.Context,
	tx pgx.Tx,
	actorStudentID string,
	seatIDs []int64,
	studentIDs []string,
	ordinals []int32,
	want int,
) ([]BookedSeat, error) {
	rows, err := tx.Query(ctx, `
WITH requested AS (
  SELECT *
  FROM unnest($1::bigint[], $2::varchar[], $3::int[]) AS r(seat_id, student_id, ord)
),
inserted AS (
  INSERT INTO seat_bookings (seat_id, student_id, booked_by_student_id)
  SELECT seat_id, student_id, $4
  FROM requested
  ON CONFLICT DO NOTHING
  RETURNING seat_id, student_id, ticket_version, booked_at
)
SELECT i.seat_id,
       i.student_id,
       s.zone,
       s.row_no,
       s.col_no,
       i.ticket_version,
       i.booked_at
FROM inserted i
JOIN seats s ON s.seat_id = i.seat_id
JOIN requested r ON r.seat_id = i.seat_id
ORDER BY r.ord`,
		seatIDs,
		studentIDs,
		ordinals,
		actorStudentID,
	)
	if err != nil {
		return nil, fmt.Errorf("insert seat bookings: %w", err)
	}
	defer rows.Close()

	bookings := []BookedSeat{}
	for rows.Next() {
		var booking BookedSeat
		if err := rows.Scan(
			&booking.SeatID,
			&booking.StudentID,
			&booking.Zone,
			&booking.RowNo,
			&booking.ColNo,
			&booking.TicketVersion,
			&booking.BookedAt,
		); err != nil {
			return nil, fmt.Errorf("scan inserted booking: %w", err)
		}
		bookings = append(bookings, booking)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate inserted bookings: %w", err)
	}
	if len(bookings) != want {
		return nil, ErrBookingConflict
	}
	return bookings, nil
}

func deleteOwnBooking(ctx context.Context, tx pgx.Tx, actorStudentID string) ([]BookedSeat, error) {
	rows, err := tx.Query(ctx, `
WITH deleted AS (
  DELETE FROM seat_bookings b
  WHERE b.student_id = $1
  RETURNING b.seat_id,
            b.student_id,
            b.ticket_version,
            b.booked_at
)
SELECT d.seat_id,
       d.student_id,
       s.zone,
       s.row_no,
       s.col_no,
       d.ticket_version,
       d.booked_at
FROM deleted d
JOIN seats s ON s.seat_id = d.seat_id
ORDER BY d.seat_id`, actorStudentID)
	if err != nil {
		return nil, fmt.Errorf("delete own booking: %w", err)
	}
	defer rows.Close()

	bookings := []BookedSeat{}
	for rows.Next() {
		booking, err := scanBookedSeat(rows)
		if err != nil {
			return nil, fmt.Errorf("scan released booking: %w", err)
		}
		bookings = append(bookings, booking)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate released bookings: %w", err)
	}
	if len(bookings) == 0 {
		return nil, ErrBookingNotFound
	}
	return bookings, nil
}

func getStudentBookingForUpdate(ctx context.Context, tx pgx.Tx, actorStudentID string) (BookedSeat, error) {
	row := tx.QueryRow(ctx, `
SELECT b.seat_id,
       b.student_id,
       s.zone,
       s.row_no,
       s.col_no,
       b.ticket_version,
       b.booked_at
FROM seat_bookings b
JOIN seats s ON s.seat_id = b.seat_id
WHERE b.student_id = $1
FOR UPDATE OF b`, actorStudentID)
	booking, err := scanBookedSeat(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return BookedSeat{}, ErrBookingNotFound
	}
	if err != nil {
		return BookedSeat{}, fmt.Errorf("get booking for update: %w", err)
	}
	return booking, nil
}

func updateStudentBookingSeat(ctx context.Context, tx pgx.Tx, actorStudentID string, newSeatID int64) (BookedSeat, error) {
	row := tx.QueryRow(ctx, `
UPDATE seat_bookings b
SET seat_id = $2,
    booked_by_student_id = $1,
    booked_at = now(),
    ticket_version = b.ticket_version + 1
FROM seats s
WHERE b.student_id = $1
  AND s.seat_id = $2
RETURNING b.seat_id,
          b.student_id,
          s.zone,
          s.row_no,
          s.col_no,
          b.ticket_version,
          b.booked_at`, actorStudentID, newSeatID)
	booking, err := scanBookedSeat(row)
	if isUniqueViolation(err) {
		return BookedSeat{}, ErrBookingConflict
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return BookedSeat{}, ErrBookingNotFound
	}
	if err != nil {
		return BookedSeat{}, fmt.Errorf("update booking seat: %w", err)
	}
	return booking, nil
}

type bookedSeatScanner interface {
	Scan(dest ...any) error
}

func scanBookedSeat(row bookedSeatScanner) (BookedSeat, error) {
	var booking BookedSeat
	if err := row.Scan(
		&booking.SeatID,
		&booking.StudentID,
		&booking.Zone,
		&booking.RowNo,
		&booking.ColNo,
		&booking.TicketVersion,
		&booking.BookedAt,
	); err != nil {
		return BookedSeat{}, err
	}
	return booking, nil
}

func writeBookingAudit(ctx context.Context, tx pgx.Tx, actorStudentID string, booking BookedSeat) error {
	_, err := tx.Exec(ctx, `
INSERT INTO audit_logs (actor_type, actor_id, action, target_seat_id, target_student, after_json)
VALUES (
  'student',
  $1,
  'book',
  $2,
  $3,
  jsonb_build_object(
    'seat_id', $2::bigint,
    'student_id', $3::varchar,
    'booked_by_student_id', $1::varchar,
    'ticket_version', $4::int
  )
)`,
		actorStudentID,
		booking.SeatID,
		booking.StudentID,
		booking.TicketVersion,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrBookingConflict
		}
		return fmt.Errorf("write booking audit: %w", err)
	}
	return nil
}

func writeReleaseAudit(ctx context.Context, tx pgx.Tx, actorStudentID string, booking BookedSeat) error {
	_, err := tx.Exec(ctx, `
INSERT INTO audit_logs (actor_type, actor_id, action, target_seat_id, target_student, before_json)
VALUES (
  'student',
  $1,
  'release',
  $2,
  $3,
  jsonb_build_object(
    'seat_id', $2::bigint,
    'student_id', $3::varchar,
    'ticket_version', $4::int
  )
)`,
		actorStudentID,
		booking.SeatID,
		booking.StudentID,
		booking.TicketVersion,
	)
	if err != nil {
		return fmt.Errorf("write release audit: %w", err)
	}
	return nil
}

func writeChangeAudit(ctx context.Context, tx pgx.Tx, actorStudentID string, change SeatChange) error {
	_, err := tx.Exec(ctx, `
INSERT INTO audit_logs (actor_type, actor_id, action, target_seat_id, target_student, before_json, after_json)
VALUES (
  'student',
  $1,
  'change_seat',
  $2,
  $3,
  jsonb_build_object(
    'seat_id', $4::bigint,
    'student_id', $3::varchar,
    'ticket_version', $5::int
  ),
  jsonb_build_object(
    'seat_id', $2::bigint,
    'student_id', $3::varchar,
    'ticket_version', $6::int
  )
)`,
		actorStudentID,
		change.Booking.SeatID,
		change.Booking.StudentID,
		change.Previous.SeatID,
		change.Previous.TicketVersion,
		change.Booking.TicketVersion,
	)
	if err != nil {
		return fmt.Errorf("write change audit: %w", err)
	}
	return nil
}

func ensureReleaseAllowed(ctx context.Context, tx pgx.Tx, now time.Time) error {
	showStart, ok, err := getConfigTime(ctx, tx, "show_start_at")
	if err != nil {
		return err
	}
	if ok && !now.Before(showStart) {
		return ErrBookingFrozen
	}
	return nil
}

func ensureChangeAllowed(ctx context.Context, tx pgx.Tx, now time.Time) error {
	if err := ensureReleaseAllowed(ctx, tx, now); err != nil {
		return err
	}
	openAt, ok, err := getConfigTime(ctx, tx, "booking_open_at")
	if err != nil {
		return err
	}
	if ok && now.Before(openAt) {
		return ErrBookingNotOpen
	}
	lockAt, ok, err := getConfigTime(ctx, tx, "booking_lock_at")
	if err != nil {
		return err
	}
	if ok && !now.Before(lockAt) {
		return ErrChangeLocked
	}
	return nil
}

func ensureBookingAllowed(ctx context.Context, tx pgx.Tx, now time.Time) error {
	if err := ensureReleaseAllowed(ctx, tx, now); err != nil {
		return err
	}
	openAt, ok, err := getConfigTime(ctx, tx, "booking_open_at")
	if err != nil {
		return err
	}
	if ok && now.Before(openAt) {
		return ErrBookingNotOpen
	}
	lockAt, ok, err := getConfigTime(ctx, tx, "booking_lock_at")
	if err != nil {
		return err
	}
	if ok && !now.Before(lockAt) {
		return ErrBookingLocked
	}
	return nil
}

func getConfigTime(ctx context.Context, tx pgx.Tx, key string) (time.Time, bool, error) {
	var raw []byte
	if err := tx.QueryRow(ctx, `
SELECT value_json
FROM system_config
WHERE key = $1`, key).Scan(&raw); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return time.Time{}, false, nil
		}
		return time.Time{}, false, fmt.Errorf("read %s config: %w", key, err)
	}

	value, err := parseConfigTime(raw)
	if err != nil {
		return time.Time{}, false, fmt.Errorf("parse %s config: %w", key, err)
	}
	return value, true, nil
}

func parseConfigTime(raw []byte) (time.Time, error) {
	var asString string
	if err := json.Unmarshal(raw, &asString); err == nil {
		return time.Parse(time.RFC3339, asString)
	}

	var envelope struct {
		Value string `json:"value"`
	}
	if err := json.Unmarshal(raw, &envelope); err == nil && envelope.Value != "" {
		return time.Parse(time.RFC3339, envelope.Value)
	}

	return time.Time{}, errors.New("expected RFC3339 string")
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == uniqueViolationCode
}
