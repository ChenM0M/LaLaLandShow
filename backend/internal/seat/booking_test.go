package seat

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeBookingStore struct {
	bookings       []BookedSeat
	released       []BookedSeat
	rebook         RebookResult
	change         SeatChange
	err            error
	rebookErr      error
	releaseErr     error
	changeErr      error
	seen           []BookEntry
	seenRebook     []BookEntry
	seenReleaseFor string
	seenChangeFor  string
	seenNewSeatID  int64
}

func (s *fakeBookingStore) GetSnapshot(context.Context, string) (Snapshot, error) {
	return Snapshot{}, nil
}

func (s *fakeBookingStore) BookSeats(_ context.Context, _ string, entries []BookEntry) ([]BookedSeat, error) {
	s.seen = append([]BookEntry(nil), entries...)
	if s.err != nil {
		return nil, s.err
	}
	return s.bookings, nil
}

func (s *fakeBookingStore) RebookSeats(_ context.Context, _ string, entries []BookEntry) (RebookResult, error) {
	s.seenRebook = append([]BookEntry(nil), entries...)
	if s.rebookErr != nil {
		return RebookResult{}, s.rebookErr
	}
	return s.rebook, nil
}

func (s *fakeBookingStore) ReleaseSeats(_ context.Context, actorStudentID string) ([]BookedSeat, error) {
	s.seenReleaseFor = actorStudentID
	if s.releaseErr != nil {
		return nil, s.releaseErr
	}
	return s.released, nil
}

func (s *fakeBookingStore) ChangeSeat(_ context.Context, actorStudentID string, newSeatID int64) (SeatChange, error) {
	s.seenChangeFor = actorStudentID
	s.seenNewSeatID = newSeatID
	if s.changeErr != nil {
		return SeatChange{}, s.changeErr
	}
	return s.change, nil
}

