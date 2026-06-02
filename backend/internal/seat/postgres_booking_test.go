package seat

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
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
	ensureBookingWindowOpen(t, ctx, db)

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
	ensureBookingWindowOpen(t, ctx, db)

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

func TestPostgresProxyBookingOwnershipIntegration(t *testing.T) {
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
	ensureBookingWindowOpen(t, ctx, db)

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
		t.Fatalf("ReleaseSeats(actor with proxy booking) error = %v", err)
	}
	if len(released) != 1 {
		t.Fatalf("len(released) = %d, want 1", len(released))
	}
	if released[0].StudentID != actorID || released[0].SeatID != seatIDs[0] {
		t.Fatalf("released = %#v, want actor seat %d only", released[0], seatIDs[0])
	}

	var count int
	if err := db.QueryRow(ctx, `
SELECT count(*)
FROM seat_bookings
WHERE student_id = $1
  AND booked_by_student_id = $2`, proxyID, actorID).Scan(&count); err != nil {
		t.Fatalf("count retained proxy booking: %v", err)
	}
	if count != 1 {
		t.Fatalf("proxy booking count after actor release = %d, want 1", count)
	}

	if _, err := store.BookSeats(ctx, actorID, []BookEntry{
		{SeatID: seatIDs[0], StudentID: actorID},
		{SeatID: seatIDs[2], StudentID: proxyID},
	}); !errors.Is(err, ErrBookingConflict) {
		t.Fatalf("BookSeats(actor rebooks occupied proxy) error = %v, want %v", err, ErrBookingConflict)
	}
	assertNoBookingForStudent(t, ctx, db, actorID)
	assertSingleBookingForStudent(t, ctx, db, proxyID)

	releasedProxy, err := store.ReleaseSeats(ctx, proxyID)
	if err != nil {
		t.Fatalf("ReleaseSeats(proxy owner) error = %v", err)
	}
	if len(releasedProxy) != 1 || releasedProxy[0].StudentID != proxyID {
		t.Fatalf("released proxy = %#v, want proxy owner's own booking", releasedProxy)
	}
	assertNoBookingForStudent(t, ctx, db, proxyID)
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

