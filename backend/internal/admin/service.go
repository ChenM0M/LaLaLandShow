package admin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"lalalandshow/backend/internal/student"
)

const (
	defaultPageSize = 50
	maxPageSize     = 100
)

var (
	ErrInvalidCredentials = errors.New("invalid username or password")
	ErrForbidden          = errors.New("admin role required")
	ErrInvalidSeatID      = errors.New("seat_id must be positive")
	ErrInvalidAssignment  = errors.New("assignment must contain either student_id, custom_label, or release=true")
	ErrAssignmentConflict = errors.New("seat or student already has an assignment")
	ErrSeatNotFound       = errors.New("seat not found")
	ErrConfigNotFound     = errors.New("config key not found")
	ErrInvalidConfigKey   = errors.New("invalid config key")
	ErrInvalidJSON        = errors.New("value_json must be valid JSON")
	ErrInvalidExportType  = errors.New("invalid export type")
)

var configKeyPattern = regexp.MustCompile(`^[a-z][a-z0-9_]{1,63}$`)

type Store interface {
	FindStaffAccount(ctx context.Context, username string) (StaffAccount, error)
	ListSeats(ctx context.Context) ([]SeatDetail, error)
	AssignSeat(ctx context.Context, input AssignSeatInput) (SeatDetail, error)
	LockSeat(ctx context.Context, input LockSeatInput) (SeatDetail, error)
	ListStudents(ctx context.Context, filter StudentFilter) (StudentList, error)
	ListAuditLogs(ctx context.Context, filter AuditFilter) (AuditLogList, error)
	ListCheckinEvents(ctx context.Context, filter CheckinFilter) (CheckinEventList, error)
	GetConfig(ctx context.Context, key string) (ConfigEntry, error)
	SetConfig(ctx context.Context, input ConfigInput) (ConfigEntry, error)
	PublicConfig(ctx context.Context) (map[string]json.RawMessage, error)
	WindowState(ctx context.Context, now time.Time) (WindowState, error)
	ExportCSV(ctx context.Context, exportType string) (CSVExport, error)
}

type TokenIssuer interface {
	GenerateStaffToken(username, role string, now time.Time) (string, error)
}

