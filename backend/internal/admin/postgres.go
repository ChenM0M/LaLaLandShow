package admin

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"lalalandshow/backend/internal/student"
)

const uniqueViolationCode = "23505"

type PostgresStore struct {
	db *pgxpool.Pool
}

type currentBooking struct {
	SeatID            int64
	StudentID         *string
	CustomLabel       *string
	BookedByStudentID *string
	TicketVersion     int
	BookedAt          time.Time
}

func NewPostgresStore(db *pgxpool.Pool) *PostgresStore {
	return &PostgresStore{db: db}
}

func (s *PostgresStore) FindStaffAccount(ctx context.Context, username string) (StaffAccount, error) {
	row := s.db.QueryRow(ctx, `
SELECT username, password_hash, role
FROM staff_accounts
WHERE username = $1`, username)
	var account StaffAccount
	if err := row.Scan(&account.Username, &account.PasswordHash, &account.Role); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return StaffAccount{}, ErrInvalidCredentials
		}
		return StaffAccount{}, fmt.Errorf("find admin account: %w", err)
	}
	return account, nil
}

func (s *PostgresStore) ListSeats(ctx context.Context) ([]SeatDetail, error) {
	rows, err := s.db.Query(ctx, seatDetailSQL+`
ORDER BY
  CASE se.zone WHEN 'left' THEN 1 WHEN 'middle' THEN 2 WHEN 'right' THEN 3 ELSE 4 END,
  se.row_no,
  se.col_no`)
	if err != nil {
		return nil, fmt.Errorf("query admin seats: %w", err)
	}
	defer rows.Close()

	seats := []SeatDetail{}
	for rows.Next() {
		seat, err := scanSeatDetail(rows)
		if err != nil {
			return nil, err
		}
		seats = append(seats, seat)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate admin seats: %w", err)
	}
	return seats, nil
}

func (s *PostgresStore) AssignSeat(ctx context.Context, input AssignSeatInput) (SeatDetail, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return SeatDetail{}, fmt.Errorf("begin admin assign transaction: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	if err := lockSeat(ctx, tx, input.SeatID); err != nil {
		return SeatDetail{}, err
	}

	targetBooking, targetExists, err := getSeatBookingForUpdate(ctx, tx, input.SeatID)
	if err != nil {
		return SeatDetail{}, err
	}

	if input.Release {
		if targetExists {
			if err := deleteBookingBySeat(ctx, tx, input.SeatID); err != nil {
				return SeatDetail{}, err
			}
			if err := setNoticeForBooking(ctx, tx, targetBooking, input.SeatID, nil, input.Actor); err != nil {
				return SeatDetail{}, err
			}
			if err := writeAudit(ctx, tx, "admin_release", input.Actor, input.SeatID, targetBooking.StudentID, bookingAuditJSON(targetBooking), nil); err != nil {
				return SeatDetail{}, err
			}
		}
		if err := tx.Commit(ctx); err != nil {
			return SeatDetail{}, fmt.Errorf("commit admin release: %w", err)
		}
		return s.GetSeat(ctx, input.SeatID)
	}

	if targetExists && !input.ForceReleaseExisting {
		return SeatDetail{}, ErrAssignmentConflict
	}
	if targetExists {
		if err := deleteBookingBySeat(ctx, tx, input.SeatID); err != nil {
			return SeatDetail{}, err
		}
		if err := setNoticeForBooking(ctx, tx, targetBooking, input.SeatID, nil, input.Actor); err != nil {
			return SeatDetail{}, err
		}
	}

	version := 1
	var movingBooking *currentBooking
	if input.StudentID != "" {
		if err := ensureAssignableStudentProfile(ctx, tx, input.StudentID); err != nil {
			return SeatDetail{}, err
		}

		studentBooking, exists, err := getStudentBookingForUpdate(ctx, tx, input.StudentID)
		if err != nil {
			return SeatDetail{}, err
		}
		if exists && studentBooking.SeatID != input.SeatID {
			if !input.ForceReleaseExisting {
				return SeatDetail{}, ErrAssignmentConflict
			}
			if err := deleteBookingBySeat(ctx, tx, studentBooking.SeatID); err != nil {
				return SeatDetail{}, err
			}
			movingBooking = &studentBooking
			version = studentBooking.TicketVersion + 1
		}
	}

	inserted, err := insertAdminBooking(ctx, tx, input.SeatID, input.StudentID, input.CustomLabel, version)
	if err != nil {
		return SeatDetail{}, err
	}
	if input.StudentID != "" {
		var previousSeatID *int64
		if movingBooking != nil {
			previousSeatID = &movingBooking.SeatID
		}
		if err := setStudentNotice(ctx, tx, input.StudentID, previousSeatID, &input.SeatID, input.Actor); err != nil {
			return SeatDetail{}, err
		}
	}

	var before json.RawMessage
	if targetExists {
		before = bookingAuditJSON(targetBooking)
	}
	if movingBooking != nil && before == nil {
		before = bookingAuditJSON(*movingBooking)
	}
	action := "admin_assign"
	if targetExists || movingBooking != nil || input.ForceReleaseExisting {
		action = "admin_force"
	}
	if err := writeAudit(ctx, tx, action, input.Actor, input.SeatID, inserted.StudentID, before, bookingAuditJSON(inserted)); err != nil {
		return SeatDetail{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return SeatDetail{}, fmt.Errorf("commit admin assign: %w", err)
	}
	return s.GetSeat(ctx, input.SeatID)
}

func ensureAssignableStudentProfile(ctx context.Context, tx pgx.Tx, studentID string) error {
	var exists bool
	if err := tx.QueryRow(ctx, `
SELECT EXISTS (
  SELECT 1
  FROM students
  WHERE student_id = $1
    AND class_code IS NOT NULL
    AND class_code ~ '^(7|8|9|10|11|12)0[1-5]$'
    AND seat_no_in_class IS NOT NULL
    AND seat_no_in_class BETWEEN 1 AND 70
)`, studentID).Scan(&exists); err != nil {
		return fmt.Errorf("check assigned student registration: %w", err)
	}
	if !exists {
		return student.ErrIncompleteProfile
	}
	return nil
}

func (s *PostgresStore) LockSeat(ctx context.Context, input LockSeatInput) (SeatDetail, error) {
	row := s.db.QueryRow(ctx, `
UPDATE seats
SET is_blocked = $2,
    block_reason = CASE WHEN $2 THEN $3 ELSE NULL END,
    updated_at = now()
WHERE seat_id = $1
RETURNING seat_id`, input.SeatID, input.Locked, input.BlockReason)
	var seatID int64
	if err := row.Scan(&seatID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return SeatDetail{}, ErrSeatNotFound
		}
		return SeatDetail{}, fmt.Errorf("lock admin seat: %w", err)
	}
	after := json.RawMessage(fmt.Sprintf(`{"seat_id":%d,"is_blocked":%t}`, input.SeatID, input.Locked))
	if err := s.writePoolAudit(ctx, "admin_lock", input.Actor, input.SeatID, nil, nil, after); err != nil {
		return SeatDetail{}, err
	}
	return s.GetSeat(ctx, input.SeatID)
}

