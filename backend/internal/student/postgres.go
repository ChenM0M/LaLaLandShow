package student

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

func (s *PostgresStore) FindOrCreateStudent(ctx context.Context, studentID string) (Student, error) {
	if _, err := s.db.Exec(ctx, `
INSERT INTO students (student_id)
VALUES ($1)
ON CONFLICT (student_id) DO NOTHING`, studentID); err != nil {
		return Student{}, fmt.Errorf("insert student: %w", err)
	}
	return s.getStudent(ctx, studentID)
}

func (s *PostgresStore) UpdateStudentProfile(ctx context.Context, studentID string, input RegisterInput) (Student, error) {
	row := s.db.QueryRow(ctx, `
UPDATE students
SET class_code = $2,
    seat_no_in_class = $3,
    display_name = $4,
    updated_at = now()
WHERE student_id = $1
RETURNING student_id, class_code, seat_no_in_class, display_name, pending_notice`,
		studentID,
		input.ClassCode,
		input.SeatNoInClass,
		input.DisplayName,
	)
	return scanStudent(row)
}

func (s *PostgresStore) GetOverview(ctx context.Context, studentID string) (Overview, error) {
	student, err := s.getStudent(ctx, studentID)
	if err != nil {
		return Overview{}, err
	}

	booking, err := s.getOwnBooking(ctx, studentID)
	if err != nil {
		return Overview{}, err
	}
	proxyBookings, err := s.getProxyBookings(ctx, studentID)
	if err != nil {
		return Overview{}, err
	}

	return Overview{
		Student:       student,
		Booking:       booking,
		ProxyBookings: proxyBookings,
	}, nil
}

func (s *PostgresStore) ClearPendingNotice(ctx context.Context, studentID string) (Student, error) {
	row := s.db.QueryRow(ctx, `
UPDATE students
SET pending_notice = NULL,
    updated_at = now()
WHERE student_id = $1
RETURNING student_id, class_code, seat_no_in_class, display_name, pending_notice`, studentID)
	return scanStudent(row)
}

func (s *PostgresStore) getStudent(ctx context.Context, studentID string) (Student, error) {
	row := s.db.QueryRow(ctx, `
SELECT student_id, class_code, seat_no_in_class, display_name, pending_notice
FROM students
WHERE student_id = $1`, studentID)
	return scanStudent(row)
}

func (s *PostgresStore) getOwnBooking(ctx context.Context, studentID string) (*Booking, error) {
	row := s.db.QueryRow(ctx, `
SELECT b.seat_id, b.student_id, s.zone, s.row_no, s.col_no, b.ticket_version, b.booked_at
FROM seat_bookings b
JOIN seats s ON s.seat_id = b.seat_id
WHERE b.student_id = $1`, studentID)

	booking, err := scanBooking(row)
	if errors.Is(err, ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &booking, nil
}

func (s *PostgresStore) getProxyBookings(ctx context.Context, studentID string) ([]Booking, error) {
	rows, err := s.db.Query(ctx, `
SELECT b.seat_id, b.student_id, s.zone, s.row_no, s.col_no, b.ticket_version, b.booked_at
FROM seat_bookings b
JOIN seats s ON s.seat_id = b.seat_id
WHERE b.booked_by_student_id = $1
  AND b.student_id IS DISTINCT FROM $1
ORDER BY b.booked_at, b.seat_id`, studentID)
	if err != nil {
		return nil, fmt.Errorf("query proxy bookings: %w", err)
	}
	defer rows.Close()

	var bookings []Booking
	for rows.Next() {
		booking, err := scanBooking(rows)
		if err != nil {
			return nil, err
		}
		bookings = append(bookings, booking)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("scan proxy bookings: %w", err)
	}
	if bookings == nil {
		bookings = []Booking{}
	}
	return bookings, nil
}

type studentScanner interface {
	Scan(dest ...any) error
}

func scanStudent(row studentScanner) (Student, error) {
	var (
		studentID     string
		classCode     sql.NullString
		seatNoInClass sql.NullInt64
		displayName   sql.NullString
		pendingNotice []byte
	)
	if err := row.Scan(&studentID, &classCode, &seatNoInClass, &displayName, &pendingNotice); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Student{}, ErrNotFound
		}
		return Student{}, fmt.Errorf("scan student: %w", err)
	}

	student := Student{
		StudentID: studentID,
	}
	if classCode.Valid {
		student.ClassCode = &classCode.String
	}
	if seatNoInClass.Valid {
		seatNo := int(seatNoInClass.Int64)
		student.SeatNoInClass = &seatNo
	}
	if displayName.Valid {
		student.DisplayName = &displayName.String
	}
	if len(pendingNotice) > 0 {
		student.PendingNotice = json.RawMessage(append([]byte(nil), pendingNotice...))
	}
	student.NeedRegister = student.ClassCode == nil || student.SeatNoInClass == nil
	return student, nil
}

type bookingScanner interface {
	Scan(dest ...any) error
}

func scanBooking(row bookingScanner) (Booking, error) {
	var booking Booking
	var bookedAt time.Time
	if err := row.Scan(
		&booking.SeatID,
		&booking.StudentID,
		&booking.Zone,
		&booking.RowNo,
		&booking.ColNo,
		&booking.TicketVersion,
		&bookedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Booking{}, ErrNotFound
		}
		return Booking{}, fmt.Errorf("scan booking: %w", err)
	}
	booking.BookedAt = bookedAt
	return booking, nil
}
