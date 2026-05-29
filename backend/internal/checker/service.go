package checker

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"lalalandshow/backend/internal/auth"
	"lalalandshow/backend/internal/student"
	"lalalandshow/backend/internal/ticket"
)

const (
	MethodQR             = "qr"
	MethodManual         = "manual"
	MethodForceNoBooking = "force_no_booking"
	ResultApproved       = "approved"
)

var (
	ErrInvalidCredentials = errors.New("invalid username or password")
	ErrInvalidRole        = errors.New("invalid staff role")
	ErrInvalidMethod      = errors.New("invalid check-in method")
	ErrStaleTicket        = errors.New("ticket qr does not match current booking")
	ErrMissingQRToken     = errors.New("qr_token is required")
)

type Store interface {
	FindStaffAccount(ctx context.Context, username string) (StaffAccount, error)
	GetBooking(ctx context.Context, studentID string) (Booking, error)
	InsertCheckIn(ctx context.Context, input CheckInInput) (CheckInEvent, error)
	Stats(ctx context.Context) (Stats, error)
	Roster(ctx context.Context) ([]RosterRow, error)
}

type TokenIssuer interface {
	GenerateStaffToken(username, role string, now time.Time) (string, error)
}

type Service struct {
	store  Store
	tokens TokenIssuer
	secret []byte
	now    func() time.Time
}

type StaffAccount struct {
	Username     string
	PasswordHash string
	Role         string
}

type LoginResult struct {
	Token string `json:"token"`
	Staff Staff  `json:"staff"`
}

type Staff struct {
	Username string `json:"username"`
	Role     string `json:"role"`
}

type Booking struct {
	StudentID       string     `json:"student_id"`
	ClassCode       *string    `json:"class_code,omitempty"`
	SeatNoInClass   *int       `json:"seat_no_in_class,omitempty"`
	DisplayName     *string    `json:"display_name,omitempty"`
	SeatID          int64      `json:"seat_id"`
	Zone            string     `json:"zone"`
	RowNo           int        `json:"row_no"`
	ColNo           int        `json:"col_no"`
	TicketVersion   int        `json:"ticket_version"`
	BookedAt        time.Time  `json:"booked_at"`
	LastCheckInAt   *time.Time `json:"last_checkin_at,omitempty"`
	CheckInCount    int        `json:"checkin_count"`
	CheckedInBefore bool       `json:"checked_in_before"`
}

type Verification struct {
	Valid     bool              `json:"valid"`
	Method    string            `json:"method"`
	QRPayload *ticket.QRPayload `json:"qr_payload,omitempty"`
	Booking   Booking           `json:"booking"`
}

type CheckInRequest struct {
	Method    string `json:"method"`
	QRToken   string `json:"qr_token,omitempty"`
	StudentID string `json:"student_id,omitempty"`
}

type CheckInInput struct {
	StudentID     string
	SeatID        *int64
	TicketVersion *int
	Method        string
	Result        string
	Scanner       string
	IPAddr        string
}

type CheckInEvent struct {
	ID            int64     `json:"id"`
	StudentID     string    `json:"student_id"`
	SeatID        *int64    `json:"seat_id,omitempty"`
	TicketVersion *int      `json:"ticket_version,omitempty"`
	Method        string    `json:"method"`
	Result        string    `json:"result"`
	IsFirstScan   bool      `json:"is_first_scan"`
	Scanner       string    `json:"scanner_account"`
	ScannedAt     time.Time `json:"scanned_at"`
}

type CheckInResult struct {
	Event   CheckInEvent `json:"event"`
	Booking *Booking     `json:"booking,omitempty"`
}

type Stats struct {
	CheckedIn      int `json:"checked_in"`
	Total          int `json:"total"`
	LastMinuteRate int `json:"last_minute_rate"`
}

type RosterRow struct {
	StudentID      string
	ClassCode      string
	SeatNoInClass  *int
	DisplayName    string
	SeatID         int64
	Zone           string
	RowNo          int
	ColNo          int
	TicketVersion  int
	CheckedIn      bool
	FirstScannedAt *time.Time
	LastScannedAt  *time.Time
	ScanCount      int
}

func NewService(store Store, tokens TokenIssuer, hmacSecret string) *Service {
	return &Service{
		store:  store,
		tokens: tokens,
		secret: []byte(hmacSecret),
		now:    time.Now,
	}
}

