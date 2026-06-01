package seat

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPostgresBookSeatsIntegration(t *testing.T) {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Skip("DATABASE_URL is not set")
	}

	ctx := context.Background()
	db, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect postgres: %v", err)
	}
	defer db.Close()

	seatID := firstFreeSeatID(t, ctx, db)
	studentID := fmt.Sprintf("7%06d", time.Now().UnixNano()%1_000_000)
	ensureRegisteredStudent(t, ctx, db, studentID)

	store := NewPostgresSnapshotStore(db)
	bookings, err := store.BookSeats(ctx, studentID, []BookEntry{{
		SeatID:    seatID,
		StudentID: studentID,
	}})
	if err != nil {
		t.Fatalf("BookSeats() error = %v", err)
	}
	if len(bookings) != 1 {
		t.Fatalf("len(bookings) = %d, want 1", len(bookings))
	}
	if bookings[0].SeatID != seatID || bookings[0].StudentID != studentID {
		t.Fatalf("booking = %#v, want seat_id=%d student_id=%s", bookings[0], seatID, studentID)
	}
}

func TestPostgresChangeAndReleaseSeatIntegration(t *testing.T) {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Skip("DATABASE_URL is not set")
	}

	ctx := context.Background()
	db, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect postgres: %v", err)
	}
	defer db.Close()

	firstSeatID := firstFreeSeatID(t, ctx, db)
	studentID := fmt.Sprintf("8%06d", time.Now().UnixNano()%1_000_000)
	ensureRegisteredStudent(t, ctx, db, studentID)

	store := NewPostgresSnapshotStore(db)
	bookings, err := store.BookSeats(ctx, studentID, []BookEntry{{
		SeatID:    firstSeatID,
		StudentID: studentID,
	}})
	if err != nil {
		t.Fatalf("BookSeats() error = %v", err)
	}
	if len(bookings) != 1 {
		t.Fatalf("len(bookings) = %d, want 1", len(bookings))
	}

	secondSeatID := firstFreeSeatID(t, ctx, db)
	change, err := store.ChangeSeat(ctx, studentID, secondSeatID)
	if err != nil {
		t.Fatalf("ChangeSeat() error = %v", err)
	}
	if change.Previous.SeatID != firstSeatID || change.Booking.SeatID != secondSeatID {
		t.Fatalf("change = %#v, want from %d to %d", change, firstSeatID, secondSeatID)
	}
	if change.Booking.TicketVersion != change.Previous.TicketVersion+1 {
		t.Fatalf("ticket version = %d, want previous + 1", change.Booking.TicketVersion)
	}

	releasedBookings, err := store.ReleaseSeats(ctx, studentID)
	if err != nil {
		t.Fatalf("ReleaseSeats() error = %v", err)
	}
	if len(releasedBookings) != 1 {
		t.Fatalf("len(releasedBookings) = %d, want 1", len(releasedBookings))
	}
	released := releasedBookings[0]
	if released.SeatID != secondSeatID {
		t.Fatalf("released seat = %d, want %d", released.SeatID, secondSeatID)
	}

	var count int
	if err := db.QueryRow(ctx, `
SELECT count(*)
FROM seat_bookings
WHERE student_id = $1`, studentID).Scan(&count); err != nil {
		t.Fatalf("count released booking: %v", err)
	}
	if count != 0 {
		t.Fatalf("booking count after release = %d, want 0", count)
	}
}

