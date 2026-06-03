package admin

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
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
	ErrInvalidCredentials  = errors.New("invalid username or password")
	ErrForbidden           = errors.New("admin role required")
	ErrInvalidSeatID       = errors.New("seat_id must be positive")
	ErrInvalidAssignment   = errors.New("assignment must contain either student_id, custom_label, or release=true")
	ErrAssignmentConflict  = errors.New("seat or student already has an assignment")
	ErrSeatNotFound        = errors.New("seat not found")
	ErrConfigNotFound      = errors.New("config key not found")
	ErrInvalidConfigKey    = errors.New("invalid config key")
	ErrInvalidJSON         = errors.New("value_json must be valid JSON")
	ErrInvalidConfigValue  = errors.New("invalid config value")
	ErrInvalidExportType   = errors.New("invalid export type")
	ErrInvalidBackup       = errors.New("invalid backup snapshot")
	ErrInvalidRestoreToken = errors.New("invalid restore confirmation token")
)

var configKeyPattern = regexp.MustCompile(`^[a-z][a-z0-9_]{1,63}$`)

type Store interface {
	FindStaffAccount(ctx context.Context, username string) (StaffAccount, error)
	ListSeats(ctx context.Context) ([]SeatDetail, error)
	AssignSeat(ctx context.Context, input AssignSeatInput) (SeatDetail, error)
	LockSeat(ctx context.Context, input LockSeatInput) (SeatDetail, error)
	ListStudents(ctx context.Context, filter StudentFilter) (StudentList, error)
	ListStudentSeatConflicts(ctx context.Context, filter StudentFilter) ([]StudentSeatConflict, error)
	ListAuditLogs(ctx context.Context, filter AuditFilter) (AuditLogList, error)
	ListCheckinEvents(ctx context.Context, filter CheckinFilter) (CheckinEventList, error)
	DashboardStats(ctx context.Context, now time.Time) (DashboardStats, error)
	GetConfig(ctx context.Context, key string) (ConfigEntry, error)
	SetConfig(ctx context.Context, input ConfigInput) (ConfigEntry, error)
	PublicConfig(ctx context.Context) (map[string]json.RawMessage, error)
	WindowState(ctx context.Context, now time.Time) (WindowState, error)
	ExportCSV(ctx context.Context, exportType string) (CSVExport, error)
	ExportBackup(ctx context.Context, now time.Time) (BackupSnapshot, error)
	RestoreBackup(ctx context.Context, input RestoreBackupInput) (RestoreResult, error)
}

type TokenIssuer interface {
	GenerateStaffToken(username, role string, now time.Time) (string, error)
}