func (s *PostgresStore) GetSeat(ctx context.Context, seatID int64) (SeatDetail, error) {
	row := s.db.QueryRow(ctx, seatDetailSQL+`WHERE se.seat_id = $1`, seatID)
	seat, err := scanSeatDetail(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return SeatDetail{}, ErrSeatNotFound
		}
		return SeatDetail{}, err
	}
	return seat, nil
}

func (s *PostgresStore) ListStudents(ctx context.Context, filter StudentFilter) (StudentList, error) {
	offset := (filter.Page - 1) * filter.PageSize
	rows, err := s.db.Query(ctx, `
WITH filtered AS (
  SELECT st.student_id,
         st.class_code,
         st.seat_no_in_class,
         st.display_name,
         st.pending_notice,
         b.seat_id,
         b.booked_at,
         b.ticket_version,
         b.booked_by_student_id
  FROM students st
  LEFT JOIN seat_bookings b ON b.student_id = st.student_id
  WHERE ($1 = '' OR st.class_code LIKE $1 || '%')
    AND ($2 = '' OR st.class_code = $2)
    AND ($3 = '' OR st.student_id ILIKE '%' || $3 || '%' OR st.display_name ILIKE '%' || $3 || '%')
)
SELECT count(*) OVER(),
       student_id,
       class_code,
       seat_no_in_class,
       display_name,
       pending_notice,
       seat_id,
       booked_at,
       ticket_version,
       booked_by_student_id
FROM filtered
ORDER BY student_id
LIMIT $4 OFFSET $5`, filter.GradePrefix, filter.ClassCode, filter.Search, filter.PageSize, offset)
	if err != nil {
		return StudentList{}, fmt.Errorf("query admin students: %w", err)
	}
	defer rows.Close()

	list := StudentList{Students: []StudentRow{}, Page: filter.Page, PageSize: filter.PageSize}
	for rows.Next() {
		var (
			row             StudentRow
			total           int
			classCode       sql.NullString
			seatNoInClass   sql.NullInt64
			displayName     sql.NullString
			pendingNotice   []byte
			seatID          sql.NullInt64
			bookedAt        sql.NullTime
			ticketVersion   sql.NullInt64
			bookedByStudent sql.NullString
		)
		if err := rows.Scan(
			&total,
			&row.StudentID,
			&classCode,
			&seatNoInClass,
			&displayName,
			&pendingNotice,
			&seatID,
			&bookedAt,
			&ticketVersion,
			&bookedByStudent,
		); err != nil {
			return StudentList{}, fmt.Errorf("scan admin student: %w", err)
		}
		list.Total = total
		row.ClassCode = stringPtr(classCode)
		row.SeatNoInClass = intPtr(seatNoInClass)
		row.DisplayName = stringPtr(displayName)
		row.NeedRegister = row.ClassCode == nil || row.SeatNoInClass == nil
		if len(pendingNotice) > 0 {
			row.PendingNotice = append(json.RawMessage(nil), pendingNotice...)
		}
		if seatID.Valid {
			value := seatID.Int64
			row.SeatID = &value
		}
		if bookedAt.Valid {
			value := bookedAt.Time
			row.BookedAt = &value
		}
		if ticketVersion.Valid {
			value := int(ticketVersion.Int64)
			row.TicketVersion = &value
		}
		row.ProxyBookedBy = stringPtr(bookedByStudent)
		list.Students = append(list.Students, row)
	}
	if err := rows.Err(); err != nil {
		return StudentList{}, fmt.Errorf("iterate admin students: %w", err)
	}
	return list, nil
}

func (s *PostgresStore) ListStudentSeatConflicts(ctx context.Context, filter StudentFilter) ([]StudentSeatConflict, error) {
	rows, err := s.db.Query(ctx, `
WITH filtered AS (
  SELECT student_id, class_code, seat_no_in_class, display_name
  FROM students
  WHERE class_code IS NOT NULL
    AND btrim(class_code) <> ''
    AND seat_no_in_class IS NOT NULL
    AND seat_no_in_class > 0
    AND ($1 = '' OR class_code LIKE $1 || '%')
    AND ($2 = '' OR class_code = $2)
    AND ($3 = '' OR student_id ILIKE '%' || $3 || '%' OR display_name ILIKE '%' || $3 || '%')
)
SELECT class_code,
       seat_no_in_class,
       array_agg(student_id ORDER BY student_id),
       count(*)::int
FROM filtered
GROUP BY class_code, seat_no_in_class
HAVING count(*) > 1
ORDER BY class_code, seat_no_in_class`, filter.GradePrefix, filter.ClassCode, filter.Search)
	if err != nil {
		return nil, fmt.Errorf("query student seat conflicts: %w", err)
	}
	defer rows.Close()

	conflicts := []StudentSeatConflict{}
	for rows.Next() {
		var conflict StudentSeatConflict
		if err := rows.Scan(&conflict.ClassCode, &conflict.SeatNoInClass, &conflict.StudentIDs, &conflict.Count); err != nil {
			return nil, fmt.Errorf("scan student seat conflict: %w", err)
		}
		conflicts = append(conflicts, conflict)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate student seat conflicts: %w", err)
	}
	return conflicts, nil
}

func (s *PostgresStore) ListAuditLogs(ctx context.Context, filter AuditFilter) (AuditLogList, error) {
	offset := (filter.Page - 1) * filter.PageSize
	rows, err := s.db.Query(ctx, `
WITH filtered AS (
  SELECT id, actor_type, actor_id, action, target_seat_id, target_student, before_json, after_json, created_at
  FROM audit_logs
  WHERE ($1 = '' OR action = $1)
    AND ($2 = '' OR actor_id = $2)
)
SELECT count(*) OVER(), id, actor_type, actor_id, action, target_seat_id, target_student, before_json, after_json, created_at
FROM filtered
ORDER BY created_at DESC, id DESC
LIMIT $3 OFFSET $4`, filter.Action, filter.Actor, filter.PageSize, offset)
	if err != nil {
		return AuditLogList{}, fmt.Errorf("query admin audit logs: %w", err)
	}
	defer rows.Close()

	list := AuditLogList{Logs: []AuditLogRow{}, Page: filter.Page, PageSize: filter.PageSize}
	for rows.Next() {
		var (
			total         int
			row           AuditLogRow
			actorID       sql.NullString
			targetSeatID  sql.NullInt64
			targetStudent sql.NullString
			beforeJSON    []byte
			afterJSON     []byte
		)
		if err := rows.Scan(&total, &row.ID, &row.ActorType, &actorID, &row.Action, &targetSeatID, &targetStudent, &beforeJSON, &afterJSON, &row.CreatedAt); err != nil {
			return AuditLogList{}, fmt.Errorf("scan admin audit log: %w", err)
		}
		list.Total = total
		row.ActorID = stringPtr(actorID)
		if targetSeatID.Valid {
			value := targetSeatID.Int64
			row.TargetSeatID = &value
		}
		row.TargetStudent = stringPtr(targetStudent)
		row.Before = rawJSON(beforeJSON)
		row.After = rawJSON(afterJSON)
		list.Logs = append(list.Logs, row)
	}
	if err := rows.Err(); err != nil {
		return AuditLogList{}, fmt.Errorf("iterate admin audit logs: %w", err)
	}
	return list, nil
}

