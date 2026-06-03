package admin

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"lalalandshow/backend/internal/auth"
	"lalalandshow/backend/internal/student"
)

type fakeStore struct {
	account      StaffAccount
	accountErr   error
	assignInput  AssignSeatInput
	assignErr    error
	configInput  ConfigInput
	exportType   string
	windowState  WindowState
	dashboardAt  time.Time
	backupAt     time.Time
	restoreInput RestoreBackupInput
}

func (s *fakeStore) FindStaffAccount(_ context.Context, _ string) (StaffAccount, error) {
	if s.accountErr != nil {
		return StaffAccount{}, s.accountErr
	}
	return s.account, nil
}

func (s *fakeStore) ListSeats(context.Context) ([]SeatDetail, error) {
	return []SeatDetail{}, nil
}

func (s *fakeStore) AssignSeat(_ context.Context, input AssignSeatInput) (SeatDetail, error) {
	s.assignInput = input
	if s.assignErr != nil {
		return SeatDetail{}, s.assignErr
	}
	return SeatDetail{SeatID: input.SeatID}, nil
}

func (s *fakeStore) LockSeat(_ context.Context, input LockSeatInput) (SeatDetail, error) {
	return SeatDetail{SeatID: input.SeatID, IsBlocked: input.Locked}, nil
}

func (s *fakeStore) ListStudents(_ context.Context, filter StudentFilter) (StudentList, error) {
	return StudentList{Page: filter.Page, PageSize: filter.PageSize}, nil
}

func (s *fakeStore) ListStudentSeatConflicts(_ context.Context, _ StudentFilter) ([]StudentSeatConflict, error) {
	return []StudentSeatConflict{}, nil
}

func (s *fakeStore) ListAuditLogs(_ context.Context, filter AuditFilter) (AuditLogList, error) {
	return AuditLogList{Page: filter.Page, PageSize: filter.PageSize}, nil
}

func (s *fakeStore) ListCheckinEvents(_ context.Context, filter CheckinFilter) (CheckinEventList, error) {
	return CheckinEventList{Page: filter.Page, PageSize: filter.PageSize}, nil
}

func (s *fakeStore) DashboardStats(_ context.Context, now time.Time) (DashboardStats, error) {
	s.dashboardAt = now
	return DashboardStats{GeneratedAt: now.UTC(), Window: WindowState{State: "open"}}, nil
}

func (s *fakeStore) GetConfig(_ context.Context, key string) (ConfigEntry, error) {
	return ConfigEntry{Key: key, ValueJSON: json.RawMessage(`"ok"`)}, nil
}

func (s *fakeStore) SetConfig(_ context.Context, input ConfigInput) (ConfigEntry, error) {
	s.configInput = input
	return ConfigEntry{Key: input.Key, ValueJSON: input.ValueJSON}, nil
}

func (s *fakeStore) PublicConfig(context.Context) (map[string]json.RawMessage, error) {
	return map[string]json.RawMessage{}, nil
}

func (s *fakeStore) WindowState(context.Context, time.Time) (WindowState, error) {
	if s.windowState.State == "" {
		return WindowState{State: "open"}, nil
	}
	return s.windowState, nil
}

func (s *fakeStore) ExportCSV(_ context.Context, exportType string) (CSVExport, error) {
	s.exportType = exportType
	return CSVExport{Filename: "test.csv", Header: []string{"id"}}, nil
}

func (s *fakeStore) ExportBackup(_ context.Context, now time.Time) (BackupSnapshot, error) {
	s.backupAt = now
	return minimalBackupSnapshot(now), nil
}

func (s *fakeStore) RestoreBackup(_ context.Context, input RestoreBackupInput) (RestoreResult, error) {
	s.restoreInput = input
	return RestoreResult{RestoredAt: input.Now.UTC(), Summary: backupSummary(input.Snapshot)}, nil
}