type Service struct {
	store         Store
	tokens        TokenIssuer
	restoreSecret string
	now           func() time.Time
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
	Page        int
	PageSize    int
	GradePrefix string
	ClassCode   string
	Search      string
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

type StudentSeatConflict struct {
	ClassCode     string   `json:"class_code"`
	SeatNoInClass int      `json:"seat_no_in_class"`
	StudentIDs    []string `json:"student_ids"`
	Count         int      `json:"count"`
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

type DashboardStats struct {
	GeneratedAt time.Time          `json:"generated_at"`
	Window      WindowState        `json:"window"`
	Seats       DashboardSeatStats `json:"seats"`
	Students    DashboardStudents  `json:"students"`
	Checkins    DashboardCheckins  `json:"checkins"`
	Recent      DashboardRecent    `json:"recent"`
}

type DashboardSeatStats struct {
	Total          int `json:"total"`
	Booked         int `json:"booked"`
	Free           int `json:"free"`
	Blocked        int `json:"blocked"`
	StudentBooked  int `json:"student_booked"`
	CustomOccupied int `json:"custom_occupied"`
}

type DashboardStudents struct {
	Total         int `json:"total"`
	Registered    int `json:"registered"`
	NeedRegister  int `json:"need_register"`
	PendingNotice int `json:"pending_notice"`
}

type DashboardCheckins struct {
	Approved       int `json:"approved"`
	Rejected       int `json:"rejected"`
	UniqueStudents int `json:"unique_students"`
	FirstScans     int `json:"first_scans"`
	LastMinuteRate int `json:"last_minute_rate"`
}

type DashboardRecent struct {
	AuditLogs     []AuditLogRow     `json:"audit_logs"`
	CheckinEvents []CheckinEventRow `json:"checkin_events"`
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

type BackupSnapshot struct {
	FormatVersion int                  `json:"format_version"`
	GeneratedAt   time.Time            `json:"generated_at"`
	SystemConfig  []BackupConfig       `json:"system_config"`
	Seats         []BackupSeat         `json:"seats"`
	Students      []BackupStudent      `json:"students"`
	SeatBookings  []BackupSeatBooking  `json:"seat_bookings"`
	CheckinEvents []BackupCheckinEvent `json:"checkin_events"`
	AuditLogs     []BackupAuditLog     `json:"audit_logs"`
	Summary       BackupSummary        `json:"summary"`
}

type BackupConfig struct {
	Key       string          `json:"key"`
	ValueJSON json.RawMessage `json:"value_json"`
	UpdatedBy *string         `json:"updated_by"`
	UpdatedAt time.Time       `json:"updated_at"`
}

type BackupSeat struct {
	SeatID      int64     `json:"seat_id"`
	Zone        string    `json:"zone"`
	RowNo       int       `json:"row_no"`
	ColNo       int       `json:"col_no"`
	IsBlocked   bool      `json:"is_blocked"`
	BlockReason *string   `json:"block_reason"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type BackupStudent struct {
	StudentID     string          `json:"student_id"`
	ClassCode     *string         `json:"class_code"`
	SeatNoInClass *int            `json:"seat_no_in_class"`
	DisplayName   *string         `json:"display_name"`
	PendingNotice json.RawMessage `json:"pending_notice,omitempty"`
	CreatedAt     time.Time       `json:"created_at"`
	UpdatedAt     time.Time       `json:"updated_at"`
}

type BackupSeatBooking struct {
	SeatID            int64     `json:"seat_id"`
	StudentID         *string   `json:"student_id"`
	CustomLabel       *string   `json:"custom_label"`
	BookedByStudentID *string   `json:"booked_by_student_id"`
	BookedAt          time.Time `json:"booked_at"`
	TicketVersion     int       `json:"ticket_version"`
}

type BackupCheckinEvent struct {
	ID             int64     `json:"id"`
	StudentID      *string   `json:"student_id"`
	SeatID         *int64    `json:"seat_id"`
	TicketVersion  *int      `json:"ticket_version"`
	Method         string    `json:"method"`
	Result         string    `json:"result"`
	IsFirstScan    bool      `json:"is_first_scan"`
	ScannerAccount *string   `json:"scanner_account"`
	IPAddr         *string   `json:"ip_addr"`
	ScannedAt      time.Time `json:"scanned_at"`
}

type BackupAuditLog struct {
	ID            int64           `json:"id"`
	ActorType     string          `json:"actor_type"`
	ActorID       *string         `json:"actor_id"`
	Action        string          `json:"action"`
	TargetSeatID  *int64          `json:"target_seat_id"`
	TargetStudent *string         `json:"target_student"`
	Before        json.RawMessage `json:"before_json,omitempty"`
	After         json.RawMessage `json:"after_json,omitempty"`
	IPAddr        *string         `json:"ip_addr"`
	CreatedAt     time.Time       `json:"created_at"`
}

type BackupSummary struct {
	SystemConfig  int `json:"system_config"`
	Seats         int `json:"seats"`
	Students      int `json:"students"`
	SeatBookings  int `json:"seat_bookings"`
	CheckinEvents int `json:"checkin_events"`
	AuditLogs     int `json:"audit_logs"`
}

type RestorePlan struct {
	SnapshotHash      string        `json:"snapshot_hash"`
	ConfirmationToken string        `json:"confirmation_token"`
	Summary           BackupSummary `json:"summary"`
	Warnings          []string      `json:"warnings"`
}

type RestoreBackupInput struct {
	Snapshot BackupSnapshot
	Actor    string
	Now      time.Time
}

type RestoreResult struct {
	SnapshotHash string        `json:"snapshot_hash"`
	RestoredAt   time.Time     `json:"restored_at"`
	Summary      BackupSummary `json:"summary"`
}

func NewService(store Store, tokens TokenIssuer, restoreSecret ...string) *Service {
	secret := ""
	if len(restoreSecret) > 0 {
		secret = restoreSecret[0]
	}
	return &Service{
		store:         store,
		tokens:        tokens,
		restoreSecret: secret,
		now:           time.Now,
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
	filter.GradePrefix = strings.TrimSpace(filter.GradePrefix)
	filter.ClassCode = strings.TrimSpace(filter.ClassCode)
	filter.Search = strings.TrimSpace(filter.Search)
	return s.store.ListStudents(ctx, filter)
}

func (s *Service) ListStudentSeatConflicts(ctx context.Context, filter StudentFilter) ([]StudentSeatConflict, error) {
	filter.Page, filter.PageSize = normalizePage(filter.Page, filter.PageSize)
	filter.GradePrefix = strings.TrimSpace(filter.GradePrefix)
	filter.ClassCode = strings.TrimSpace(filter.ClassCode)
	filter.Search = strings.TrimSpace(filter.Search)
	return s.store.ListStudentSeatConflicts(ctx, filter)
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

func (s *Service) DashboardStats(ctx context.Context) (DashboardStats, error) {
	return s.store.DashboardStats(ctx, s.now())
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
	if err := validateConfigValue(input.Key, input.ValueJSON); err != nil {
		return ConfigEntry{}, err
	}
	return s.store.SetConfig(ctx, input)
}

func validateConfigValue(key string, raw json.RawMessage) error {
	switch key {
	case "booking_open_at", "booking_lock_at", "show_start_at", "countdown_target_at":
		var value string
		if err := json.Unmarshal(raw, &value); err != nil || strings.TrimSpace(value) == "" {
			return ErrInvalidConfigValue
		}
		if _, err := time.Parse(time.RFC3339, value); err != nil {
			return ErrInvalidConfigValue
		}
	}
	return nil
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

func (s *Service) ExportBackup(ctx context.Context) (BackupSnapshot, error) {
	snapshot, err := s.store.ExportBackup(ctx, s.now())
	if err != nil {
		return BackupSnapshot{}, err
	}
	snapshot.Summary = backupSummary(snapshot)
	return snapshot, nil
}

func (s *Service) PlanRestore(snapshot BackupSnapshot) (RestorePlan, error) {
	if err := validateBackupSnapshot(snapshot); err != nil {
		return RestorePlan{}, err
	}
	hash, err := snapshotHash(snapshot)
	if err != nil {
		return RestorePlan{}, err
	}
	warnings := []string{
		"restore replaces system_config, seats, students, seat_bookings, checkin_events, and audit_logs",
		"staff_accounts are not restored and continue to be seeded from environment variables",
	}
	return RestorePlan{
		SnapshotHash:      hash,
		ConfirmationToken: s.restoreToken(hash),
		Summary:           backupSummary(snapshot),
		Warnings:          warnings,
	}, nil
}

func (s *Service) RestoreBackup(ctx context.Context, snapshot BackupSnapshot, token, actor string) (RestoreResult, error) {
	plan, err := s.PlanRestore(snapshot)
	if err != nil {
		return RestoreResult{}, err
	}
	if !hmac.Equal([]byte(strings.TrimSpace(token)), []byte(plan.ConfirmationToken)) {
		return RestoreResult{}, ErrInvalidRestoreToken
	}
	now := s.now()
	result, err := s.store.RestoreBackup(ctx, RestoreBackupInput{
		Snapshot: snapshot,
		Actor:    strings.TrimSpace(actor),
		Now:      now,
	})
	if err != nil {
		return RestoreResult{}, err
	}
	result.SnapshotHash = plan.SnapshotHash
	result.RestoredAt = now.UTC()
	result.Summary = plan.Summary
	return result, nil
}

func (s *Service) restoreToken(hash string) string {
	mac := hmac.New(sha256.New, []byte(s.restoreSecret))
	mac.Write([]byte("lalaland:restore:v1:"))
	mac.Write([]byte(hash))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func snapshotHash(snapshot BackupSnapshot) (string, error) {
	raw, err := json.Marshal(snapshot)
	if err != nil {
		return "", fmt.Errorf("marshal backup snapshot: %w", err)
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

func backupSummary(snapshot BackupSnapshot) BackupSummary {
	return BackupSummary{
		SystemConfig:  len(snapshot.SystemConfig),
		Seats:         len(snapshot.Seats),
		Students:      len(snapshot.Students),
		SeatBookings:  len(snapshot.SeatBookings),
		CheckinEvents: len(snapshot.CheckinEvents),
		AuditLogs:     len(snapshot.AuditLogs),
	}
}

func validateBackupSnapshot(snapshot BackupSnapshot) error {
	if snapshot.FormatVersion != 1 {
		return ErrInvalidBackup
	}
	if len(snapshot.Seats) == 0 {
		return ErrInvalidBackup
	}

	seatIDs := map[int64]struct{}{}
	for _, seat := range snapshot.Seats {
		if seat.SeatID <= 0 || seat.RowNo <= 0 || seat.ColNo <= 0 {
			return ErrInvalidBackup
		}
		if seat.Zone != "left" && seat.Zone != "middle" && seat.Zone != "right" {
			return ErrInvalidBackup
		}
		if _, exists := seatIDs[seat.SeatID]; exists {
			return ErrInvalidBackup
		}
		seatIDs[seat.SeatID] = struct{}{}
	}

	studentIDs := map[string]struct{}{}
	for _, studentRow := range snapshot.Students {
		if !student.IsValidStudentID(studentRow.StudentID) {
			return ErrInvalidBackup
		}
		if (studentRow.ClassCode == nil) != (studentRow.SeatNoInClass == nil) {
			return ErrInvalidBackup
		}
		if studentRow.ClassCode != nil && !student.IsValidClassCode(*studentRow.ClassCode) {
			return ErrInvalidBackup
		}
		if studentRow.SeatNoInClass != nil && !student.IsValidSeatNoInClass(*studentRow.SeatNoInClass) {
			return ErrInvalidBackup
		}
		if len(studentRow.PendingNotice) > 0 && !json.Valid(studentRow.PendingNotice) {
			return ErrInvalidBackup
		}
		if _, exists := studentIDs[studentRow.StudentID]; exists {
			return ErrInvalidBackup
		}
		studentIDs[studentRow.StudentID] = struct{}{}
	}

	bookedSeats := map[int64]struct{}{}
	bookedStudents := map[string]struct{}{}
	for _, booking := range snapshot.SeatBookings {
		if _, exists := seatIDs[booking.SeatID]; !exists {
			return ErrInvalidBackup
		}
		if (booking.StudentID == nil) == (booking.CustomLabel == nil) {
			return ErrInvalidBackup
		}
		if booking.StudentID != nil {
			if !student.IsValidStudentID(*booking.StudentID) {
				return ErrInvalidBackup
			}
			if _, exists := studentIDs[*booking.StudentID]; !exists {
				return ErrInvalidBackup
			}
			if _, exists := bookedStudents[*booking.StudentID]; exists {
				return ErrInvalidBackup
			}
			bookedStudents[*booking.StudentID] = struct{}{}
		}
		if booking.BookedByStudentID != nil {
			if !student.IsValidStudentID(*booking.BookedByStudentID) {
				return ErrInvalidBackup
			}
			if _, exists := studentIDs[*booking.BookedByStudentID]; !exists {
				return ErrInvalidBackup
			}
		}
		if booking.TicketVersion <= 0 {
			return ErrInvalidBackup
		}
		if _, exists := bookedSeats[booking.SeatID]; exists {
			return ErrInvalidBackup
		}
		bookedSeats[booking.SeatID] = struct{}{}
	}

	configKeys := map[string]struct{}{}
	for _, entry := range snapshot.SystemConfig {
		if !configKeyPattern.MatchString(entry.Key) || !json.Valid(entry.ValueJSON) {
			return ErrInvalidBackup
		}
		if _, exists := configKeys[entry.Key]; exists {
			return ErrInvalidBackup
		}
		configKeys[entry.Key] = struct{}{}
	}
	return nil
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