func (s *PostgresStore) ListCheckinEvents(ctx context.Context, filter CheckinFilter) (CheckinEventList, error) {
	offset := (filter.Page - 1) * filter.PageSize
	rows, err := s.db.Query(ctx, `
WITH filtered AS (
  SELECT e.id,
         e.student_id,
         e.seat_id,
         e.ticket_version,
         e.method,
         e.result,
         e.is_first_scan,
         e.scanner_account,
         e.scanned_at,
         st.class_code,
         st.seat_no_in_class,
         st.display_name
  FROM checkin_events e
  LEFT JOIN students st ON st.student_id = e.student_id
  WHERE ($1 = '' OR e.student_id = $1)
    AND ($2 = '' OR e.method = $2)
)
SELECT count(*) OVER(), id, student_id, seat_id, ticket_version, method, result, is_first_scan, scanner_account, scanned_at, class_code, seat_no_in_class, display_name
FROM filtered
ORDER BY scanned_at DESC, id DESC
LIMIT $3 OFFSET $4`, filter.StudentID, filter.Method, filter.PageSize, offset)
	if err != nil {
		return CheckinEventList{}, fmt.Errorf("query admin checkin events: %w", err)
	}
	defer rows.Close()

	list := CheckinEventList{Events: []CheckinEventRow{}, Page: filter.Page, PageSize: filter.PageSize}
	for rows.Next() {
		var (
			total          int
			row            CheckinEventRow
			studentID      sql.NullString
			seatID         sql.NullInt64
			ticketVersion  sql.NullInt64
			scannerAccount sql.NullString
			classCode      sql.NullString
			seatNoInClass  sql.NullInt64
			displayName    sql.NullString
		)
		if err := rows.Scan(&total, &row.ID, &studentID, &seatID, &ticketVersion, &row.Method, &row.Result, &row.IsFirstScan, &scannerAccount, &row.ScannedAt, &classCode, &seatNoInClass, &displayName); err != nil {
			return CheckinEventList{}, fmt.Errorf("scan admin checkin event: %w", err)
		}
		list.Total = total
		row.StudentID = stringPtr(studentID)
		if seatID.Valid {
			value := seatID.Int64
			row.SeatID = &value
		}
		if ticketVersion.Valid {
			value := int(ticketVersion.Int64)
			row.TicketVersion = &value
		}
		row.ScannerAccount = stringPtr(scannerAccount)
		row.ClassCode = stringPtr(classCode)
		row.SeatNoInClass = intPtr(seatNoInClass)
		row.DisplayName = stringPtr(displayName)
		list.Events = append(list.Events, row)
	}
	if err := rows.Err(); err != nil {
		return CheckinEventList{}, fmt.Errorf("iterate admin checkin events: %w", err)
	}
	return list, nil
}

func (s *PostgresStore) DashboardStats(ctx context.Context, now time.Time) (DashboardStats, error) {
	stats := DashboardStats{GeneratedAt: now.UTC()}

	window, err := s.WindowState(ctx, now)
	if err != nil {
		return DashboardStats{}, err
	}
	stats.Window = window

	if err := s.db.QueryRow(ctx, `
SELECT count(*)::int,
       count(b.seat_id)::int,
       count(*) FILTER (WHERE b.seat_id IS NULL AND NOT se.is_blocked)::int,
       count(*) FILTER (WHERE se.is_blocked)::int,
       count(*) FILTER (WHERE b.student_id IS NOT NULL)::int,
       count(*) FILTER (WHERE b.custom_label IS NOT NULL)::int
FROM seats se
LEFT JOIN seat_bookings b ON b.seat_id = se.seat_id`).Scan(
		&stats.Seats.Total,
		&stats.Seats.Booked,
		&stats.Seats.Free,
		&stats.Seats.Blocked,
		&stats.Seats.StudentBooked,
		&stats.Seats.CustomOccupied,
	); err != nil {
		return DashboardStats{}, fmt.Errorf("query dashboard seat stats: %w", err)
	}

	if err := s.db.QueryRow(ctx, `
SELECT count(*)::int,
       count(*) FILTER (WHERE class_code IS NOT NULL AND seat_no_in_class IS NOT NULL)::int,
       count(*) FILTER (WHERE class_code IS NULL OR seat_no_in_class IS NULL)::int,
       count(*) FILTER (WHERE pending_notice IS NOT NULL)::int
FROM students`).Scan(
		&stats.Students.Total,
		&stats.Students.Registered,
		&stats.Students.NeedRegister,
		&stats.Students.PendingNotice,
	); err != nil {
		return DashboardStats{}, fmt.Errorf("query dashboard student stats: %w", err)
	}

	if err := s.db.QueryRow(ctx, `
SELECT count(*) FILTER (WHERE result = 'approved')::int,
       count(*) FILTER (WHERE result <> 'approved')::int,
       count(DISTINCT student_id) FILTER (WHERE result = 'approved' AND student_id IS NOT NULL)::int,
       count(*) FILTER (WHERE result = 'approved' AND is_first_scan)::int,
       count(*) FILTER (WHERE result = 'approved' AND scanned_at >= $1::timestamptz - interval '1 minute')::int
FROM checkin_events`, now).Scan(
		&stats.Checkins.Approved,
		&stats.Checkins.Rejected,
		&stats.Checkins.UniqueStudents,
		&stats.Checkins.FirstScans,
		&stats.Checkins.LastMinuteRate,
	); err != nil {
		return DashboardStats{}, fmt.Errorf("query dashboard checkin stats: %w", err)
	}

	audit, err := s.ListAuditLogs(ctx, AuditFilter{Page: 1, PageSize: 8})
	if err != nil {
		return DashboardStats{}, err
	}
	checkins, err := s.ListCheckinEvents(ctx, CheckinFilter{Page: 1, PageSize: 8})
	if err != nil {
		return DashboardStats{}, err
	}
	stats.Recent = DashboardRecent{
		AuditLogs:     audit.Logs,
		CheckinEvents: checkins.Events,
	}
	return stats, nil
}