func TestServiceLoginRequiresAdminRole(t *testing.T) {
	hash, err := bcrypt.GenerateFromPassword([]byte("secret"), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("GenerateFromPassword() error = %v", err)
	}
	service := NewService(&fakeStore{account: StaffAccount{
		Username:     "checker",
		PasswordHash: string(hash),
		Role:         "checker",
	}}, auth.NewManager("jwt-secret", time.Hour))

	if _, err := service.Login(context.Background(), "checker", "secret"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("Login(checker) error = %v, want %v", err, ErrForbidden)
	}
}

func TestServiceLoginReturnsAdminToken(t *testing.T) {
	hash, err := bcrypt.GenerateFromPassword([]byte("secret"), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("GenerateFromPassword() error = %v", err)
	}
	manager := auth.NewManager("jwt-secret", time.Hour)
	service := NewService(&fakeStore{account: StaffAccount{
		Username:     "admin",
		PasswordHash: string(hash),
		Role:         "admin",
	}}, manager)
	service.now = func() time.Time { return time.Unix(1000, 0).UTC() }

	result, err := service.Login(context.Background(), "admin", "secret")
	if err != nil {
		t.Fatalf("Login(admin) error = %v", err)
	}
	claims, err := manager.ParseStaffToken(result.Token, time.Unix(1001, 0).UTC(), "admin")
	if err != nil {
		t.Fatalf("ParseStaffToken() error = %v", err)
	}
	if claims.Subject != "admin" || claims.Role != "admin" {
		t.Fatalf("claims = %#v, want admin", claims)
	}
}

func TestServiceAssignSeatValidatesInput(t *testing.T) {
	service := NewService(&fakeStore{}, auth.NewManager("jwt-secret", time.Hour))

	if _, err := service.AssignSeat(context.Background(), AssignSeatInput{SeatID: 1}); !errors.Is(err, ErrInvalidAssignment) {
		t.Fatalf("AssignSeat(empty) error = %v, want %v", err, ErrInvalidAssignment)
	}
	if _, err := service.AssignSeat(context.Background(), AssignSeatInput{SeatID: 1, StudentID: "abc"}); !errors.Is(err, student.ErrInvalidStudentID) {
		t.Fatalf("AssignSeat(invalid student) error = %v, want invalid student id", err)
	}
}

func TestServiceSetConfigRequiresJSON(t *testing.T) {
	service := NewService(&fakeStore{}, auth.NewManager("jwt-secret", time.Hour))

	if _, err := service.SetConfig(context.Background(), ConfigInput{Key: "play_name", ValueJSON: json.RawMessage(`not-json`)}); !errors.Is(err, ErrInvalidJSON) {
		t.Fatalf("SetConfig(invalid JSON) error = %v, want %v", err, ErrInvalidJSON)
	}
}

func TestServiceSetConfigValidatesTimeValues(t *testing.T) {
	service := NewService(&fakeStore{}, auth.NewManager("jwt-secret", time.Hour))

	if _, err := service.SetConfig(context.Background(), ConfigInput{Key: "booking_open_at", ValueJSON: json.RawMessage(`"tomorrow"`)}); !errors.Is(err, ErrInvalidConfigValue) {
		t.Fatalf("SetConfig(invalid time) error = %v, want %v", err, ErrInvalidConfigValue)
	}
	if _, err := service.SetConfig(context.Background(), ConfigInput{Key: "booking_open_at", ValueJSON: json.RawMessage(`"2026-05-31T12:00:00Z"`)}); err != nil {
		t.Fatalf("SetConfig(valid time) error = %v", err)
	}
}

func TestServiceExportTypeWhitelist(t *testing.T) {
	store := &fakeStore{}
	service := NewService(store, auth.NewManager("jwt-secret", time.Hour))

	if _, err := service.ExportCSV(context.Background(), "bookings"); err != nil {
		t.Fatalf("ExportCSV(bookings) error = %v", err)
	}
	if store.exportType != "bookings" {
		t.Fatalf("exportType = %q, want bookings", store.exportType)
	}
	if _, err := service.ExportCSV(context.Background(), "secrets"); !errors.Is(err, ErrInvalidExportType) {
		t.Fatalf("ExportCSV(secrets) error = %v, want %v", err, ErrInvalidExportType)
	}
}