func TestPostgresProxyBatchReleaseIntegration(t *testing.T) {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Skip("DATABASE_URL is not set")
	}

	ctx := context.Background()
	db, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect postgres: %v", err)
	}
	defer db.Close()

	seatIDs := firstNFreeSeatIDs(t, ctx, db, 3)
	actorID := fmt.Sprintf("5%06d", time.Now().UnixNano()%1_000_000)
	proxyID := fmt.Sprintf("4%06d", time.Now().UnixNano()%1_000_000)
	ensureRegisteredStudent(t, ctx, db, actorID)

	store := NewPostgresSnapshotStore(db)
	bookings, err := store.BookSeats(ctx, actorID, []BookEntry{
		{SeatID: seatIDs[0], StudentID: actorID},
		{SeatID: seatIDs[1], StudentID: proxyID},
	})
	if err != nil {
		t.Fatalf("BookSeats(proxy batch) error = %v", err)
	}
	if len(bookings) != 2 {
		t.Fatalf("len(bookings) = %d, want 2", len(bookings))
	}
	assertSemiRegisteredStudentExists(t, ctx, db, proxyID)

	if _, err := store.BookSeats(ctx, actorID, []BookEntry{{SeatID: seatIDs[2], StudentID: actorID}}); !errors.Is(err, ErrBookingConflict) {
		t.Fatalf("BookSeats(second batch) error = %v, want %v", err, ErrBookingConflict)
	}

	released, err := store.ReleaseSeats(ctx, actorID)
	if err != nil {
		t.Fatalf("ReleaseSeats(proxy batch) error = %v", err)
	}
	if len(released) != 2 {
		t.Fatalf("len(released) = %d, want 2", len(released))
	}
	if released[0].StudentID != actorID {
		t.Fatalf("first released student = %q, want actor %q", released[0].StudentID, actorID)
	}

	var count int
	if err := db.QueryRow(ctx, `
SELECT count(*)
FROM seat_bookings
WHERE student_id = ANY($1)
   OR booked_by_student_id = $2`, []string{actorID, proxyID}, actorID).Scan(&count); err != nil {
		t.Fatalf("count released proxy batch: %v", err)
	}
	if count != 0 {
		t.Fatalf("booking count after proxy release = %d, want 0", count)
	}
}

func TestPostgresBookingWindowIntegration(t *testing.T) {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Skip("DATABASE_URL is not set")
	}

	ctx := context.Background()
	db, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect postgres: %v", err)
	}
	defer db.Close()

	now := time.Now().UTC()
	defer func() {
		setConfigTime(t, ctx, db, "booking_open_at", now.Add(-time.Hour))
		setConfigTime(t, ctx, db, "booking_lock_at", now.Add(30*24*time.Hour))
		setConfigTime(t, ctx, db, "show_start_at", now.Add(31*24*time.Hour))
	}()

	store := NewPostgresSnapshotStore(db)
	seatID := firstFreeSeatID(t, ctx, db)
	studentID := fmt.Sprintf("9%06d", time.Now().UnixNano()%1_000_000)
	ensureRegisteredStudent(t, ctx, db, studentID)

	setConfigTime(t, ctx, db, "booking_open_at", now.Add(time.Hour))
	setConfigTime(t, ctx, db, "booking_lock_at", now.Add(2*time.Hour))
	setConfigTime(t, ctx, db, "show_start_at", now.Add(3*time.Hour))
	if _, err := store.BookSeats(ctx, studentID, []BookEntry{{SeatID: seatID, StudentID: studentID}}); !errors.Is(err, ErrBookingNotOpen) {
		t.Fatalf("BookSeats(before open) error = %v, want %v", err, ErrBookingNotOpen)
	}

	setConfigTime(t, ctx, db, "booking_open_at", now.Add(-2*time.Hour))
	setConfigTime(t, ctx, db, "booking_lock_at", now.Add(-time.Hour))
	if _, err := store.BookSeats(ctx, studentID, []BookEntry{{SeatID: seatID, StudentID: studentID}}); !errors.Is(err, ErrBookingLocked) {
		t.Fatalf("BookSeats(after lock) error = %v, want %v", err, ErrBookingLocked)
	}
}