func (s *PostgresStore) GetConfig(ctx context.Context, key string) (ConfigEntry, error) {
	row := s.db.QueryRow(ctx, `
SELECT key, value_json, updated_by, updated_at
FROM system_config
WHERE key = $1`, key)
	entry, err := scanConfig(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return ConfigEntry{}, ErrConfigNotFound
	}
	if err != nil {
		return ConfigEntry{}, fmt.Errorf("get admin config: %w", err)
	}
	return entry, nil
}

func (s *PostgresStore) SetConfig(ctx context.Context, input ConfigInput) (ConfigEntry, error) {
	row := s.db.QueryRow(ctx, `
INSERT INTO system_config (key, value_json, updated_by, updated_at)
VALUES ($1, $2::jsonb, $3, now())
ON CONFLICT (key) DO UPDATE
SET value_json = excluded.value_json,
    updated_by = excluded.updated_by,
    updated_at = now()
RETURNING key, value_json, updated_by, updated_at`, input.Key, string(input.ValueJSON), input.Actor)
	entry, err := scanConfig(row)
	if err != nil {
		return ConfigEntry{}, fmt.Errorf("set admin config: %w", err)
	}
	after, _ := json.Marshal(map[string]any{"key": entry.Key, "value_json": json.RawMessage(entry.ValueJSON)})
	if err := s.writePoolAudit(ctx, "admin_config", input.Actor, 0, nil, nil, json.RawMessage(after)); err != nil {
		return ConfigEntry{}, err
	}
	return entry, nil
}

func (s *PostgresStore) PublicConfig(ctx context.Context) (map[string]json.RawMessage, error) {
	rows, err := s.db.Query(ctx, `
SELECT key, value_json
FROM system_config
WHERE key = ANY($1)
ORDER BY key`, []string{
		"play_name",
		"venue",
		"countdown_target_at",
		"show_start_at",
		"booking_open_at",
		"booking_lock_at",
		"story_background",
		"story_summary",
		"cast_list",
		"gallery_urls",
	})
	if err != nil {
		return nil, fmt.Errorf("query public config: %w", err)
	}
	defer rows.Close()

	config := map[string]json.RawMessage{}
	for rows.Next() {
		var key string
		var value []byte
		if err := rows.Scan(&key, &value); err != nil {
			return nil, fmt.Errorf("scan public config: %w", err)
		}
		config[key] = rawJSON(value)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate public config: %w", err)
	}
	return config, nil
}

func (s *PostgresStore) WindowState(ctx context.Context, now time.Time) (WindowState, error) {
	values, err := s.configTimes(ctx, "booking_open_at", "booking_lock_at", "show_start_at")
	if err != nil {
		return WindowState{}, err
	}
	state := "open"
	if showStart, ok := values["show_start_at"]; ok && !now.Before(showStart) {
		state = "show_started"
	} else if lockAt, ok := values["booking_lock_at"]; ok && !now.Before(lockAt) {
		state = "locked"
	} else if openAt, ok := values["booking_open_at"]; ok && now.Before(openAt) {
		state = "before_open"
	}
	return WindowState{State: state}, nil
}

func (s *PostgresStore) ExportCSV(ctx context.Context, exportType string) (CSVExport, error) {
	switch exportType {
	case "students":
		return s.exportStudents(ctx)
	case "bookings":
		return s.exportBookings(ctx)
	case "audit":
		return s.exportAudit(ctx)
	case "checkin":
		return s.exportCheckin(ctx)
	default:
		return CSVExport{}, ErrInvalidExportType
	}
}

func (s *PostgresStore) ExportBackup(ctx context.Context, now time.Time) (BackupSnapshot, error) {
	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{
		IsoLevel:   pgx.RepeatableRead,
		AccessMode: pgx.ReadOnly,
	})
	if err != nil {
		return BackupSnapshot{}, fmt.Errorf("begin backup transaction: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	snapshot := BackupSnapshot{
		FormatVersion: 1,
		GeneratedAt:   now.UTC(),
	}

	configs, err := backupConfig(ctx, tx)
	if err != nil {
		return BackupSnapshot{}, err
	}
	snapshot.SystemConfig = configs

	seats, err := backupSeats(ctx, tx)
	if err != nil {
		return BackupSnapshot{}, err
	}
	snapshot.Seats = seats

	students, err := backupStudents(ctx, tx)
	if err != nil {
		return BackupSnapshot{}, err
	}
	snapshot.Students = students

	bookings, err := backupSeatBookings(ctx, tx)
	if err != nil {
		return BackupSnapshot{}, err
	}
	snapshot.SeatBookings = bookings

	checkins, err := backupCheckinEvents(ctx, tx)
	if err != nil {
		return BackupSnapshot{}, err
	}
	snapshot.CheckinEvents = checkins

	audit, err := backupAuditLogs(ctx, tx)
	if err != nil {
		return BackupSnapshot{}, err
	}
	snapshot.AuditLogs = audit
	snapshot.Summary = backupSummary(snapshot)
	if err := tx.Commit(ctx); err != nil {
		return BackupSnapshot{}, fmt.Errorf("commit backup transaction: %w", err)
	}
	return snapshot, nil
}