func TestPostgresSeatOperationsStageMatrixIntegration(t *testing.T) {
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
	defer ensureBookingWindowOpen(t, ctx, db)

	store := NewPostgresSnapshotStore(db)

	t.Run("before open rejects booking changes but allows release of existing booking", func(t *testing.T) {
		seatIDs := firstNFreeSeatIDs(t, ctx, db, 4)
		studentID := uniqueStudentID("1", 10)
		ensureRegisteredStudent(t, ctx, db, studentID)
		bookSeatForStageTest(t, ctx, db, store, studentID, seatIDs[0])
		setBookingStage(t, ctx, db, bookingStageBeforeOpen)

		if _, err := store.BookSeats(ctx, uniqueStudentID("1", 11), []BookEntry{{SeatID: seatIDs[1], StudentID: uniqueStudentID("1", 11)}}); !errors.Is(err, ErrBookingNotOpen) {
			t.Fatalf("BookSeats(before open) error = %v, want %v", err, ErrBookingNotOpen)
		}
		if _, err := store.RebookSeats(ctx, studentID, []BookEntry{{SeatID: seatIDs[1], StudentID: studentID}}); !errors.Is(err, ErrBookingNotOpen) {
			t.Fatalf("RebookSeats(before open) error = %v, want %v", err, ErrBookingNotOpen)
		}
		if _, err := store.ChangeSeat(ctx, studentID, seatIDs[2]); !errors.Is(err, ErrBookingNotOpen) {
			t.Fatalf("ChangeSeat(before open) error = %v, want %v", err, ErrBookingNotOpen)
		}
		if released, err := store.ReleaseSeats(ctx, studentID); err != nil || len(released) != 1 {
			t.Fatalf("ReleaseSeats(before open) = len %d, err %v; want one released booking", len(released), err)
		}
		assertNoBookingForStudent(t, ctx, db, studentID)
	})

	t.Run("open allows book rebook change and release", func(t *testing.T) {
		setBookingStage(t, ctx, db, bookingStageOpen)
		seatIDs := firstNFreeSeatIDs(t, ctx, db, 4)
		studentID := uniqueStudentID("2", 20)
		ensureRegisteredStudent(t, ctx, db, studentID)

		if bookings, err := store.BookSeats(ctx, studentID, []BookEntry{{SeatID: seatIDs[0], StudentID: studentID}}); err != nil || len(bookings) != 1 {
			t.Fatalf("BookSeats(open) = len %d, err %v; want one booking", len(bookings), err)
		}
		if result, err := store.RebookSeats(ctx, studentID, []BookEntry{{SeatID: seatIDs[1], StudentID: studentID}}); err != nil || len(result.Bookings) != 1 || len(result.ReleasedBookings) != 1 {
			t.Fatalf("RebookSeats(open) = %#v, err %v; want one release and one booking", result, err)
		}
		if change, err := store.ChangeSeat(ctx, studentID, seatIDs[2]); err != nil || change.Booking.SeatID != seatIDs[2] {
			t.Fatalf("ChangeSeat(open) = %#v, err %v; want seat %d", change, err, seatIDs[2])
		}
		if released, err := store.ReleaseSeats(ctx, studentID); err != nil || len(released) != 1 {
			t.Fatalf("ReleaseSeats(open) = len %d, err %v; want one released booking", len(released), err)
		}
		assertNoBookingForStudent(t, ctx, db, studentID)
	})

	t.Run("locked rejects booking and seat changes but allows release", func(t *testing.T) {
		seatIDs := firstNFreeSeatIDs(t, ctx, db, 4)
		studentID := uniqueStudentID("3", 30)
		otherID := uniqueStudentID("3", 31)
		ensureRegisteredStudent(t, ctx, db, studentID)
		ensureRegisteredStudent(t, ctx, db, otherID)
		bookSeatForStageTest(t, ctx, db, store, studentID, seatIDs[0])
		setBookingStage(t, ctx, db, bookingStageLocked)

		if _, err := store.BookSeats(ctx, otherID, []BookEntry{{SeatID: seatIDs[1], StudentID: otherID}}); !errors.Is(err, ErrBookingLocked) {
			t.Fatalf("BookSeats(locked) error = %v, want %v", err, ErrBookingLocked)
		}
		if _, err := store.RebookSeats(ctx, studentID, []BookEntry{{SeatID: seatIDs[1], StudentID: studentID}}); !errors.Is(err, ErrBookingLocked) {
			t.Fatalf("RebookSeats(locked) error = %v, want %v", err, ErrBookingLocked)
		}
		if _, err := store.ChangeSeat(ctx, studentID, seatIDs[2]); !errors.Is(err, ErrChangeLocked) {
			t.Fatalf("ChangeSeat(locked) error = %v, want %v", err, ErrChangeLocked)
		}
		if released, err := store.ReleaseSeats(ctx, studentID); err != nil || len(released) != 1 {
			t.Fatalf("ReleaseSeats(locked) = len %d, err %v; want one released booking", len(released), err)
		}
		assertNoBookingForStudent(t, ctx, db, studentID)
	})

	t.Run("show started freezes every seat operation", func(t *testing.T) {
		seatIDs := firstNFreeSeatIDs(t, ctx, db, 4)
		studentID := uniqueStudentID("4", 40)
		otherID := uniqueStudentID("4", 41)
		ensureRegisteredStudent(t, ctx, db, studentID)
		ensureRegisteredStudent(t, ctx, db, otherID)
		bookSeatForStageTest(t, ctx, db, store, studentID, seatIDs[0])
		setBookingStage(t, ctx, db, bookingStageShowStarted)

		if _, err := store.BookSeats(ctx, otherID, []BookEntry{{SeatID: seatIDs[1], StudentID: otherID}}); !errors.Is(err, ErrBookingFrozen) {
			t.Fatalf("BookSeats(show started) error = %v, want %v", err, ErrBookingFrozen)
		}
		if _, err := store.RebookSeats(ctx, studentID, []BookEntry{{SeatID: seatIDs[1], StudentID: studentID}}); !errors.Is(err, ErrBookingFrozen) {
			t.Fatalf("RebookSeats(show started) error = %v, want %v", err, ErrBookingFrozen)
		}
		if _, err := store.ChangeSeat(ctx, studentID, seatIDs[2]); !errors.Is(err, ErrBookingFrozen) {
			t.Fatalf("ChangeSeat(show started) error = %v, want %v", err, ErrBookingFrozen)
		}
		if _, err := store.ReleaseSeats(ctx, studentID); !errors.Is(err, ErrBookingFrozen) {
			t.Fatalf("ReleaseSeats(show started) error = %v, want %v", err, ErrBookingFrozen)
		}
		assertSingleBookingForStudent(t, ctx, db, studentID)
	})
}

