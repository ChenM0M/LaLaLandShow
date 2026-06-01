package seat

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"lalalandshow/backend/internal/student"
)

var (
	ErrInvalidBookingBatch     = errors.New("booking request must contain 1 to 3 seats")
	ErrInvalidBookingStudentID = errors.New("student_id must be exactly seven digits")
	ErrInvalidSeatID           = errors.New("seat_id must be positive")
	ErrDuplicateSeat           = errors.New("booking request contains duplicate seat_id")
	ErrDuplicateStudent        = errors.New("booking request contains duplicate student_id")
	ErrMissingSelfBooking      = errors.New("booking request must include the authenticated student")
	ErrBookingConflict         = errors.New("one or more seats or students are already booked")
	ErrBookingNotOpen          = errors.New("seat booking is not open yet")
	ErrBookingLocked           = errors.New("seat booking is locked")
	ErrSeatLocked              = errors.New("one or more seats are locked")
	ErrSeatNotFound            = errors.New("one or more seats were not found")
	ErrBookingNotFound         = errors.New("student has no booking")
	ErrSameSeatChange          = errors.New("new seat must be different from current seat")
	ErrChangeLocked            = errors.New("seat changes are locked")
	ErrBookingFrozen           = errors.New("seat operations are frozen")
	ErrIncompleteRegistration  = errors.New("student registration is incomplete")
)

type Store interface {
	SnapshotStore
	BookSeats(ctx context.Context, actorStudentID string, entries []BookEntry) ([]BookedSeat, error)
	RebookSeats(ctx context.Context, actorStudentID string, entries []BookEntry) (RebookResult, error)
	ReleaseSeats(ctx context.Context, actorStudentID string) ([]BookedSeat, error)
	ChangeSeat(ctx context.Context, actorStudentID string, newSeatID int64) (SeatChange, error)
}

type Service struct {
	store Store
}

func NewService(store Store) *Service {
	return &Service{store: store}
}

type BookEntry struct {
	SeatID    int64  `json:"seat_id"`
	StudentID string `json:"student_id"`
}

type BookedSeat struct {
	SeatID        int64     `json:"seat_id"`
	StudentID     string    `json:"student_id"`
	Zone          string    `json:"zone"`
	RowNo         int       `json:"row_no"`
	ColNo         int       `json:"col_no"`
	TicketVersion int       `json:"ticket_version"`
	BookedAt      time.Time `json:"booked_at"`
}

type BookResult struct {
	Bookings []BookedSeat `json:"bookings"`
}

type ReleaseResult struct {
	Released         BookedSeat   `json:"released"`
	ReleasedBookings []BookedSeat `json:"released_bookings,omitempty"`
}

type RebookResult struct {
	ReleasedBookings []BookedSeat `json:"released_bookings"`
	Bookings         []BookedSeat `json:"bookings"`
}

type SeatChange struct {
	Previous BookedSeat `json:"previous"`
	Booking  BookedSeat `json:"booking"`
}

func (s *Service) Book(ctx context.Context, actorStudentID string, entries []BookEntry) (BookResult, error) {
	normalized, err := normalizeBookEntries(actorStudentID, entries)
	if err != nil {
		return BookResult{}, err
	}

	bookings, err := s.store.BookSeats(ctx, actorStudentID, normalized)
	if err != nil {
		return BookResult{}, fmt.Errorf("book seats: %w", err)
	}
	return BookResult{Bookings: bookings}, nil
}

func (s *Service) Rebook(ctx context.Context, actorStudentID string, entries []BookEntry) (RebookResult, error) {
	normalized, err := normalizeBookEntries(actorStudentID, entries)
	if err != nil {
		return RebookResult{}, err
	}

	result, err := s.store.RebookSeats(ctx, actorStudentID, normalized)
	if err != nil {
		return RebookResult{}, fmt.Errorf("rebook seats: %w", err)
	}
	return result, nil
}

func (s *Service) Release(ctx context.Context, actorStudentID string) (ReleaseResult, error) {
	actorStudentID = strings.TrimSpace(actorStudentID)
	if !student.IsValidStudentID(actorStudentID) {
		return ReleaseResult{}, ErrInvalidBookingStudentID
	}

	released, err := s.store.ReleaseSeats(ctx, actorStudentID)
	if err != nil {
		return ReleaseResult{}, fmt.Errorf("release seat: %w", err)
	}
	if len(released) == 0 {
		return ReleaseResult{}, ErrBookingNotFound
	}
	return ReleaseResult{Released: released[0], ReleasedBookings: released}, nil
}

func (s *Service) Change(ctx context.Context, actorStudentID string, newSeatID int64) (SeatChange, error) {
	actorStudentID = strings.TrimSpace(actorStudentID)
	if !student.IsValidStudentID(actorStudentID) {
		return SeatChange{}, ErrInvalidBookingStudentID
	}
	if newSeatID <= 0 {
		return SeatChange{}, ErrInvalidSeatID
	}

	change, err := s.store.ChangeSeat(ctx, actorStudentID, newSeatID)
	if err != nil {
		return SeatChange{}, fmt.Errorf("change seat: %w", err)
	}
	return change, nil
}

func normalizeBookEntries(actorStudentID string, entries []BookEntry) ([]BookEntry, error) {
	actorStudentID = strings.TrimSpace(actorStudentID)
	if !student.IsValidStudentID(actorStudentID) {
		return nil, ErrInvalidBookingStudentID
	}
	if len(entries) < 1 || len(entries) > 3 {
		return nil, ErrInvalidBookingBatch
	}

	seenSeats := map[int64]struct{}{}
	seenStudents := map[string]struct{}{}
	hasSelf := false
	normalized := make([]BookEntry, 0, len(entries))
	for _, entry := range entries {
		entry.StudentID = strings.TrimSpace(entry.StudentID)
		if entry.SeatID <= 0 {
			return nil, ErrInvalidSeatID
		}
		if !student.IsValidStudentID(entry.StudentID) {
			return nil, ErrInvalidBookingStudentID
		}
		if _, ok := seenSeats[entry.SeatID]; ok {
			return nil, ErrDuplicateSeat
		}
		if _, ok := seenStudents[entry.StudentID]; ok {
			return nil, ErrDuplicateStudent
		}
		if entry.StudentID == actorStudentID {
			hasSelf = true
		}

		seenSeats[entry.SeatID] = struct{}{}
		seenStudents[entry.StudentID] = struct{}{}
		normalized = append(normalized, entry)
	}
	if !hasSelf {
		return nil, ErrMissingSelfBooking
	}
	return normalized, nil
}