func (s *PostgresStore) RestoreBackup(ctx context.Context, input RestoreBackupInput) (RestoreResult, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return RestoreResult{}, fmt.Errorf("begin restore transaction: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	for _, stmt := range []string{
		`DELETE FROM seat_bookings`,
		`DELETE FROM checkin_events`,
		`DELETE FROM audit_logs`,
		`DELETE FROM students`,
		`DELETE FROM seats`,
		`DELETE FROM system_config`,
	} {
		if _, err := tx.Exec(ctx, stmt); err != nil {
			return RestoreResult{}, fmt.Errorf("clear restore target: %w", err)
		}
	}

	for _, entry := range input.Snapshot.SystemConfig {
		if _, err := tx.Exec(ctx, `
INSERT INTO system_config (key, value_json, updated_by, updated_at)
VALUES ($1, $2::jsonb, $3, $4)`, entry.Key, string(entry.ValueJSON), entry.UpdatedBy, entry.UpdatedAt); err != nil {
			return RestoreResult{}, fmt.Errorf("restore system_config: %w", err)
		}
	}
	for _, seat := range input.Snapshot.Seats {
		if _, err := tx.Exec(ctx, `
INSERT INTO seats (seat_id, zone, row_no, col_no, is_blocked, block_reason, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`, seat.SeatID, seat.Zone, seat.RowNo, seat.ColNo, seat.IsBlocked, seat.BlockReason, seat.CreatedAt, seat.UpdatedAt); err != nil {
			return RestoreResult{}, fmt.Errorf("restore seats: %w", err)
		}
	}
	for _, studentRow := range input.Snapshot.Students {
		if _, err := tx.Exec(ctx, `
INSERT INTO students (student_id, class_code, seat_no_in_class, display_name, pending_notice, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5::jsonb, $6, $7)`, studentRow.StudentID, studentRow.ClassCode, studentRow.SeatNoInClass, studentRow.DisplayName, nullableJSON(studentRow.PendingNotice), studentRow.CreatedAt, studentRow.UpdatedAt); err != nil {
			return RestoreResult{}, fmt.Errorf("restore students: %w", err)
		}
	}
	for _, booking := range input.Snapshot.SeatBookings {
		if _, err := tx.Exec(ctx, `
INSERT INTO seat_bookings (seat_id, student_id, custom_label, booked_by_student_id, booked_at, ticket_version)
VALUES ($1, $2, $3, $4, $5, $6)`, booking.SeatID, booking.StudentID, booking.CustomLabel, booking.BookedByStudentID, booking.BookedAt, booking.TicketVersion); err != nil {
			return RestoreResult{}, fmt.Errorf("restore seat_bookings: %w", err)
		}
	}
	for _, event := range input.Snapshot.CheckinEvents {
		if _, err := tx.Exec(ctx, `
INSERT INTO checkin_events (id, student_id, seat_id, ticket_version, method, result, is_first_scan, scanner_account, ip_addr, scanned_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9::inet, $10)`, event.ID, event.StudentID, event.SeatID, event.TicketVersion, event.Method, event.Result, event.IsFirstScan, event.ScannerAccount, event.IPAddr, event.ScannedAt); err != nil {
			return RestoreResult{}, fmt.Errorf("restore checkin_events: %w", err)
		}
	}
	for _, audit := range input.Snapshot.AuditLogs {
		if _, err := tx.Exec(ctx, `
INSERT INTO audit_logs (id, actor_type, actor_id, action, target_seat_id, target_student, before_json, after_json, ip_addr, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7::jsonb, $8::jsonb, $9::inet, $10)`, audit.ID, audit.ActorType, audit.ActorID, audit.Action, audit.TargetSeatID, audit.TargetStudent, nullableJSON(audit.Before), nullableJSON(audit.After), audit.IPAddr, audit.CreatedAt); err != nil {
			return RestoreResult{}, fmt.Errorf("restore audit_logs: %w", err)
		}
	}

	for _, stmt := range []string{
		`SELECT setval(pg_get_serial_sequence('seats', 'seat_id'), COALESCE((SELECT max(seat_id) FROM seats), 1), true)`,
		`SELECT setval(pg_get_serial_sequence('checkin_events', 'id'), COALESCE((SELECT max(id) FROM checkin_events), 1), true)`,
		`SELECT setval(pg_get_serial_sequence('audit_logs', 'id'), COALESCE((SELECT max(id) FROM audit_logs), 1), true)`,
	} {
		if _, err := tx.Exec(ctx, stmt); err != nil {
			return RestoreResult{}, fmt.Errorf("reset restore sequences: %w", err)
		}
	}

	after, _ := json.Marshal(map[string]any{
		"restored_by": input.Actor,
		"restored_at": input.Now.UTC().Format(time.RFC3339),
		"summary":     backupSummary(input.Snapshot),
	})
	if err := writeAudit(ctx, tx, "admin_restore", input.Actor, 0, nil, nil, after); err != nil {
		return RestoreResult{}, err
	}
	if _, err := tx.Exec(ctx, `SELECT setval(pg_get_serial_sequence('audit_logs', 'id'), COALESCE((SELECT max(id) FROM audit_logs), 1), true)`); err != nil {
		return RestoreResult{}, fmt.Errorf("reset audit sequence after restore audit: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return RestoreResult{}, fmt.Errorf("commit restore transaction: %w", err)
	}
	return RestoreResult{RestoredAt: input.Now.UTC(), Summary: backupSummary(input.Snapshot)}, nil
}

const seatDetailSQL = `
SELECT se.seat_id,
       se.zone,
       se.row_no,
       se.col_no,
       se.is_blocked,
       se.block_reason,
       b.student_id,
       b.custom_label,
       b.booked_by_student_id,
       st.class_code,
       st.seat_no_in_class,
       st.display_name,
       b.ticket_version,
       b.booked_at,
       st.pending_notice
FROM seats se
LEFT JOIN seat_bookings b ON b.seat_id = se.seat_id
LEFT JOIN students st ON st.student_id = b.student_id
`

type rowScanner interface {
	Scan(dest ...any) error
}

type queryer interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

func scanSeatDetail(row rowScanner) (SeatDetail, error) {
	var (
		seat              SeatDetail
		blockReason       sql.NullString
		studentID         sql.NullString
		customLabel       sql.NullString
		bookedByStudentID sql.NullString
		classCode         sql.NullString
		seatNoInClass     sql.NullInt64
		displayName       sql.NullString
		ticketVersion     sql.NullInt64
		bookedAt          sql.NullTime
		pendingNotice     []byte
	)
	if err := row.Scan(
		&seat.SeatID,
		&seat.Zone,
		&seat.RowNo,
		&seat.ColNo,
		&seat.IsBlocked,
		&blockReason,
		&studentID,
		&customLabel,
		&bookedByStudentID,
		&classCode,
		&seatNoInClass,
		&displayName,
		&ticketVersion,
		&bookedAt,
		&pendingNotice,
	); err != nil {
		return SeatDetail{}, err
	}
	seat.BlockReason = stringPtr(blockReason)
	if studentID.Valid || customLabel.Valid {
		booking := &BookingDetail{
			StudentID:         stringPtr(studentID),
			CustomLabel:       stringPtr(customLabel),
			BookedByStudentID: stringPtr(bookedByStudentID),
			ClassCode:         stringPtr(classCode),
			SeatNoInClass:     intPtr(seatNoInClass),
			DisplayName:       stringPtr(displayName),
		}
		if ticketVersion.Valid {
			booking.TicketVersion = int(ticketVersion.Int64)
		}
		if bookedAt.Valid {
			booking.BookedAt = bookedAt.Time
		}
		if len(pendingNotice) > 0 {
			booking.PendingNotice = rawJSON(pendingNotice)
		}
		seat.Booking = booking
	}
	return seat, nil
}

func lockSeat(ctx context.Context, tx pgx.Tx, seatID int64) error {
	var exists bool
	if err := tx.QueryRow(ctx, `
SELECT true
FROM seats
WHERE seat_id = $1
FOR UPDATE`, seatID).Scan(&exists); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrSeatNotFound
		}
		return fmt.Errorf("lock admin seat: %w", err)
	}
	return nil
}

