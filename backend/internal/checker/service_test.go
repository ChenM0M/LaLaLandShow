package checker

import (
	"context"
	"errors"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"lalalandshow/backend/internal/auth"
	"lalalandshow/backend/internal/ticket"
)

type fakeStore struct {
	account      StaffAccount
	accountErr   error
	booking      Booking
	bookingErr   error
	inserted     []CheckInInput
	seenCheckIns map[string]int
	stats        Stats
	roster       []RosterRow
}

func (s *fakeStore) FindStaffAccount(_ context.Context, _ string) (StaffAccount, error) {
	if s.accountErr != nil {
		return StaffAccount{}, s.accountErr
	}
	return s.account, nil
}

func (s *fakeStore) GetBooking(_ context.Context, studentID string) (Booking, error) {
	if s.bookingErr != nil {
		return Booking{}, s.bookingErr
	}
	booking := s.booking
	booking.StudentID = studentID
	return booking, nil
}

func (s *fakeStore) InsertCheckIn(_ context.Context, input CheckInInput) (CheckInEvent, error) {
	s.inserted = append(s.inserted, input)
	if s.seenCheckIns == nil {
		s.seenCheckIns = map[string]int{}
	}
	seen := s.seenCheckIns[input.StudentID]
	s.seenCheckIns[input.StudentID] = seen + 1
	return CheckInEvent{
		ID:            int64(len(s.inserted)),
		StudentID:     input.StudentID,
		SeatID:        input.SeatID,
		TicketVersion: input.TicketVersion,
		Method:        input.Method,
		Result:        input.Result,
		IsFirstScan:   seen == 0,
		Scanner:       input.Scanner,
		ScannedAt:     time.Unix(int64(len(s.inserted)), 0).UTC(),
	}, nil
}

func (s *fakeStore) Stats(_ context.Context) (Stats, error) {
	return s.stats, nil
}

func (s *fakeStore) Roster(_ context.Context) ([]RosterRow, error) {
	return s.roster, nil
}

func TestServiceLoginReturnsStaffToken(t *testing.T) {
	hash, err := bcrypt.GenerateFromPassword([]byte("secret"), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("GenerateFromPassword() error = %v", err)
	}
	store := &fakeStore{
		account: StaffAccount{
			Username:     "checker",
			PasswordHash: string(hash),
			Role:         "checker",
		},
	}
	manager := auth.NewManager("jwt-secret", time.Hour)
	service := NewService(store, manager, "hmac-secret")
	service.now = func() time.Time { return time.Unix(1000, 0).UTC() }

	result, err := service.Login(context.Background(), "checker", "secret")
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	claims, err := manager.ParseStaffToken(result.Token, time.Unix(1001, 0).UTC(), "checker", "admin")
	if err != nil {
		t.Fatalf("ParseStaffToken() error = %v", err)
	}
	if claims.Subject != "checker" || claims.Role != "checker" {
		t.Fatalf("claims = %#v, want checker role", claims)
	}
}

func TestServiceVerifyQRRejectsTampering(t *testing.T) {
	service := NewService(&fakeStore{}, auth.NewManager("jwt-secret", time.Hour), "hmac-secret")

	if _, err := service.VerifyQR(context.Background(), "bad.token"); !errors.Is(err, ticket.ErrInvalidQR) {
		t.Fatalf("VerifyQR(tampered) error = %v, want %v", err, ticket.ErrInvalidQR)
	}
}

func TestServiceVerifyQRRejectsStaleTicket(t *testing.T) {
	payload := ticket.QRPayload{
		StudentID:     "1234567",
		SeatID:        42,
		TicketVersion: 1,
		IssuedAt:      time.Unix(1000, 0).UTC().Format(time.RFC3339),
	}
	token, err := ticket.SignQR(payload, []byte("hmac-secret"))
	if err != nil {
		t.Fatalf("SignQR() error = %v", err)
	}
	store := &fakeStore{
		booking: Booking{
			SeatID:        42,
			Zone:          "middle",
			RowNo:         1,
			ColNo:         1,
			TicketVersion: 2,
		},
	}
	service := NewService(store, auth.NewManager("jwt-secret", time.Hour), "hmac-secret")

	if _, err := service.VerifyQR(context.Background(), token); !errors.Is(err, ErrStaleTicket) {
		t.Fatalf("VerifyQR(stale) error = %v, want %v", err, ErrStaleTicket)
	}
}

func TestServiceCheckInQRMarksRepeatedScan(t *testing.T) {
	payload := ticket.QRPayload{
		StudentID:     "1234567",
		SeatID:        42,
		TicketVersion: 3,
		IssuedAt:      time.Unix(1000, 0).UTC().Format(time.RFC3339),
	}
	token, err := ticket.SignQR(payload, []byte("hmac-secret"))
	if err != nil {
		t.Fatalf("SignQR() error = %v", err)
	}
	store := &fakeStore{
		booking: Booking{
			SeatID:        42,
			Zone:          "middle",
			RowNo:         1,
			ColNo:         1,
			TicketVersion: 3,
		},
	}
	service := NewService(store, auth.NewManager("jwt-secret", time.Hour), "hmac-secret")
	scanner := auth.StaffIdentity{Username: "checker", Role: "checker"}

	first, err := service.CheckIn(context.Background(), CheckInRequest{Method: MethodQR, QRToken: token}, scanner, "")
	if err != nil {
		t.Fatalf("first CheckIn() error = %v", err)
	}
	second, err := service.CheckIn(context.Background(), CheckInRequest{Method: MethodQR, QRToken: token}, scanner, "")
	if err != nil {
		t.Fatalf("second CheckIn() error = %v", err)
	}
	if !first.Event.IsFirstScan {
		t.Fatalf("first scan IsFirstScan = false, want true")
	}
	if second.Event.IsFirstScan {
		t.Fatalf("second scan IsFirstScan = true, want false")
	}
	if len(store.inserted) != 2 || store.inserted[0].Method != MethodQR {
		t.Fatalf("inserted = %#v, want qr check-ins", store.inserted)
	}
}

func TestServiceCheckInForceNoBooking(t *testing.T) {
	store := &fakeStore{}
	service := NewService(store, auth.NewManager("jwt-secret", time.Hour), "hmac-secret")
	scanner := auth.StaffIdentity{Username: "admin", Role: "admin"}

	result, err := service.CheckIn(context.Background(), CheckInRequest{
		Method:    MethodForceNoBooking,
		StudentID: "7654321",
	}, scanner, "")
	if err != nil {
		t.Fatalf("CheckIn(force) error = %v", err)
	}
	if result.Event.Method != MethodForceNoBooking || result.Event.SeatID != nil {
		t.Fatalf("event = %#v, want force no-booking event without seat", result.Event)
	}
}