func TestServiceDashboardStatsUsesServiceClock(t *testing.T) {
	store := &fakeStore{}
	service := NewService(store, auth.NewManager("jwt-secret", time.Hour))
	now := time.Date(2026, 5, 29, 12, 30, 0, 0, time.FixedZone("CST", 8*60*60))
	service.now = func() time.Time { return now }

	stats, err := service.DashboardStats(context.Background())
	if err != nil {
		t.Fatalf("DashboardStats() error = %v", err)
	}
	if !store.dashboardAt.Equal(now) {
		t.Fatalf("dashboardAt = %v, want %v", store.dashboardAt, now)
	}
	if stats.GeneratedAt.Location() != time.UTC {
		t.Fatalf("GeneratedAt location = %v, want UTC", stats.GeneratedAt.Location())
	}
}

func TestServicePlanRestoreRejectsInvalidSnapshot(t *testing.T) {
	service := NewService(&fakeStore{}, auth.NewManager("jwt-secret", time.Hour), "restore-secret")

	if _, err := service.PlanRestore(BackupSnapshot{FormatVersion: 99}); !errors.Is(err, ErrInvalidBackup) {
		t.Fatalf("PlanRestore(invalid) error = %v, want %v", err, ErrInvalidBackup)
	}
}

func TestServiceRestoreBackupRequiresConfirmationToken(t *testing.T) {
	store := &fakeStore{}
	service := NewService(store, auth.NewManager("jwt-secret", time.Hour), "restore-secret")
	now := time.Date(2026, 5, 29, 12, 30, 0, 0, time.UTC)
	service.now = func() time.Time { return now }
	snapshot := minimalBackupSnapshot(now)

	if _, err := service.RestoreBackup(context.Background(), snapshot, "wrong", "admin"); !errors.Is(err, ErrInvalidRestoreToken) {
		t.Fatalf("RestoreBackup(wrong token) error = %v, want %v", err, ErrInvalidRestoreToken)
	}

	plan, err := service.PlanRestore(snapshot)
	if err != nil {
		t.Fatalf("PlanRestore() error = %v", err)
	}
	result, err := service.RestoreBackup(context.Background(), snapshot, plan.ConfirmationToken, "admin")
	if err != nil {
		t.Fatalf("RestoreBackup() error = %v", err)
	}
	if result.SnapshotHash != plan.SnapshotHash {
		t.Fatalf("SnapshotHash = %q, want %q", result.SnapshotHash, plan.SnapshotHash)
	}
	if store.restoreInput.Actor != "admin" {
		t.Fatalf("restore actor = %q, want admin", store.restoreInput.Actor)
	}
}

func minimalBackupSnapshot(now time.Time) BackupSnapshot {
	return BackupSnapshot{
		FormatVersion: 1,
		GeneratedAt:   now.UTC(),
		SystemConfig: []BackupConfig{{
			Key:       "play_name",
			ValueJSON: json.RawMessage(`"La La Land"`),
			UpdatedAt: now.UTC(),
		}},
		Seats: []BackupSeat{{
			SeatID:    1,
			Zone:      "middle",
			RowNo:     1,
			ColNo:     1,
			CreatedAt: now.UTC(),
			UpdatedAt: now.UTC(),
		}},
		Students: []BackupStudent{{
			StudentID: "1121001",
			CreatedAt: now.UTC(),
			UpdatedAt: now.UTC(),
		}},
		SeatBookings: []BackupSeatBooking{{
			SeatID:        1,
			StudentID:     stringLiteral("1121001"),
			BookedAt:      now.UTC(),
			TicketVersion: 1,
		}},
	}
}

func stringLiteral(value string) *string {
	return &value
}