func getSeatBookingForUpdate(ctx context.Context, tx pgx.Tx, seatID int64) (currentBooking, bool, error) {
	row := tx.QueryRow(ctx, `
SELECT seat_id, student_id, custom_label, booked_by_student_id, ticket_version, booked_at
FROM seat_bookings
WHERE seat_id = $1
FOR UPDATE`, seatID)
	booking, err := scanCurrentBooking(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return currentBooking{}, false, nil
	}
	if err != nil {
		return currentBooking{}, false, fmt.Errorf("get target booking: %w", err)
	}
	return booking, true, nil
}

func getStudentBookingForUpdate(ctx context.Context, tx pgx.Tx, studentID string) (currentBooking, bool, error) {
	row := tx.QueryRow(ctx, `
SELECT seat_id, student_id, custom_label, booked_by_student_id, ticket_version, booked_at
FROM seat_bookings
WHERE student_id = $1
FOR UPDATE`, studentID)
	booking, err := scanCurrentBooking(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return currentBooking{}, false, nil
	}
	if err != nil {
		return currentBooking{}, false, fmt.Errorf("get student booking: %w", err)
	}
	return booking, true, nil
}

func scanCurrentBooking(row rowScanner) (currentBooking, error) {
	var (
		booking           currentBooking
		studentID         sql.NullString
		customLabel       sql.NullString
		bookedByStudentID sql.NullString
	)
	if err := row.Scan(&booking.SeatID, &studentID, &customLabel, &bookedByStudentID, &booking.TicketVersion, &booking.BookedAt); err != nil {
		return currentBooking{}, err
	}
	booking.StudentID = stringPtr(studentID)
	booking.CustomLabel = stringPtr(customLabel)
	booking.BookedByStudentID = stringPtr(bookedByStudentID)
	return booking, nil
}

func deleteBookingBySeat(ctx context.Context, tx pgx.Tx, seatID int64) error {
	if _, err := tx.Exec(ctx, `DELETE FROM seat_bookings WHERE seat_id = $1`, seatID); err != nil {
		return fmt.Errorf("delete admin booking: %w", err)
	}
	return nil
}

func insertAdminBooking(ctx context.Context, tx pgx.Tx, seatID int64, studentID, customLabel string, ticketVersion int) (currentBooking, error) {
	var studentValue any
	var customValue any
	if studentID != "" {
		studentValue = studentID
	}
	if customLabel != "" {
		customValue = customLabel
	}
	row := tx.QueryRow(ctx, `
INSERT INTO seat_bookings (seat_id, student_id, custom_label, ticket_version)
VALUES ($1, $2, $3, $4)
RETURNING seat_id, student_id, custom_label, booked_by_student_id, ticket_version, booked_at`, seatID, studentValue, customValue, ticketVersion)
	booking, err := scanCurrentBooking(row)
	if isUniqueViolation(err) {
		return currentBooking{}, ErrAssignmentConflict
	}
	if err != nil {
		return currentBooking{}, fmt.Errorf("insert admin booking: %w", err)
	}
	return booking, nil
}

func setNoticeForBooking(ctx context.Context, tx pgx.Tx, booking currentBooking, previousSeatID int64, newSeatID *int64, actor string) error {
	if booking.StudentID == nil {
		return nil
	}
	return setStudentNotice(ctx, tx, *booking.StudentID, &previousSeatID, newSeatID, actor)
}

func setStudentNotice(ctx context.Context, tx pgx.Tx, studentID string, previousSeatID, newSeatID *int64, actor string) error {
	notice, err := json.Marshal(map[string]any{
		"type":             "admin_seat_adjustment",
		"previous_seat_id": previousSeatID,
		"new_seat_id":      newSeatID,
		"updated_by":       actor,
		"updated_at":       time.Now().UTC().Format(time.RFC3339),
	})
	if err != nil {
		return fmt.Errorf("marshal pending notice: %w", err)
	}
	if _, err := tx.Exec(ctx, `
UPDATE students
SET pending_notice = $2::jsonb,
    updated_at = now()
WHERE student_id = $1`, studentID, string(notice)); err != nil {
		return fmt.Errorf("set pending notice: %w", err)
	}
	return nil
}

func bookingAuditJSON(booking currentBooking) json.RawMessage {
	payload, _ := json.Marshal(map[string]any{
		"seat_id":              booking.SeatID,
		"student_id":           booking.StudentID,
		"custom_label":         booking.CustomLabel,
		"booked_by_student_id": booking.BookedByStudentID,
		"ticket_version":       booking.TicketVersion,
	})
	return payload
}

func writeAudit(ctx context.Context, tx pgx.Tx, action, actor string, seatID int64, targetStudent *string, before, after json.RawMessage) error {
	var seatValue any
	if seatID > 0 {
		seatValue = seatID
	}
	_, err := tx.Exec(ctx, `
INSERT INTO audit_logs (actor_type, actor_id, action, target_seat_id, target_student, before_json, after_json)
VALUES ('admin', $1, $2, $3, $4, $5::jsonb, $6::jsonb)`, actor, action, seatValue, targetStudent, nullableJSON(before), nullableJSON(after))
	if err != nil {
		return fmt.Errorf("write admin audit: %w", err)
	}
	return nil
}

func (s *PostgresStore) writePoolAudit(ctx context.Context, action, actor string, seatID int64, targetStudent *string, before, after json.RawMessage) error {
	var seatValue any
	if seatID > 0 {
		seatValue = seatID
	}
	_, err := s.db.Exec(ctx, `
INSERT INTO audit_logs (actor_type, actor_id, action, target_seat_id, target_student, before_json, after_json)
VALUES ('admin', $1, $2, $3, $4, $5::jsonb, $6::jsonb)`, actor, action, seatValue, targetStudent, nullableJSON(before), nullableJSON(after))
	if err != nil {
		return fmt.Errorf("write admin audit: %w", err)
	}
	return nil
}

func nullableJSON(value json.RawMessage) any {
	if len(value) == 0 {
		return nil
	}
	return string(value)
}

func scanConfig(row rowScanner) (ConfigEntry, error) {
	var (
		entry     ConfigEntry
		valueJSON []byte
		updatedBy sql.NullString
	)
	if err := row.Scan(&entry.Key, &valueJSON, &updatedBy, &entry.UpdatedAt); err != nil {
		return ConfigEntry{}, err
	}
	entry.ValueJSON = rawJSON(valueJSON)
	entry.UpdatedBy = stringPtr(updatedBy)
	return entry, nil
}