type Service struct {
	store  Store
	tokens TokenIssuer
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

type SeatDetail struct {
	SeatID      int64          `json:"seat_id"`
	Zone        string         `json:"zone"`
	RowNo       int            `json:"row_no"`
	ColNo       int            `json:"col_no"`
	IsBlocked   bool           `json:"is_blocked"`
	BlockReason *string        `json:"block_reason"`
	Booking     *BookingDetail `json:"booking"`
}

type BookingDetail struct {
	StudentID         *string         `json:"student_id"`
	CustomLabel       *string         `json:"custom_label"`
	BookedByStudentID *string         `json:"booked_by_student_id"`
	ClassCode         *string         `json:"class_code"`
	SeatNoInClass     *int            `json:"seat_no_in_class"`
	DisplayName       *string         `json:"display_name"`
	TicketVersion     int             `json:"ticket_version"`
	BookedAt          time.Time       `json:"booked_at"`
	PendingNotice     json.RawMessage `json:"pending_notice,omitempty"`
}

type AssignSeatInput struct {
	SeatID               int64
	StudentID            string
	CustomLabel          string
	Release              bool
	ForceReleaseExisting bool
	Actor                string
}

type LockSeatInput struct {
	SeatID      int64
	Locked      bool
	BlockReason *string
	Actor       string
}

type StudentFilter struct {
	Page      int
	PageSize  int
	ClassCode string
	Search    string
}

type StudentRow struct {
	StudentID     string          `json:"student_id"`
	ClassCode     *string         `json:"class_code"`
	SeatNoInClass *int            `json:"seat_no_in_class"`
	DisplayName   *string         `json:"display_name"`
	NeedRegister  bool            `json:"need_register"`
	PendingNotice json.RawMessage `json:"pending_notice,omitempty"`
	SeatID        *int64          `json:"seat_id"`
	BookedAt      *time.Time      `json:"booked_at"`
	TicketVersion *int            `json:"ticket_version"`
	ProxyBookedBy *string         `json:"booked_by_student_id"`
}

type StudentList struct {
	Students []StudentRow `json:"students"`
	Total    int          `json:"total"`
	Page     int          `json:"page"`
	PageSize int          `json:"page_size"`
}

type AuditFilter struct {
	Page     int
	PageSize int
	Action   string
	Actor    string
}

type AuditLogRow struct {
	ID            int64           `json:"id"`
	ActorType     string          `json:"actor_type"`
	ActorID       *string         `json:"actor_id"`
	Action        string          `json:"action"`
	TargetSeatID  *int64          `json:"target_seat_id"`
	TargetStudent *string         `json:"target_student"`
	Before        json.RawMessage `json:"before_json,omitempty"`
	After         json.RawMessage `json:"after_json,omitempty"`
	CreatedAt     time.Time       `json:"created_at"`
}

type AuditLogList struct {
	Logs     []AuditLogRow `json:"logs"`
	Total    int           `json:"total"`
	Page     int           `json:"page"`
	PageSize int           `json:"page_size"`
}

type CheckinFilter struct {
	Page      int
	PageSize  int
	StudentID string
	Method    string
}

type CheckinEventRow struct {
	ID             int64     `json:"id"`
	StudentID      *string   `json:"student_id"`
	SeatID         *int64    `json:"seat_id"`
	TicketVersion  *int      `json:"ticket_version"`
	Method         string    `json:"method"`
	Result         string    `json:"result"`
	IsFirstScan    bool      `json:"is_first_scan"`
	ScannerAccount *string   `json:"scanner_account"`
	ScannedAt      time.Time `json:"scanned_at"`
	ClassCode      *string   `json:"class_code,omitempty"`
	SeatNoInClass  *int      `json:"seat_no_in_class,omitempty"`
	DisplayName    *string   `json:"display_name,omitempty"`
}

type CheckinEventList struct {
	Events   []CheckinEventRow `json:"events"`
	Total    int               `json:"total"`
	Page     int               `json:"page"`
	PageSize int               `json:"page_size"`
}

type ConfigInput struct {
	Key       string
	ValueJSON json.RawMessage
	Actor     string
}

type ConfigEntry struct {
	Key       string          `json:"key"`
	ValueJSON json.RawMessage `json:"value_json"`
	UpdatedBy *string         `json:"updated_by"`
	UpdatedAt time.Time       `json:"updated_at"`
}

type WindowState struct {
	State string `json:"state"`
}

type CSVExport struct {
	Filename string
	Header   []string
	Rows     [][]string
}

func NewService(store Store, tokens TokenIssuer) *Service {
	return &Service{
		store:  store,
		tokens: tokens,
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
	if account.Role != "admin" {
		return LoginResult{}, ErrForbidden
	}
	if err := bcrypt.CompareHashAndPassword([]byte(account.PasswordHash), []byte(password)); err != nil {
		return LoginResult{}, ErrInvalidCredentials
	}

	token, err := s.tokens.GenerateStaffToken(account.Username, account.Role, s.now())
	if err != nil {
		return LoginResult{}, fmt.Errorf("generate admin token: %w", err)
	}
	return LoginResult{
		Token: token,
		Staff: Staff{Username: account.Username, Role: account.Role},
	}, nil
}

func (s *Service) ListSeats(ctx context.Context) ([]SeatDetail, error) {
	return s.store.ListSeats(ctx)
}

func (s *Service) AssignSeat(ctx context.Context, input AssignSeatInput) (SeatDetail, error) {
	input.Actor = strings.TrimSpace(input.Actor)
	input.StudentID = strings.TrimSpace(input.StudentID)
	input.CustomLabel = strings.TrimSpace(input.CustomLabel)

	if input.SeatID <= 0 {
		return SeatDetail{}, ErrInvalidSeatID
	}
	if input.Release {
		input.StudentID = ""
		input.CustomLabel = ""
		return s.store.AssignSeat(ctx, input)
	}
	if input.StudentID == "" && input.CustomLabel == "" {
		return SeatDetail{}, ErrInvalidAssignment
	}
	if input.StudentID != "" && input.CustomLabel != "" {
		return SeatDetail{}, ErrInvalidAssignment
	}
	if input.StudentID != "" && !student.IsValidStudentID(input.StudentID) {
		return SeatDetail{}, student.ErrInvalidStudentID
	}
	return s.store.AssignSeat(ctx, input)
}

func (s *Service) LockSeat(ctx context.Context, input LockSeatInput) (SeatDetail, error) {
	input.Actor = strings.TrimSpace(input.Actor)
	if input.SeatID <= 0 {
		return SeatDetail{}, ErrInvalidSeatID
	}
	if input.BlockReason != nil {
		reason := strings.TrimSpace(*input.BlockReason)
		if reason == "" {
			input.BlockReason = nil
		} else {
			input.BlockReason = &reason
		}
	}
	return s.store.LockSeat(ctx, input)
}

func (s *Service) ListStudents(ctx context.Context, filter StudentFilter) (StudentList, error) {
	filter.Page, filter.PageSize = normalizePage(filter.Page, filter.PageSize)
	filter.ClassCode = strings.TrimSpace(filter.ClassCode)
	filter.Search = strings.TrimSpace(filter.Search)
	return s.store.ListStudents(ctx, filter)
}

func (s *Service) ListAuditLogs(ctx context.Context, filter AuditFilter) (AuditLogList, error) {
	filter.Page, filter.PageSize = normalizePage(filter.Page, filter.PageSize)
	filter.Action = strings.TrimSpace(filter.Action)
	filter.Actor = strings.TrimSpace(filter.Actor)
	return s.store.ListAuditLogs(ctx, filter)
}

func (s *Service) ListCheckinEvents(ctx context.Context, filter CheckinFilter) (CheckinEventList, error) {
	filter.Page, filter.PageSize = normalizePage(filter.Page, filter.PageSize)
	filter.StudentID = strings.TrimSpace(filter.StudentID)
	filter.Method = strings.TrimSpace(filter.Method)
	if filter.StudentID != "" && !student.IsValidStudentID(filter.StudentID) {
		return CheckinEventList{}, student.ErrInvalidStudentID
	}
	return s.store.ListCheckinEvents(ctx, filter)
}

func (s *Service) GetConfig(ctx context.Context, key string) (ConfigEntry, error) {
	key = strings.TrimSpace(key)
	if !configKeyPattern.MatchString(key) {
		return ConfigEntry{}, ErrInvalidConfigKey
	}
	return s.store.GetConfig(ctx, key)
}

func (s *Service) SetConfig(ctx context.Context, input ConfigInput) (ConfigEntry, error) {
	input.Key = strings.TrimSpace(input.Key)
	input.Actor = strings.TrimSpace(input.Actor)
	if !configKeyPattern.MatchString(input.Key) {
		return ConfigEntry{}, ErrInvalidConfigKey
	}
	if !json.Valid(input.ValueJSON) {
		return ConfigEntry{}, ErrInvalidJSON
	}
	return s.store.SetConfig(ctx, input)
}

func (s *Service) PublicConfig(ctx context.Context) (map[string]json.RawMessage, error) {
	return s.store.PublicConfig(ctx)
}

func (s *Service) WindowState(ctx context.Context) (WindowState, error) {
	return s.store.WindowState(ctx, s.now())
}

func (s *Service) ExportCSV(ctx context.Context, exportType string) (CSVExport, error) {
	exportType = strings.TrimSpace(exportType)
	switch exportType {
	case "students", "bookings", "audit", "checkin":
		return s.store.ExportCSV(ctx, exportType)
	default:
		return CSVExport{}, ErrInvalidExportType
	}
}

func normalizePage(page, pageSize int) (int, int) {
	if page < 1 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = defaultPageSize
	}
	if pageSize > maxPageSize {
		pageSize = maxPageSize
	}
	return page, pageSize
}
