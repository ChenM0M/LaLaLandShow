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

	released, err := store.ReleaseSeat(ctx, studentID)
	if err != nil {
		t.Fatalf("ReleaseSeat() error = %v", err)
	}
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