func (s *PostgresStore) configTimes(ctx context.Context, keys ...string) (map[string]time.Time, error) {
	rows, err := s.db.Query(ctx, `
SELECT key, value_json
FROM system_config
WHERE key = ANY($1)`, keys)
	if err != nil {
		return nil, fmt.Errorf("query window config: %w", err)
	}
	defer rows.Close()

	values := map[string]time.Time{}
	for rows.Next() {
		var key string
		var raw []byte
		if err := rows.Scan(&key, &raw); err != nil {
			return nil, fmt.Errorf("scan window config: %w", err)
		}
		value, err := parseConfigTime(raw)
		if err != nil {
			return nil, fmt.Errorf("parse %s config: %w", key, err)
		}
		values[key] = value
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate window config: %w", err)
	}
	return values, nil
}

func parseConfigTime(raw []byte) (time.Time, error) {
	var asString string
	if err := json.Unmarshal(raw, &asString); err == nil {
		return time.Parse(time.RFC3339, asString)
	}
	var envelope struct {
		Value string `json:"value"`
	}
	if err := json.Unmarshal(raw, &envelope); err == nil && envelope.Value != "" {
		return time.Parse(time.RFC3339, envelope.Value)
	}
	return time.Time{}, errors.New("expected RFC3339 string")
}

func (s *PostgresStore) exportStudents(ctx context.Context) (CSVExport, error) {
	rows, err := s.db.Query(ctx, `
SELECT student_id, COALESCE(class_code, ''), seat_no_in_class, COALESCE(display_name, ''), pending_notice IS NOT NULL
FROM students
ORDER BY student_id`)
	if err != nil {
		return CSVExport{}, fmt.Errorf("export students: %w", err)
	}
	defer rows.Close()

	export := CSVExport{Filename: "lalaland-students.csv", Header: []string{"student_id", "class_code", "seat_no_in_class", "display_name", "has_pending_notice"}}
	for rows.Next() {
		var studentID, classCode, displayName string
		var seatNo sql.NullInt64
		var hasNotice bool
		if err := rows.Scan(&studentID, &classCode, &seatNo, &displayName, &hasNotice); err != nil {
			return CSVExport{}, err
		}
		export.Rows = append(export.Rows, []string{studentID, classCode, nullableIntString(seatNo), displayName, strconv.FormatBool(hasNotice)})
	}
	return export, rows.Err()
}

func (s *PostgresStore) exportBookings(ctx context.Context) (CSVExport, error) {
	rows, err := s.db.Query(ctx, `
SELECT b.seat_id, se.zone, se.row_no, se.col_no, COALESCE(b.student_id, ''), COALESCE(b.custom_label, ''), COALESCE(b.booked_by_student_id, ''), b.ticket_version, b.booked_at
FROM seat_bookings b
JOIN seats se ON se.seat_id = b.seat_id
ORDER BY se.zone, se.row_no, se.col_no`)
	if err != nil {
		return CSVExport{}, fmt.Errorf("export bookings: %w", err)
	}
	defer rows.Close()

	export := CSVExport{Filename: "lalaland-bookings.csv", Header: []string{"seat_id", "zone", "row_no", "col_no", "student_id", "custom_label", "booked_by_student_id", "ticket_version", "booked_at"}}
	for rows.Next() {
		var seatID int64
		var zone, studentID, customLabel, bookedBy string
		var rowNo, colNo, version int
		var bookedAt time.Time
		if err := rows.Scan(&seatID, &zone, &rowNo, &colNo, &studentID, &customLabel, &bookedBy, &version, &bookedAt); err != nil {
			return CSVExport{}, err
		}
		export.Rows = append(export.Rows, []string{strconv.FormatInt(seatID, 10), zone, strconv.Itoa(rowNo), strconv.Itoa(colNo), studentID, customLabel, bookedBy, strconv.Itoa(version), bookedAt.UTC().Format(time.RFC3339)})
	}
	return export, rows.Err()
}

func (s *PostgresStore) exportAudit(ctx context.Context) (CSVExport, error) {
	rows, err := s.db.Query(ctx, `
SELECT id, actor_type, COALESCE(actor_id, ''), action, target_seat_id, COALESCE(target_student, ''), created_at
FROM audit_logs
ORDER BY created_at DESC, id DESC`)
	if err != nil {
		return CSVExport{}, fmt.Errorf("export audit: %w", err)
	}
	defer rows.Close()

	export := CSVExport{Filename: "lalaland-audit.csv", Header: []string{"id", "actor_type", "actor_id", "action", "target_seat_id", "target_student", "created_at"}}
	for rows.Next() {
		var id int64
		var actorType, actorID, action, targetStudent string
		var targetSeatID sql.NullInt64
		var createdAt time.Time
		if err := rows.Scan(&id, &actorType, &actorID, &action, &targetSeatID, &targetStudent, &createdAt); err != nil {
			return CSVExport{}, err
		}
		export.Rows = append(export.Rows, []string{strconv.FormatInt(id, 10), actorType, actorID, action, nullableIntString(targetSeatID), targetStudent, createdAt.UTC().Format(time.RFC3339)})
	}
	return export, rows.Err()
}

func (s *PostgresStore) exportCheckin(ctx context.Context) (CSVExport, error) {
	rows, err := s.db.Query(ctx, `
SELECT id, student_id, seat_id, ticket_version, method, result, is_first_scan, COALESCE(scanner_account, ''), scanned_at
FROM checkin_events
ORDER BY scanned_at DESC, id DESC`)
	if err != nil {
		return CSVExport{}, fmt.Errorf("export checkin: %w", err)
	}
	defer rows.Close()

	export := CSVExport{Filename: "lalaland-checkin.csv", Header: []string{"id", "student_id", "seat_id", "ticket_version", "method", "result", "is_first_scan", "scanner_account", "scanned_at"}}
	for rows.Next() {
		var id int64
		var studentID sql.NullString
		var seatID, version sql.NullInt64
		var method, result, scanner string
		var first bool
		var scannedAt time.Time
		if err := rows.Scan(&id, &studentID, &seatID, &version, &method, &result, &first, &scanner, &scannedAt); err != nil {
			return CSVExport{}, err
		}
		export.Rows = append(export.Rows, []string{strconv.FormatInt(id, 10), nullableString(studentID), nullableIntString(seatID), nullableIntString(version), method, result, strconv.FormatBool(first), scanner, scannedAt.UTC().Format(time.RFC3339)})
	}
	return export, rows.Err()
}

