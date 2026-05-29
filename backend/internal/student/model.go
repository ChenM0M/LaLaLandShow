package student

import (
	"encoding/json"
	"time"
)

type Student struct {
	StudentID     string          `json:"student_id"`
	ClassCode     *string         `json:"class_code"`
	SeatNoInClass *int            `json:"seat_no_in_class"`
	DisplayName   *string         `json:"display_name"`
	NeedRegister  bool            `json:"need_register"`
	PendingNotice json.RawMessage `json:"pending_notice,omitempty"`
}

type Booking struct {
	SeatID        int64     `json:"seat_id"`
	Zone          string    `json:"zone"`
	RowNo         int       `json:"row_no"`
	ColNo         int       `json:"col_no"`
	TicketVersion int       `json:"ticket_version"`
	BookedAt      time.Time `json:"booked_at"`
}

type Overview struct {
	Student       Student   `json:"student"`
	Booking       *Booking  `json:"booking"`
	ProxyBookings []Booking `json:"proxy_bookings"`
}

type LoginResult struct {
	Token         string          `json:"token"`
	Student       Student         `json:"student"`
	NeedRegister  bool            `json:"need_register"`
	PendingNotice json.RawMessage `json:"pending_notice,omitempty"`
}

type RegisterInput struct {
	ClassCode     string  `json:"class_code"`
	SeatNoInClass int     `json:"seat_no_in_class"`
	DisplayName   *string `json:"display_name"`
}
