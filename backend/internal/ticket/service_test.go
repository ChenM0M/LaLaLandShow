package ticket

import (
	"context"
	"errors"
	"testing"
	"time"

	"lalalandshow/backend/internal/student"
)

type fakeStore struct {
	record      Record
	records     []Record
	recordErr   error
	times       map[string]time.Time
	strings     map[string]string
	timeErr     error
	stringErr   error
	seenStudent string
}

func (s *fakeStore) GetTicketRecord(_ context.Context, studentID string) (Record, error) {
	s.seenStudent = studentID
	if s.recordErr != nil {
		return Record{}, s.recordErr
	}
	return s.record, nil
}

func (s *fakeStore) ListTicketRecords(_ context.Context, studentID string) ([]Record, error) {
	s.seenStudent = studentID
	if s.recordErr != nil {
		return nil, s.recordErr
	}
	if s.records != nil {
		return s.records, nil
	}
	if s.record.StudentID == "" {
		return nil, nil
	}
	return []Record{s.record}, nil
}

func (s *fakeStore) GetConfigTime(_ context.Context, key string) (time.Time, bool, error) {
	if s.timeErr != nil {
		return time.Time{}, false, s.timeErr
	}
	value, ok := s.times[key]
	return value, ok, nil
}

func (s *fakeStore) GetConfigString(_ context.Context, key string) (string, bool, error) {
	if s.stringErr != nil {
		return "", false, s.stringErr
	}
	value, ok := s.strings[key]
	return value, ok, nil
}

func TestServiceGetReturnsSignedTicket(t *testing.T) {
	now := time.Unix(2000, 0).UTC()
	classCode := "1201"
	seatNo := 18
	store := &fakeStore{
		record: Record{
			StudentID:     "1234567",
			ClassCode:     &classCode,
			SeatNoInClass: &seatNo,
			SeatID:        42,
			Zone:          "middle",
			RowNo:         4,
			ColNo:         9,
			TicketVersion: 2,
		},
		times: map[string]time.Time{
			"booking_lock_at": now.Add(-time.Hour),
			"show_start_at":   now.Add(time.Hour),
		},
		strings: map[string]string{
			"play_name": "La La Land",
			"venue":     "Auditorium",
		},
	}
	service := NewService(store, "secret")
	service.now = func() time.Time { return now }

	ticket, err := service.Get(context.Background(), " 1234567 ")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if store.seenStudent != "1234567" {
		t.Fatalf("student = %q, want normalized id", store.seenStudent)
	}
	if ticket.QRToken == "" {
		t.Fatal("QRToken is empty")
	}
	payload, err := VerifyQR(ticket.QRToken, []byte("secret"))
	if err != nil {
		t.Fatalf("VerifyQR() error = %v", err)
	}
	if payload.StudentID != "1234567" || payload.SeatID != 42 || payload.TicketVersion != 2 {
		t.Fatalf("payload = %#v, want student/seat/version", payload)
	}
}

func TestServiceGetBlocksBeforeLock(t *testing.T) {
	now := time.Unix(2000, 0).UTC()
	store := &fakeStore{
		times: map[string]time.Time{
			"booking_lock_at": now.Add(time.Hour),
		},
	}
	service := NewService(store, "secret")
	service.now = func() time.Time { return now }

	if _, err := service.Get(context.Background(), "1234567"); !errors.Is(err, ErrTicketNotAvailable) {
		t.Fatalf("Get() error = %v, want %v", err, ErrTicketNotAvailable)
	}
}

func TestServiceGetRejectsIncompleteHolderProfile(t *testing.T) {
	now := time.Unix(2000, 0).UTC()
	store := &fakeStore{
		record: Record{
			StudentID:     "1234567",
			SeatID:        42,
			Zone:          "middle",
			RowNo:         4,
			ColNo:         9,
			TicketVersion: 2,
		},
		times: map[string]time.Time{
			"booking_lock_at": now.Add(-time.Hour),
		},
	}
	service := NewService(store, "secret")
	service.now = func() time.Time { return now }

	if _, err := service.Get(context.Background(), "1234567"); !errors.Is(err, student.ErrIncompleteProfile) {
		t.Fatalf("Get(incomplete profile) error = %v, want %v", err, student.ErrIncompleteProfile)
	}
}

