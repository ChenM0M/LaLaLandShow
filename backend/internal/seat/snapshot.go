package seat

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	StateFree   = "free"
	StateLocked = "locked"
	StateMine   = "mine"
	StateOthers = "others"
)

type Snapshot struct {
	GeneratedAt time.Time      `json:"generated_at"`
	Seats       []SnapshotSeat `json:"seats"`
}

type SnapshotSeat struct {
	SeatID    int64  `json:"seat_id"`
	Zone      string `json:"zone"`
	RowNo     int    `json:"row_no"`
	ColNo     int    `json:"col_no"`
	IsBlocked bool   `json:"is_blocked"`
	State     string `json:"state"`
}

type SnapshotStore interface {
	GetSnapshot(ctx context.Context, studentID string) (Snapshot, error)
}

type PostgresSnapshotStore struct {
	db *pgxpool.Pool
}

func NewPostgresSnapshotStore(db *pgxpool.Pool) *PostgresSnapshotStore {
	return &PostgresSnapshotStore{db: db}
}

func (s *PostgresSnapshotStore) GetSnapshot(ctx context.Context, studentID string) (Snapshot, error) {
	rows, err := s.db.Query(ctx, `
SELECT s.seat_id,
       s.zone,
       s.row_no,
       s.col_no,
       s.is_blocked,
       b.student_id,
       b.custom_label
FROM seats s
LEFT JOIN seat_bookings b ON b.seat_id = s.seat_id
ORDER BY
  CASE s.zone
    WHEN 'left' THEN 1
    WHEN 'middle' THEN 2
    WHEN 'right' THEN 3
    ELSE 4
  END,
  s.row_no,
  s.col_no`)
	if err != nil {
		return Snapshot{}, fmt.Errorf("query seat snapshot: %w", err)
	}
	defer rows.Close()

	snapshot := Snapshot{
		GeneratedAt: time.Now().UTC(),
		Seats:       []SnapshotSeat{},
	}
	for rows.Next() {
		var (
			seat             SnapshotSeat
			bookingStudentID sql.NullString
			customLabel      sql.NullString
		)
		if err := rows.Scan(
			&seat.SeatID,
			&seat.Zone,
			&seat.RowNo,
			&seat.ColNo,
			&seat.IsBlocked,
			&bookingStudentID,
			&customLabel,
		); err != nil {
			return Snapshot{}, fmt.Errorf("scan seat snapshot: %w", err)
		}
		seat.State = classifySnapshotSeat(seat.IsBlocked, bookingStudentID, customLabel, studentID)
		snapshot.Seats = append(snapshot.Seats, seat)
	}
	if err := rows.Err(); err != nil {
		return Snapshot{}, fmt.Errorf("iterate seat snapshot: %w", err)
	}
	return snapshot, nil
}

func classifySnapshotSeat(isBlocked bool, bookingStudentID, customLabel sql.NullString, currentStudentID string) string {
	if isBlocked {
		return StateLocked
	}
	if bookingStudentID.Valid {
		if bookingStudentID.String == currentStudentID {
			return StateMine
		}
		return StateOthers
	}
	if customLabel.Valid {
		return StateOthers
	}
	return StateFree
}