func TestPostgresBookSeatsRequiresCompleteActorProfileIntegration(t *testing.T) {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Skip("DATABASE_URL is not set")
	}

	ctx := context.Background()
	db, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect postgres: %v", err)
	}
	defer db.Close()

	seatID := firstFreeSeatID(t, ctx, db)
	studentID := fmt.Sprintf("6%06d", time.Now().UnixNano()%1_000_000)
	if _, err := db.Exec(ctx, `
INSERT INTO students (student_id)
VALUES ($1)
ON CONFLICT (student_id) DO UPDATE
SET class_code = NULL,
    seat_no_in_class = NULL,
    updated_at = now()`, studentID); err != nil {
		t.Fatalf("insert incomplete student: %v", err)
	}

	store := NewPostgresSnapshotStore(db)
	if _, err := store.BookSeats(ctx, studentID, []BookEntry{{SeatID: seatID, StudentID: studentID}}); !errors.Is(err, ErrIncompleteRegistration) {
		t.Fatalf("BookSeats(incomplete actor) error = %v, want %v", err, ErrIncompleteRegistration)
	}
}

func firstNFreeSeatIDs(t *testing.T, ctx context.Context, db *pgxpool.Pool, want int) []int64 {
	t.Helper()

	rows, err := db.Query(ctx, `
SELECT s.seat_id
FROM seats s
WHERE NOT s.is_blocked
  AND NOT EXISTS (
    SELECT 1
    FROM seat_bookings b
    WHERE b.seat_id = s.seat_id
  )
ORDER BY s.seat_id
LIMIT $1`, want)
	if err != nil {
		t.Fatalf("find free seats: %v", err)
	}
	defer rows.Close()

	seatIDs := []int64{}
	for rows.Next() {
		var seatID int64
		if err := rows.Scan(&seatID); err != nil {
			t.Fatalf("scan free seat: %v", err)
		}
		seatIDs = append(seatIDs, seatID)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate free seats: %v", err)
	}
	if len(seatIDs) != want {
		t.Fatalf("free seats = %d, want %d", len(seatIDs), want)
	}
	return seatIDs
}

func firstFreeSeatID(t *testing.T, ctx context.Context, db *pgxpool.Pool) int64 {
	t.Helper()

	var seatID int64
	err := db.QueryRow(ctx, `
SELECT s.seat_id
FROM seats s
WHERE NOT s.is_blocked
  AND NOT EXISTS (
    SELECT 1
    FROM seat_bookings b
    WHERE b.seat_id = s.seat_id
  )
ORDER BY s.seat_id
LIMIT 1`).Scan(&seatID)
	if err != nil {
		t.Fatalf("find free seat: %v", err)
	}
	return seatID
}

func setConfigTime(t *testing.T, ctx context.Context, db *pgxpool.Pool, key string, value time.Time) {
	t.Helper()

	if _, err := db.Exec(ctx, `
INSERT INTO system_config (key, value_json, updated_by, updated_at)
VALUES ($1, to_jsonb($2::text), 'test', now())
ON CONFLICT (key) DO UPDATE
SET value_json = excluded.value_json,
    updated_by = excluded.updated_by,
    updated_at = now()`, key, value.UTC().Format(time.RFC3339)); err != nil {
		t.Fatalf("set config %s: %v", key, err)
	}
}

func ensureRegisteredStudent(t *testing.T, ctx context.Context, db *pgxpool.Pool, studentID string) {
	t.Helper()

	if _, err := db.Exec(ctx, `
INSERT INTO students (student_id, class_code, seat_no_in_class)
VALUES ($1, '1103', 18)
ON CONFLICT (student_id) DO UPDATE
SET class_code = excluded.class_code,
    seat_no_in_class = excluded.seat_no_in_class,
    updated_at = now()`, studentID); err != nil {
		t.Fatalf("ensure registered student: %v", err)
	}
}

func assertSemiRegisteredStudentExists(t *testing.T, ctx context.Context, db *pgxpool.Pool, studentID string) {
	t.Helper()

	var count int
	if err := db.QueryRow(ctx, `
SELECT count(*)
FROM students
WHERE student_id = $1
  AND class_code IS NULL
  AND seat_no_in_class IS NULL`, studentID).Scan(&count); err != nil {
		t.Fatalf("find semi-registered student: %v", err)
	}
	if count != 1 {
		t.Fatalf("semi-registered student count = %d, want 1", count)
	}
}