func backupConfig(ctx context.Context, db queryer) ([]BackupConfig, error) {
	rows, err := db.Query(ctx, `
SELECT key, value_json, updated_by, updated_at
FROM system_config
ORDER BY key`)
	if err != nil {
		return nil, fmt.Errorf("backup system_config: %w", err)
	}
	defer rows.Close()

	out := []BackupConfig{}
	for rows.Next() {
		var entry BackupConfig
		var raw []byte
		var updatedBy sql.NullString
		if err := rows.Scan(&entry.Key, &raw, &updatedBy, &entry.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan backup system_config: %w", err)
		}
		entry.ValueJSON = rawJSON(raw)
		entry.UpdatedBy = stringPtr(updatedBy)
		out = append(out, entry)
	}
	return out, rows.Err()
}

func backupSeats(ctx context.Context, db queryer) ([]BackupSeat, error) {
	rows, err := db.Query(ctx, `
SELECT seat_id, zone, row_no, col_no, is_blocked, block_reason, created_at, updated_at
FROM seats
ORDER BY seat_id`)
	if err != nil {
		return nil, fmt.Errorf("backup seats: %w", err)
	}
	defer rows.Close()

	out := []BackupSeat{}
	for rows.Next() {
		var seat BackupSeat
		var blockReason sql.NullString
		if err := rows.Scan(&seat.SeatID, &seat.Zone, &seat.RowNo, &seat.ColNo, &seat.IsBlocked, &blockReason, &seat.CreatedAt, &seat.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan backup seat: %w", err)
		}
		seat.BlockReason = stringPtr(blockReason)
		out = append(out, seat)
	}
	return out, rows.Err()
}

func backupStudents(ctx context.Context, db queryer) ([]BackupStudent, error) {
	rows, err := db.Query(ctx, `
SELECT student_id, class_code, seat_no_in_class, display_name, pending_notice, created_at, updated_at
FROM students
ORDER BY student_id`)
	if err != nil {
		return nil, fmt.Errorf("backup students: %w", err)
	}
	defer rows.Close()

	out := []BackupStudent{}
	for rows.Next() {
		var row BackupStudent
		var classCode, displayName sql.NullString
		var seatNo sql.NullInt64
		var notice []byte
		if err := rows.Scan(&row.StudentID, &classCode, &seatNo, &displayName, &notice, &row.CreatedAt, &row.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan backup student: %w", err)
		}
		row.ClassCode = stringPtr(classCode)
		row.SeatNoInClass = intPtr(seatNo)
		row.DisplayName = stringPtr(displayName)
		row.PendingNotice = rawJSON(notice)
		out = append(out, row)
	}
	return out, rows.Err()
}

func backupSeatBookings(ctx context.Context, db queryer) ([]BackupSeatBooking, error) {
	rows, err := db.Query(ctx, `
SELECT seat_id, student_id, custom_label, booked_by_student_id, booked_at, ticket_version
FROM seat_bookings
ORDER BY seat_id`)
	if err != nil {
		return nil, fmt.Errorf("backup seat_bookings: %w", err)
	}
	defer rows.Close()

	out := []BackupSeatBooking{}
	for rows.Next() {
		var row BackupSeatBooking
		var studentID, customLabel, bookedBy sql.NullString
		if err := rows.Scan(&row.SeatID, &studentID, &customLabel, &bookedBy, &row.BookedAt, &row.TicketVersion); err != nil {
			return nil, fmt.Errorf("scan backup seat_booking: %w", err)
		}
		row.StudentID = stringPtr(studentID)
		row.CustomLabel = stringPtr(customLabel)
		row.BookedByStudentID = stringPtr(bookedBy)
		out = append(out, row)
	}
	return out, rows.Err()
}

func backupCheckinEvents(ctx context.Context, db queryer) ([]BackupCheckinEvent, error) {
	rows, err := db.Query(ctx, `
SELECT id, student_id, seat_id, ticket_version, method, result, is_first_scan, scanner_account, ip_addr::text, scanned_at
FROM checkin_events
ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("backup checkin_events: %w", err)
	}
	defer rows.Close()

	out := []BackupCheckinEvent{}
	for rows.Next() {
		var row BackupCheckinEvent
		var studentID, scanner, ip sql.NullString
		var seatID, version sql.NullInt64
		if err := rows.Scan(&row.ID, &studentID, &seatID, &version, &row.Method, &row.Result, &row.IsFirstScan, &scanner, &ip, &row.ScannedAt); err != nil {
			return nil, fmt.Errorf("scan backup checkin_event: %w", err)
		}
		row.StudentID = stringPtr(studentID)
		row.SeatID = int64Ptr(seatID)
		row.TicketVersion = intPtr(version)
		row.ScannerAccount = stringPtr(scanner)
		row.IPAddr = stringPtr(ip)
		out = append(out, row)
	}
	return out, rows.Err()
}

func backupAuditLogs(ctx context.Context, db queryer) ([]BackupAuditLog, error) {
	rows, err := db.Query(ctx, `
SELECT id, actor_type, actor_id, action, target_seat_id, target_student, before_json, after_json, ip_addr::text, created_at
FROM audit_logs
ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("backup audit_logs: %w", err)
	}
	defer rows.Close()

	out := []BackupAuditLog{}
	for rows.Next() {
		var row BackupAuditLog
		var actorID, targetStudent, ip sql.NullString
		var targetSeatID sql.NullInt64
		var beforeJSON, afterJSON []byte
		if err := rows.Scan(&row.ID, &row.ActorType, &actorID, &row.Action, &targetSeatID, &targetStudent, &beforeJSON, &afterJSON, &ip, &row.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan backup audit_log: %w", err)
		}
		row.ActorID = stringPtr(actorID)
		row.TargetSeatID = int64Ptr(targetSeatID)
		row.TargetStudent = stringPtr(targetStudent)
		row.Before = rawJSON(beforeJSON)
		row.After = rawJSON(afterJSON)
		row.IPAddr = stringPtr(ip)
		out = append(out, row)
	}
	return out, rows.Err()
}

func stringPtr(value sql.NullString) *string {
	if !value.Valid {
		return nil
	}
	out := value.String
	return &out
}

func intPtr(value sql.NullInt64) *int {
	if !value.Valid {
		return nil
	}
	out := int(value.Int64)
	return &out
}

func int64Ptr(value sql.NullInt64) *int64 {
	if !value.Valid {
		return nil
	}
	out := value.Int64
	return &out
}

func rawJSON(value []byte) json.RawMessage {
	if len(value) == 0 {
		return nil
	}
	return append(json.RawMessage(nil), value...)
}

func nullableString(value sql.NullString) string {
	if !value.Valid {
		return ""
	}
	return value.String
}

func nullableIntString(value sql.NullInt64) string {
	if !value.Valid {
		return ""
	}
	return strconv.FormatInt(value.Int64, 10)
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == uniqueViolationCode
}