func TestPostgresSeatOperationConflictEdgesIntegration(t *testing.T) {
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
	ensureBookingWindowOpen(t, ctx, db)

	store := NewPostgresSnapshotStore(db)
	seatIDs := firstNFreeSeatIDs(t, ctx, db, 8)
	studentA := uniqueStudentID("5", 50)
	studentB := uniqueStudentID("5", 51)
	studentC := uniqueStudentID("5", 52)
	studentD := uniqueStudentID("5", 53)
	for _, studentID := range []string{studentA, studentB, studentC, studentD} {
		ensureRegisteredStudent(t, ctx, db, studentID)
	}

	if _, err := store.BookSeats(ctx, studentA, []BookEntry{{SeatID: seatIDs[0], StudentID: studentA}}); err != nil {
		t.Fatalf("BookSeats(student A) error = %v", err)
	}
	if _, err := store.BookSeats(ctx, studentB, []BookEntry{{SeatID: seatIDs[1], StudentID: studentB}}); err != nil {
		t.Fatalf("BookSeats(student B) error = %v", err)
	}

	if _, err := store.BookSeats(ctx, studentC, []BookEntry{{SeatID: seatIDs[0], StudentID: studentC}}); !errors.Is(err, ErrBookingConflict) {
		t.Fatalf("BookSeats(occupied seat) error = %v, want %v", err, ErrBookingConflict)
	}
	if _, err := store.BookSeats(ctx, studentC, []BookEntry{{SeatID: seatIDs[2], StudentID: studentA}}); !errors.Is(err, ErrBookingConflict) {
		t.Fatalf("BookSeats(already booked student) error = %v, want %v", err, ErrBookingConflict)
	}
	if _, err := store.ChangeSeat(ctx, studentA, seatIDs[0]); !errors.Is(err, ErrSameSeatChange) {
		t.Fatalf("ChangeSeat(same seat) error = %v, want %v", err, ErrSameSeatChange)
	}
	if _, err := store.ChangeSeat(ctx, studentA, seatIDs[1]); !errors.Is(err, ErrBookingConflict) {
		t.Fatalf("ChangeSeat(occupied seat) error = %v, want %v", err, ErrBookingConflict)
	}

	setSeatBlocked(t, ctx, db, seatIDs[3], true)
	defer setSeatBlocked(t, ctx, db, seatIDs[3], false)
	if _, err := store.BookSeats(ctx, studentC, []BookEntry{{SeatID: seatIDs[3], StudentID: studentC}}); !errors.Is(err, ErrSeatLocked) {
		t.Fatalf("BookSeats(blocked seat) error = %v, want %v", err, ErrSeatLocked)
	}
	if _, err := store.ChangeSeat(ctx, studentA, seatIDs[3]); !errors.Is(err, ErrSeatLocked) {
		t.Fatalf("ChangeSeat(blocked seat) error = %v, want %v", err, ErrSeatLocked)
	}

	if _, err := store.BookSeats(ctx, studentC, []BookEntry{{SeatID: missingSeatID(t, ctx, db), StudentID: studentC}}); !errors.Is(err, ErrSeatNotFound) {
		t.Fatalf("BookSeats(missing seat) error = %v, want %v", err, ErrSeatNotFound)
	}
	if _, err := store.ReleaseSeats(ctx, studentD); !errors.Is(err, ErrBookingNotFound) {
		t.Fatalf("ReleaseSeats(no booking) error = %v, want %v", err, ErrBookingNotFound)
	}

	assertSingleBookingForStudent(t, ctx, db, studentA)
	assertSingleBookingForStudent(t, ctx, db, studentB)
	assertNoBookingForStudent(t, ctx, db, studentC)
	assertNoBookingForStudent(t, ctx, db, studentD)
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
	ensureBookingWindowOpen(t, ctx, db)

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

func TestPostgresConcurrentProxyBookingForSameStudentIntegration(t *testing.T) {
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
	ensureBookingWindowOpen(t, ctx, db)

	store := NewPostgresSnapshotStore(db)
	seatIDs := firstNFreeSeatIDs(t, ctx, db, 12)
	targetID := uniqueStudentID("1", 0)
	actorIDs := make([]string, 5)
	for i := range actorIDs {
		actorIDs[i] = uniqueStudentID("2", i)
		ensureRegisteredStudent(t, ctx, db, actorIDs[i])
	}

	results := runConcurrentBookings(actorIDs, func(i int, actorID string) error {
		_, err := store.BookSeats(ctx, actorID, []BookEntry{
			{SeatID: seatIDs[i*2], StudentID: actorID},
			{SeatID: seatIDs[i*2+1], StudentID: targetID},
		})
		return err
	})

	assertOneSuccessAndConflicts(t, results)
	assertBookingCountForStudents(t, ctx, db, append(actorIDs, targetID), 2)
	assertSingleBookingForStudent(t, ctx, db, targetID)
}

func TestPostgresConcurrentSelfAndProxyBookingSameStudentIntegration(t *testing.T) {
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
	ensureBookingWindowOpen(t, ctx, db)

	store := NewPostgresSnapshotStore(db)
	seatIDs := firstNFreeSeatIDs(t, ctx, db, 3)
	studentID := uniqueStudentID("3", 0)
	proxyID := uniqueStudentID("4", 0)
	ensureRegisteredStudent(t, ctx, db, studentID)
	ensureRegisteredStudent(t, ctx, db, proxyID)

	actors := []string{studentID, proxyID}
	results := runConcurrentBookings(actors, func(i int, actorID string) error {
		if actorID == studentID {
			_, err := store.BookSeats(ctx, actorID, []BookEntry{{SeatID: seatIDs[0], StudentID: studentID}})
			return err
		}
		_, err := store.BookSeats(ctx, actorID, []BookEntry{
			{SeatID: seatIDs[1], StudentID: proxyID},
			{SeatID: seatIDs[2], StudentID: studentID},
		})
		return err
	})

	assertOneSuccessAndConflicts(t, results)
	assertSingleBookingForStudent(t, ctx, db, studentID)
}

func TestPostgresConcurrentSameAccountDifferentBatchesIntegration(t *testing.T) {
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
	ensureBookingWindowOpen(t, ctx, db)

	store := NewPostgresSnapshotStore(db)
	seatIDs := firstNFreeSeatIDs(t, ctx, db, 6)
	actorID := uniqueStudentID("5", 0)
	proxyIDs := []string{uniqueStudentID("6", 0), uniqueStudentID("6", 1)}
	ensureRegisteredStudent(t, ctx, db, actorID)

	actors := []string{actorID, actorID}
	results := runConcurrentBookings(actors, func(i int, actorID string) error {
		_, err := store.BookSeats(ctx, actorID, []BookEntry{
			{SeatID: seatIDs[i*2], StudentID: actorID},
			{SeatID: seatIDs[i*2+1], StudentID: proxyIDs[i]},
		})
		return err
	})

	assertOneSuccessAndConflicts(t, results)
	assertSingleBookingForStudent(t, ctx, db, actorID)
	assertBookingCountForStudents(t, ctx, db, append(proxyIDs, actorID), 2)
}

func TestPostgresLaterProxyBookingForAlreadyBookedStudentIntegration(t *testing.T) {
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
	ensureBookingWindowOpen(t, ctx, db)

	store := NewPostgresSnapshotStore(db)
	seatIDs := firstNFreeSeatIDs(t, ctx, db, 4)
	actorAID := uniqueStudentID("7", 0)
	actorCID := uniqueStudentID("8", 0)
	targetID := uniqueStudentID("9", 0)
	ensureRegisteredStudent(t, ctx, db, actorAID)
	ensureRegisteredStudent(t, ctx, db, actorCID)

	if _, err := store.BookSeats(ctx, actorAID, []BookEntry{
		{SeatID: seatIDs[0], StudentID: actorAID},
		{SeatID: seatIDs[1], StudentID: targetID},
	}); err != nil {
		t.Fatalf("BookSeats(actor A) error = %v", err)
	}
	if _, err := store.BookSeats(ctx, actorCID, []BookEntry{
		{SeatID: seatIDs[2], StudentID: actorCID},
		{SeatID: seatIDs[3], StudentID: targetID},
	}); !errors.Is(err, ErrBookingConflict) {
		t.Fatalf("BookSeats(actor C for booked target) error = %v, want %v", err, ErrBookingConflict)
	}

	assertBookingCountForStudents(t, ctx, db, []string{actorAID, actorCID, targetID}, 2)
	assertSingleBookingForStudent(t, ctx, db, targetID)
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

func missingSeatID(t *testing.T, ctx context.Context, db *pgxpool.Pool) int64 {
	t.Helper()

	var seatID int64
	if err := db.QueryRow(ctx, `
SELECT COALESCE(max(seat_id), 0) + 1000000
FROM seats`).Scan(&seatID); err != nil {
		t.Fatalf("find missing seat id: %v", err)
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

type bookingStage string

const (
	bookingStageBeforeOpen  bookingStage = "before_open"
	bookingStageOpen        bookingStage = "open"
	bookingStageLocked      bookingStage = "locked"
	bookingStageShowStarted bookingStage = "show_started"
)

func setBookingStage(t *testing.T, ctx context.Context, db *pgxpool.Pool, stage bookingStage) {
	t.Helper()

	now := time.Now().UTC()
	switch stage {
	case bookingStageBeforeOpen:
		setConfigTime(t, ctx, db, "booking_open_at", now.Add(time.Hour))
		setConfigTime(t, ctx, db, "booking_lock_at", now.Add(2*time.Hour))
		setConfigTime(t, ctx, db, "show_start_at", now.Add(3*time.Hour))
	case bookingStageOpen:
		setConfigTime(t, ctx, db, "booking_open_at", now.Add(-time.Hour))
		setConfigTime(t, ctx, db, "booking_lock_at", now.Add(time.Hour))
		setConfigTime(t, ctx, db, "show_start_at", now.Add(2*time.Hour))
	case bookingStageLocked:
		setConfigTime(t, ctx, db, "booking_open_at", now.Add(-2*time.Hour))
		setConfigTime(t, ctx, db, "booking_lock_at", now.Add(-time.Hour))
		setConfigTime(t, ctx, db, "show_start_at", now.Add(time.Hour))
	case bookingStageShowStarted:
		setConfigTime(t, ctx, db, "booking_open_at", now.Add(-3*time.Hour))
		setConfigTime(t, ctx, db, "booking_lock_at", now.Add(-2*time.Hour))
		setConfigTime(t, ctx, db, "show_start_at", now.Add(-time.Hour))
	default:
		t.Fatalf("unknown booking stage %q", stage)
	}
}

func ensureBookingWindowOpen(t *testing.T, ctx context.Context, db *pgxpool.Pool) {
	t.Helper()

	setBookingStage(t, ctx, db, bookingStageOpen)
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

func setSeatBlocked(t *testing.T, ctx context.Context, db *pgxpool.Pool, seatID int64, blocked bool) {
	t.Helper()

	if _, err := db.Exec(ctx, `
UPDATE seats
SET is_blocked = $2,
    block_reason = CASE WHEN $2 THEN 'test' ELSE NULL END,
    updated_at = now()
WHERE seat_id = $1`, seatID, blocked); err != nil {
		t.Fatalf("set seat %d blocked=%t: %v", seatID, blocked, err)
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

func runConcurrentBookings(actors []string, book func(int, string) error) []error {
	var (
		ready   sync.WaitGroup
		start   = make(chan struct{})
		done    sync.WaitGroup
		results = make([]error, len(actors))
	)
	ready.Add(len(actors))
	done.Add(len(actors))
	for i, actorID := range actors {
		go func(i int, actorID string) {
			defer done.Done()
			ready.Done()
			<-start
			results[i] = book(i, actorID)
		}(i, actorID)
	}
	ready.Wait()
	close(start)
	done.Wait()
	return results
}

func assertOneSuccessAndConflicts(t *testing.T, results []error) {
	t.Helper()

	successes := 0
	conflicts := 0
	for _, err := range results {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, ErrBookingConflict):
			conflicts++
		default:
			t.Fatalf("concurrent booking error = %v, want nil or %v", err, ErrBookingConflict)
		}
	}
	if successes != 1 {
		t.Fatalf("successful bookings = %d, want 1; results=%v", successes, results)
	}
	if conflicts != len(results)-1 {
		t.Fatalf("booking conflicts = %d, want %d; results=%v", conflicts, len(results)-1, results)
	}
}

func assertSingleBookingForStudent(t *testing.T, ctx context.Context, db *pgxpool.Pool, studentID string) {
	t.Helper()

	var count int
	if err := db.QueryRow(ctx, `
SELECT count(*)
FROM seat_bookings
WHERE student_id = $1`, studentID).Scan(&count); err != nil {
		t.Fatalf("count booking for student %s: %v", studentID, err)
	}
	if count != 1 {
		t.Fatalf("booking count for student %s = %d, want 1", studentID, count)
	}
}

func assertBookingCountForStudents(t *testing.T, ctx context.Context, db *pgxpool.Pool, studentIDs []string, want int) {
	t.Helper()

	var count int
	if err := db.QueryRow(ctx, `
SELECT count(*)
FROM seat_bookings
WHERE student_id = ANY($1)`, studentIDs).Scan(&count); err != nil {
		t.Fatalf("count bookings for students: %v", err)
	}
	if count != want {
		t.Fatalf("booking count for students %v = %d, want %d", studentIDs, count, want)
	}
}

func assertNoBookingForStudent(t *testing.T, ctx context.Context, db *pgxpool.Pool, studentID string) {
	t.Helper()

	var count int
	if err := db.QueryRow(ctx, `
SELECT count(*)
FROM seat_bookings
WHERE student_id = $1`, studentID).Scan(&count); err != nil {
		t.Fatalf("count booking for student %s: %v", studentID, err)
	}
	if count != 0 {
		t.Fatalf("booking count for student %s = %d, want 0", studentID, count)
	}
}

func bookSeatForStageTest(t *testing.T, ctx context.Context, db *pgxpool.Pool, store *PostgresSnapshotStore, studentID string, seatID int64) {
	t.Helper()

	setBookingStage(t, ctx, db, bookingStageOpen)
	bookings, err := store.BookSeats(ctx, studentID, []BookEntry{{SeatID: seatID, StudentID: studentID}})
	if err != nil {
		t.Fatalf("seed booking for %s: %v", studentID, err)
	}
	if len(bookings) != 1 {
		t.Fatalf("seed bookings for %s = %d, want 1", studentID, len(bookings))
	}
}

var testStudentIDSeq int64 = time.Now().UnixNano() % 1_000_000

func uniqueStudentID(prefix string, offset int) string {
	value := atomic.AddInt64(&testStudentIDSeq, int64(offset)+1) % 1_000_000
	return fmt.Sprintf("%s%06d", prefix, value)
}