func TestServiceListDoesNotExposeProxyTicketQR(t *testing.T) {
	now := time.Unix(2000, 0).UTC()
	classCode := "1201"
	seatNo := 18
	booker := "1234567"
	store := &fakeStore{
		records: []Record{
			{
				StudentID:     "1234567",
				ClassCode:     &classCode,
				SeatNoInClass: &seatNo,
				SeatID:        42,
				Zone:          "middle",
				RowNo:         4,
				ColNo:         9,
				TicketVersion: 2,
			},
			{
				StudentID:      "7654321",
				ClassCode:      &classCode,
				SeatNoInClass:  &seatNo,
				SeatID:         43,
				Zone:           "left",
				RowNo:          5,
				ColNo:          10,
				TicketVersion:  3,
				BookingOwnerID: &booker,
			},
		},
		times: map[string]time.Time{
			"booking_lock_at": now.Add(-time.Hour),
		},
	}
	service := NewService(store, "secret")
	service.now = func() time.Time { return now }

	result, err := service.List(context.Background(), "1234567")
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(result.Tickets) != 2 {
		t.Fatalf("tickets length = %d, want 2", len(result.Tickets))
	}
	own := result.Tickets[0]
	proxy := result.Tickets[1]
	if !own.OwnedByViewer || own.QRToken == "" || own.QRPayload == nil {
		t.Fatalf("own ticket QR fields = token %q payload %#v owned %v, want visible QR", own.QRToken, own.QRPayload, own.OwnedByViewer)
	}
	if proxy.OwnedByViewer || proxy.QRToken != "" || proxy.QRPayload != nil {
		t.Fatalf("proxy ticket QR fields = token %q payload %#v owned %v, want no QR exposure", proxy.QRToken, proxy.QRPayload, proxy.OwnedByViewer)
	}
	if proxy.Holder.StudentID != "7654321" || proxy.Seat.SeatID != 43 {
		t.Fatalf("proxy ticket = %#v, want seat info only for proxied student", proxy)
	}
}

func TestServiceListAllowsIncompleteProxyTicketWithoutQR(t *testing.T) {
	now := time.Unix(2000, 0).UTC()
	classCode := "1201"
	seatNo := 18
	booker := "1234567"
	store := &fakeStore{
		records: []Record{
			{
				StudentID:     "1234567",
				ClassCode:     &classCode,
				SeatNoInClass: &seatNo,
				SeatID:        42,
				Zone:          "middle",
				RowNo:         4,
				ColNo:         9,
				TicketVersion: 2,
			},
			{
				StudentID:      "7654321",
				SeatID:         43,
				Zone:           "left",
				RowNo:          5,
				ColNo:          10,
				TicketVersion:  3,
				BookingOwnerID: &booker,
			},
		},
		times: map[string]time.Time{
			"booking_lock_at": now.Add(-time.Hour),
		},
	}
	service := NewService(store, "secret")
	service.now = func() time.Time { return now }

	result, err := service.List(context.Background(), "1234567")
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	proxy := result.Tickets[1]
	if proxy.QRToken != "" || proxy.QRPayload != nil || proxy.OwnedByViewer {
		t.Fatalf("proxy ticket QR fields = token %q payload %#v owned %v, want no QR exposure", proxy.QRToken, proxy.QRPayload, proxy.OwnedByViewer)
	}
	if proxy.Holder.StudentID != "7654321" || proxy.Holder.ClassCode != nil || proxy.Holder.SeatNoInClass != nil {
		t.Fatalf("proxy holder = %#v, want incomplete holder seat info only", proxy.Holder)
	}
}

func TestServiceImageReturnsPNG(t *testing.T) {
	now := time.Unix(2000, 0).UTC()
	classCode := "1201"
	seatNo := 18
	store := &fakeStore{
		record: Record{
			StudentID:     "1234567",
			ClassCode:     &classCode,
			SeatNoInClass: &seatNo,
			SeatID:        42,
			Zone:          "middle",
			RowNo:         4,
			ColNo:         9,
			TicketVersion: 2,
		},
		times: map[string]time.Time{
			"booking_lock_at": now.Add(-time.Hour),
		},
	}
	service := NewService(store, "secret")
	service.now = func() time.Time { return now }

	pngBytes, err := service.Image(context.Background(), "1234567")
	if err != nil {
		t.Fatalf("Image() error = %v", err)
	}
	if len(pngBytes) < 8 || string(pngBytes[:8]) != "\x89PNG\r\n\x1a\n" {
		t.Fatalf("image is not PNG, first bytes = %q", pngBytes[:8])
	}
}