func TestServiceBookValidatesBatch(t *testing.T) {
	service := NewService(&fakeBookingStore{})

	tests := []struct {
		name    string
		actor   string
		entries []BookEntry
		wantErr error
	}{
		{name: "empty batch", actor: "1121001", wantErr: ErrInvalidBookingBatch},
		{
			name:    "too many seats",
			actor:   "1121001",
			entries: []BookEntry{{SeatID: 1, StudentID: "1121001"}, {SeatID: 2, StudentID: "1121002"}, {SeatID: 3, StudentID: "1121003"}, {SeatID: 4, StudentID: "1121004"}},
			wantErr: ErrInvalidBookingBatch,
		},
		{
			name:    "invalid companion student id",
			actor:   "1121001",
			entries: []BookEntry{{SeatID: 1, StudentID: "1121001"}, {SeatID: 2, StudentID: "abc"}},
			wantErr: ErrInvalidBookingStudentID,
		},
		{
			name:    "invalid seat id",
			actor:   "1121001",
			entries: []BookEntry{{SeatID: 0, StudentID: "1121001"}},
			wantErr: ErrInvalidSeatID,
		},
		{
			name:    "duplicate seat",
			actor:   "1121001",
			entries: []BookEntry{{SeatID: 1, StudentID: "1121001"}, {SeatID: 1, StudentID: "1121002"}},
			wantErr: ErrDuplicateSeat,
		},
		{
			name:    "duplicate student",
			actor:   "1121001",
			entries: []BookEntry{{SeatID: 1, StudentID: "1121001"}, {SeatID: 2, StudentID: "1121001"}},
			wantErr: ErrDuplicateStudent,
		},
		{
			name:    "missing self",
			actor:   "1121001",
			entries: []BookEntry{{SeatID: 1, StudentID: "1121002"}},
			wantErr: ErrMissingSelfBooking,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := service.Book(context.Background(), tt.actor, tt.entries); !errors.Is(err, tt.wantErr) {
				t.Fatalf("Book() error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestServiceBookStoresNormalizedEntries(t *testing.T) {
	store := &fakeBookingStore{
		bookings: []BookedSeat{{
			SeatID:        7,
			StudentID:     "1121001",
			Zone:          "middle",
			RowNo:         1,
			ColNo:         1,
			TicketVersion: 1,
			BookedAt:      time.Now(),
		}},
	}
	service := NewService(store)

	result, err := service.Book(context.Background(), "1121001", []BookEntry{{
		SeatID:    7,
		StudentID: " 1121001 ",
	}})
	if err != nil {
		t.Fatalf("Book() error = %v", err)
	}
	if len(result.Bookings) != 1 {
		t.Fatalf("len(Bookings) = %d, want 1", len(result.Bookings))
	}
	if got := store.seen[0].StudentID; got != "1121001" {
		t.Fatalf("stored student id = %q, want normalized id", got)
	}
}

func TestServiceReleaseValidatesStudentID(t *testing.T) {
	service := NewService(&fakeBookingStore{})

	if _, err := service.Release(context.Background(), "abc"); !errors.Is(err, ErrInvalidBookingStudentID) {
		t.Fatalf("Release() error = %v, want %v", err, ErrInvalidBookingStudentID)
	}
}

func TestServiceReleaseStoresNormalizedStudentID(t *testing.T) {
	store := &fakeBookingStore{
		released: []BookedSeat{
			{
				SeatID:        7,
				StudentID:     "1121001",
				Zone:          "middle",
				RowNo:         1,
				ColNo:         1,
				TicketVersion: 1,
				BookedAt:      time.Now(),
			},
		},
	}
	service := NewService(store)

	result, err := service.Release(context.Background(), " 1121001 ")
	if err != nil {
		t.Fatalf("Release() error = %v", err)
	}
	if result.Released.SeatID != 7 {
		t.Fatalf("released seat = %d, want 7", result.Released.SeatID)
	}
	if len(result.ReleasedBookings) != 1 || result.ReleasedBookings[0].SeatID != 7 {
		t.Fatalf("released bookings = %#v, want own booking only", result.ReleasedBookings)
	}
	if store.seenReleaseFor != "1121001" {
		t.Fatalf("release student id = %q, want normalized id", store.seenReleaseFor)
	}
}

func TestServiceReleaseRejectsEmptyStoreResult(t *testing.T) {
	service := NewService(&fakeBookingStore{})

	if _, err := service.Release(context.Background(), "1121001"); !errors.Is(err, ErrBookingNotFound) {
		t.Fatalf("Release(empty) error = %v, want %v", err, ErrBookingNotFound)
	}
}

func TestServiceChangeValidatesInput(t *testing.T) {
	service := NewService(&fakeBookingStore{})

	tests := []struct {
		name    string
		actor   string
		seatID  int64
		wantErr error
	}{
		{name: "invalid student", actor: "abc", seatID: 1, wantErr: ErrInvalidBookingStudentID},
		{name: "invalid seat", actor: "1121001", seatID: 0, wantErr: ErrInvalidSeatID},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := service.Change(context.Background(), tt.actor, tt.seatID); !errors.Is(err, tt.wantErr) {
				t.Fatalf("Change() error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestServiceChangeStoresNormalizedInput(t *testing.T) {
	store := &fakeBookingStore{
		change: SeatChange{
			Previous: BookedSeat{SeatID: 7, StudentID: "1121001", TicketVersion: 1},
			Booking:  BookedSeat{SeatID: 8, StudentID: "1121001", TicketVersion: 2},
		},
	}
	service := NewService(store)

	result, err := service.Change(context.Background(), " 1121001 ", 8)
	if err != nil {
		t.Fatalf("Change() error = %v", err)
	}
	if result.Booking.SeatID != 8 {
		t.Fatalf("changed seat = %d, want 8", result.Booking.SeatID)
	}
	if store.seenChangeFor != "1121001" {
		t.Fatalf("change student id = %q, want normalized id", store.seenChangeFor)
	}
	if store.seenNewSeatID != 8 {
		t.Fatalf("new seat id = %d, want 8", store.seenNewSeatID)
	}
}