func (s *Service) Login(ctx context.Context, username, password string) (LoginResult, error) {
	username = strings.TrimSpace(username)
	password = strings.TrimSpace(password)
	if username == "" || password == "" {
		return LoginResult{}, ErrInvalidCredentials
	}

	account, err := s.store.FindStaffAccount(ctx, username)
	if err != nil {
		return LoginResult{}, err
	}
	if account.Role != "checker" && account.Role != "admin" {
		return LoginResult{}, ErrInvalidRole
	}
	if err := bcrypt.CompareHashAndPassword([]byte(account.PasswordHash), []byte(password)); err != nil {
		return LoginResult{}, ErrInvalidCredentials
	}

	token, err := s.tokens.GenerateStaffToken(account.Username, account.Role, s.now())
	if err != nil {
		return LoginResult{}, fmt.Errorf("generate staff token: %w", err)
	}
	return LoginResult{
		Token: token,
		Staff: Staff{
			Username: account.Username,
			Role:     account.Role,
		},
	}, nil
}

func (s *Service) VerifyQR(ctx context.Context, qrToken string) (Verification, error) {
	qrToken = strings.TrimSpace(qrToken)
	if qrToken == "" {
		return Verification{}, ErrMissingQRToken
	}
	payload, err := ticket.VerifyQR(qrToken, s.secret)
	if err != nil {
		return Verification{}, err
	}
	if _, err := time.Parse(time.RFC3339, payload.IssuedAt); err != nil {
		return Verification{}, ticket.ErrInvalidQR
	}

	booking, err := s.store.GetBooking(ctx, payload.StudentID)
	if err != nil {
		return Verification{}, err
	}
	if booking.SeatID != payload.SeatID || booking.TicketVersion != payload.TicketVersion {
		return Verification{}, ErrStaleTicket
	}

	return Verification{
		Valid:     true,
		Method:    MethodQR,
		QRPayload: &payload,
		Booking:   booking,
	}, nil
}

func (s *Service) Lookup(ctx context.Context, studentID string) (Verification, error) {
	studentID = strings.TrimSpace(studentID)
	if !student.IsValidStudentID(studentID) {
		return Verification{}, student.ErrInvalidStudentID
	}
	booking, err := s.store.GetBooking(ctx, studentID)
	if err != nil {
		return Verification{}, err
	}
	return Verification{
		Valid:   true,
		Method:  MethodManual,
		Booking: booking,
	}, nil
}

func (s *Service) CheckIn(ctx context.Context, req CheckInRequest, scanner auth.StaffIdentity, ipAddr string) (CheckInResult, error) {
	method := strings.TrimSpace(req.Method)
	switch method {
	case MethodQR:
		verification, err := s.VerifyQR(ctx, req.QRToken)
		if err != nil {
			return CheckInResult{}, err
		}
		version := verification.Booking.TicketVersion
		input := CheckInInput{
			StudentID:     verification.Booking.StudentID,
			SeatID:        &verification.Booking.SeatID,
			TicketVersion: &version,
			Method:        MethodQR,
			Result:        ResultApproved,
			Scanner:       scanner.Username,
			IPAddr:        ipAddr,
		}
		event, err := s.store.InsertCheckIn(ctx, input)
		if err != nil {
			return CheckInResult{}, err
		}
		return CheckInResult{Event: event, Booking: &verification.Booking}, nil
	case MethodManual:
		verification, err := s.Lookup(ctx, req.StudentID)
		if err != nil {
			return CheckInResult{}, err
		}
		version := verification.Booking.TicketVersion
		input := CheckInInput{
			StudentID:     verification.Booking.StudentID,
			SeatID:        &verification.Booking.SeatID,
			TicketVersion: &version,
			Method:        MethodManual,
			Result:        ResultApproved,
			Scanner:       scanner.Username,
			IPAddr:        ipAddr,
		}
		event, err := s.store.InsertCheckIn(ctx, input)
		if err != nil {
			return CheckInResult{}, err
		}
		return CheckInResult{Event: event, Booking: &verification.Booking}, nil
	case MethodForceNoBooking:
		studentID := strings.TrimSpace(req.StudentID)
		if !student.IsValidStudentID(studentID) {
			return CheckInResult{}, student.ErrInvalidStudentID
		}
		event, err := s.store.InsertCheckIn(ctx, CheckInInput{
			StudentID: studentID,
			Method:    MethodForceNoBooking,
			Result:    ResultApproved,
			Scanner:   scanner.Username,
			IPAddr:    ipAddr,
		})
		if err != nil {
			return CheckInResult{}, err
		}
		return CheckInResult{Event: event}, nil
	default:
		return CheckInResult{}, ErrInvalidMethod
	}
}

func (s *Service) Stats(ctx context.Context) (Stats, error) {
	return s.store.Stats(ctx)
}

func (s *Service) Roster(ctx context.Context) ([]RosterRow, error) {
	return s.store.Roster(ctx)
}
