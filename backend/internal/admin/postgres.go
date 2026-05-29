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
		if _, err := tx.Exec(ctx, `
INSERT INTO students (student_id)
VALUES ($1)
ON CONFLICT (student_id) DO NOTHING`, input.StudentID); err != nil {
			return SeatDetail{}, fmt.Errorf("ensure assigned student: %w", err)
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
  WHERE ($1 = '' OR st.class_code = $1)
    AND ($2 = '' OR st.student_id ILIKE '%' || $2 || '%' OR st.display_name ILIKE '%' || $2 || '%')
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
LIMIT $3 OFFSET $4`, filter.ClassCode, filter.Search, filter.PageSize, offset)
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
