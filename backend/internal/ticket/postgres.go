package ticket

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresStore struct {
	db *pgxpool.Pool
}

func NewPostgresStore(db *pgxpool.Pool) *PostgresStore {
	return &PostgresStore{db: db}
}

func (s *PostgresStore) GetTicketRecord(ctx context.Context, studentID string) (Record, error) {
	row := s.db.QueryRow(ctx, `
SELECT st.student_id,
       st.class_code,
       st.seat_no_in_class,
       st.display_name,
       b.seat_id,
       se.zone,
       se.row_no,
       se.col_no,
       b.ticket_version,
       b.booked_at,
       b.booked_by_student_id
FROM seat_bookings b
JOIN students st ON st.student_id = b.student_id
JOIN seats se ON se.seat_id = b.seat_id
WHERE b.student_id = $1`, studentID)

	var (
		record         Record
		classCode      sql.NullString
		seatNoInClass  sql.NullInt64
		displayName    sql.NullString
		bookingOwnerID sql.NullString
	)
	if err := row.Scan(
		&record.StudentID,
		&classCode,
		&seatNoInClass,
		&displayName,
		&record.SeatID,
		&record.Zone,
		&record.RowNo,
		&record.ColNo,
		&record.TicketVersion,
		&record.BookedAt,
		&bookingOwnerID,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Record{}, ErrTicketNotFound
		}
		return Record{}, fmt.Errorf("get ticket record: %w", err)
	}
	if classCode.Valid {
		record.ClassCode = &classCode.String
	}
	if seatNoInClass.Valid {
		value := int(seatNoInClass.Int64)
		record.SeatNoInClass = &value
	}
	if displayName.Valid {
		record.DisplayName = &displayName.String
	}
	if bookingOwnerID.Valid {
		record.BookingOwnerID = &bookingOwnerID.String
	}
	return record, nil
}

func (s *PostgresStore) ListTicketRecords(ctx context.Context, studentID string) ([]Record, error) {
	rows, err := s.db.Query(ctx, `
SELECT st.student_id,
       st.class_code,
       st.seat_no_in_class,
       st.display_name,
       b.seat_id,
       se.zone,
       se.row_no,
       se.col_no,
       b.ticket_version,
       b.booked_at,
       b.booked_by_student_id
FROM seat_bookings b
JOIN students st ON st.student_id = b.student_id
JOIN seats se ON se.seat_id = b.seat_id
WHERE b.student_id = $1 OR b.booked_by_student_id = $1
ORDER BY CASE WHEN b.student_id = $1 THEN 0 ELSE 1 END, b.booked_at, b.seat_id`, studentID)
	if err != nil {
		return nil, fmt.Errorf("list ticket records: %w", err)
	}
	defer rows.Close()

	records := make([]Record, 0)
	for rows.Next() {
		var (
			record         Record
			classCode      sql.NullString
			seatNoInClass  sql.NullInt64
			displayName    sql.NullString
			bookingOwnerID sql.NullString
		)
		if err := rows.Scan(
			&record.StudentID,
			&classCode,
			&seatNoInClass,
			&displayName,
			&record.SeatID,
			&record.Zone,
			&record.RowNo,
			&record.ColNo,
			&record.TicketVersion,
			&record.BookedAt,
			&bookingOwnerID,
		); err != nil {
			return nil, fmt.Errorf("scan ticket record: %w", err)
		}
		if classCode.Valid {
			record.ClassCode = &classCode.String
		}
		if seatNoInClass.Valid {
			value := int(seatNoInClass.Int64)
			record.SeatNoInClass = &value
		}
		if displayName.Valid {
			record.DisplayName = &displayName.String
		}
		if bookingOwnerID.Valid {
			record.BookingOwnerID = &bookingOwnerID.String
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate ticket records: %w", err)
	}
	return records, nil
}

func (s *PostgresStore) GetConfigTime(ctx context.Context, key string) (time.Time, bool, error) {
	raw, ok, err := s.getConfig(ctx, key)
	if err != nil || !ok {
		return time.Time{}, ok, err
	}
	value, err := parseConfigTime(raw)
	if err != nil {
		return time.Time{}, false, fmt.Errorf("parse %s config: %w", key, err)
	}
	return value, true, nil
}

func (s *PostgresStore) GetConfigString(ctx context.Context, key string) (string, bool, error) {
	raw, ok, err := s.getConfig(ctx, key)
	if err != nil || !ok {
		return "", ok, err
	}
	value, err := parseConfigString(raw)
	if err != nil {
		return "", false, fmt.Errorf("parse %s config: %w", key, err)
	}
	return value, true, nil
}

func (s *PostgresStore) getConfig(ctx context.Context, key string) ([]byte, bool, error) {
	var raw []byte
	if err := s.db.QueryRow(ctx, `
SELECT value_json
FROM system_config
WHERE key = $1`, key).Scan(&raw); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("read %s config: %w", key, err)
	}
	return raw, true, nil
}

func parseConfigTime(raw []byte) (time.Time, error) {
	value, err := parseConfigString(raw)
	if err != nil {
		return time.Time{}, err
	}
	return time.Parse(time.RFC3339, value)
}

func parseConfigString(raw []byte) (string, error) {
	var asString string
	if err := json.Unmarshal(raw, &asString); err == nil {
		return asString, nil
	}

	var envelope struct {
		Value string `json:"value"`
	}
	if err := json.Unmarshal(raw, &envelope); err == nil && envelope.Value != "" {
		return envelope.Value, nil
	}

	return "", errors.New("expected JSON string")
}
