package ticket

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"strings"
	"time"

	qrcode "github.com/skip2/go-qrcode"
	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"

	"lalalandshow/backend/internal/student"
)

var (
	ErrTicketNotFound     = errors.New("student has no ticket")
	ErrTicketNotAvailable = errors.New("tickets are not available before booking lock time")
)

type Store interface {
	GetTicketRecord(ctx context.Context, studentID string) (Record, error)
	ListTicketRecords(ctx context.Context, studentID string) ([]Record, error)
	GetConfigTime(ctx context.Context, key string) (time.Time, bool, error)
	GetConfigString(ctx context.Context, key string) (string, bool, error)
}

type Service struct {
	store  Store
	secret []byte
	now    func() time.Time
}

type Record struct {
	StudentID      string
	ClassCode      *string
	SeatNoInClass  *int
	DisplayName    *string
	SeatID         int64
	Zone           string
	RowNo          int
	ColNo          int
	TicketVersion  int
	BookedAt       time.Time
	BookingOwnerID *string
}

type Holder struct {
	StudentID     string  `json:"student_id"`
	ClassCode     *string `json:"class_code"`
	SeatNoInClass *int    `json:"seat_no_in_class"`
	DisplayName   *string `json:"display_name"`
}

type TicketSeat struct {
	SeatID int64  `json:"seat_id"`
	Zone   string `json:"zone"`
	RowNo  int    `json:"row_no"`
	ColNo  int    `json:"col_no"`
}

type Ticket struct {
	PlayName          string     `json:"play_name"`
	Venue             string     `json:"venue"`
	ShowStartAt       *time.Time `json:"show_start_at,omitempty"`
	IssuedAt          time.Time  `json:"issued_at"`
	Holder            Holder     `json:"holder"`
	Seat              TicketSeat `json:"seat"`
	TicketVersion     int        `json:"ticket_version"`
	QRPayload         *QRPayload `json:"qr_payload,omitempty"`
	QRToken           string     `json:"qr_token,omitempty"`
	Notice            string     `json:"notice"`
	BookedByStudentID *string    `json:"booked_by_student_id,omitempty"`
	OwnedByViewer     bool       `json:"owned_by_viewer"`
}

type TicketList struct {
	Tickets []Ticket `json:"tickets"`
}

func NewService(store Store, secret string) *Service {
	return &Service{
		store:  store,
		secret: []byte(secret),
		now:    time.Now,
	}
}

func (s *Service) Get(ctx context.Context, studentID string) (Ticket, error) {
	studentID = strings.TrimSpace(studentID)
	if !student.IsValidStudentID(studentID) {
		return Ticket{}, student.ErrInvalidStudentID
	}
	if err := s.ensureAvailable(ctx); err != nil {
		return Ticket{}, err
	}

	record, err := s.store.GetTicketRecord(ctx, studentID)
	if err != nil {
		return Ticket{}, err
	}

	issuedAt := s.now().UTC()
	return s.ticketFromRecord(ctx, record, issuedAt, studentID, true)
}

func (s *Service) List(ctx context.Context, studentID string) (TicketList, error) {
	studentID = strings.TrimSpace(studentID)
	if !student.IsValidStudentID(studentID) {
		return TicketList{}, student.ErrInvalidStudentID
	}
	if err := s.ensureAvailable(ctx); err != nil {
		return TicketList{}, err
	}

	records, err := s.store.ListTicketRecords(ctx, studentID)
	if err != nil {
		return TicketList{}, err
	}
	if len(records) == 0 {
		return TicketList{}, ErrTicketNotFound
	}

	issuedAt := s.now().UTC()
	tickets := make([]Ticket, 0, len(records))
	for _, record := range records {
		ticket, err := s.ticketFromRecord(ctx, record, issuedAt, studentID, record.StudentID == studentID)
		if err != nil {
			return TicketList{}, err
		}
		tickets = append(tickets, ticket)
	}
	return TicketList{Tickets: tickets}, nil
}

func (s *Service) ticketFromRecord(ctx context.Context, record Record, issuedAt time.Time, viewerID string, includeQR bool) (Ticket, error) {
	if includeQR && !recordHasCompleteProfile(record) {
		return Ticket{}, student.ErrIncompleteProfile
	}

	var payload *QRPayload
	var token string
	if includeQR {
		value := newQRPayload(record.StudentID, record.SeatID, record.TicketVersion, issuedAt)
		signed, err := SignQR(value, s.secret)
		if err != nil {
			return Ticket{}, err
		}
		payload = &value
		token = signed
	}

	playName := "La La Land Graduation Play"
	if configured, ok, err := s.store.GetConfigString(ctx, "play_name"); err != nil {
		return Ticket{}, err
	} else if ok && strings.TrimSpace(configured) != "" {
		playName = strings.TrimSpace(configured)
	}

	venue := "School Auditorium"
	if configured, ok, err := s.store.GetConfigString(ctx, "venue"); err != nil {
		return Ticket{}, err
	} else if ok && strings.TrimSpace(configured) != "" {
		venue = strings.TrimSpace(configured)
	}

	var showStartAt *time.Time
	if value, ok, err := s.store.GetConfigTime(ctx, "show_start_at"); err != nil {
		return Ticket{}, err
	} else if ok {
		showStartAt = &value
	}

	return Ticket{
		PlayName:    playName,
		Venue:       venue,
		ShowStartAt: showStartAt,
		IssuedAt:    issuedAt,
		Holder: Holder{
			StudentID:     record.StudentID,
			ClassCode:     record.ClassCode,
			SeatNoInClass: record.SeatNoInClass,
			DisplayName:   record.DisplayName,
		},
		Seat: TicketSeat{
			SeatID: record.SeatID,
			Zone:   record.Zone,
			RowNo:  record.RowNo,
			ColNo:  record.ColNo,
		},
		TicketVersion:     record.TicketVersion,
		QRPayload:         payload,
		QRToken:           token,
		Notice:            "Ticket admits the named holder only.",
		BookedByStudentID: record.BookingOwnerID,
		OwnedByViewer:     record.StudentID == viewerID,
	}, nil
}

func recordHasCompleteProfile(record Record) bool {
	return record.ClassCode != nil &&
		strings.TrimSpace(*record.ClassCode) != "" &&
		record.SeatNoInClass != nil &&
		*record.SeatNoInClass > 0
}

func (s *Service) Image(ctx context.Context, studentID string) ([]byte, error) {
	ticket, err := s.Get(ctx, studentID)
	if err != nil {
		return nil, err
	}
	return renderPNG(ticket)
}

func (s *Service) ensureAvailable(ctx context.Context) error {
	lockAt, ok, err := s.store.GetConfigTime(ctx, "booking_lock_at")
	if err != nil {
		return err
	}
	if ok && s.now().Before(lockAt) {
		return ErrTicketNotAvailable
	}
	return nil
}

func renderPNG(ticket Ticket) ([]byte, error) {
	qrPNG, err := qrcode.Encode(ticket.QRToken, qrcode.Medium, 220)
	if err != nil {
		return nil, fmt.Errorf("encode qr png: %w", err)
	}
	qrImage, err := png.Decode(bytes.NewReader(qrPNG))
	if err != nil {
		return nil, fmt.Errorf("decode qr png: %w", err)
	}

	canvas := image.NewRGBA(image.Rect(0, 0, 1000, 560))
	fill(canvas, color.RGBA{R: 24, G: 21, B: 54, A: 255})
	draw.Draw(canvas, image.Rect(0, 0, 1000, 86), &image.Uniform{C: color.RGBA{R: 249, G: 190, B: 70, A: 255}}, image.Point{}, draw.Src)
	draw.Draw(canvas, image.Rect(0, 86, 1000, 94), &image.Uniform{C: color.RGBA{R: 229, G: 76, B: 106, A: 255}}, image.Point{}, draw.Src)
	draw.Draw(canvas, image.Rect(690, 142, 930, 382), &image.Uniform{C: color.White}, image.Point{}, draw.Src)
	draw.Draw(canvas, image.Rect(700, 152, 920, 372), qrImage, image.Point{}, draw.Over)

	primary := color.RGBA{R: 255, G: 255, B: 255, A: 255}
	gold := color.RGBA{R: 249, G: 190, B: 70, A: 255}
	muted := color.RGBA{R: 210, G: 208, B: 226, A: 255}

	drawText(canvas, 54, 55, ticket.PlayName, color.RGBA{R: 24, G: 21, B: 54, A: 255})
	drawText(canvas, 54, 145, "ADMIT ONE", gold)
	drawText(canvas, 54, 190, "Student: "+ticket.Holder.StudentID, primary)
	drawText(canvas, 54, 226, "Class: "+optionalString(ticket.Holder.ClassCode, "not registered"), muted)
	drawText(canvas, 54, 262, fmt.Sprintf("Class Seat No: %s", optionalInt(ticket.Holder.SeatNoInClass, "not registered")), muted)
	drawText(canvas, 54, 318, fmt.Sprintf("Seat: %s zone, row %d, col %d", ticket.Seat.Zone, ticket.Seat.RowNo, ticket.Seat.ColNo), primary)
	drawText(canvas, 54, 354, fmt.Sprintf("Ticket Version: %d", ticket.TicketVersion), muted)
	drawText(canvas, 54, 410, "Venue: "+ticket.Venue, primary)
	if ticket.ShowStartAt != nil {
		drawText(canvas, 54, 446, "Show Time: "+ticket.ShowStartAt.Local().Format(time.RFC3339), muted)
	}
	drawText(canvas, 54, 512, ticket.Notice, gold)
	drawText(canvas, 710, 418, "Scan at entry", gold)
	drawText(canvas, 710, 454, "Issued: "+ticket.IssuedAt.Local().Format(time.RFC3339), muted)

	var out bytes.Buffer
	if err := png.Encode(&out, canvas); err != nil {
		return nil, fmt.Errorf("encode ticket png: %w", err)
	}
	return out.Bytes(), nil
}

func fill(img draw.Image, c color.Color) {
	draw.Draw(img, img.Bounds(), &image.Uniform{C: c}, image.Point{}, draw.Src)
}

func drawText(img draw.Image, x, y int, label string, c color.Color) {
	d := font.Drawer{
		Dst:  img,
		Src:  image.NewUniform(c),
		Face: basicfont.Face7x13,
		Dot:  fixed.P(x, y),
	}
	d.DrawString(label)
}

func optionalString(value *string, fallback string) string {
	if value == nil || strings.TrimSpace(*value) == "" {
		return fallback
	}
	return *value
}

func optionalInt(value *int, fallback string) string {
	if value == nil {
		return fallback
	}
	return fmt.Sprintf("%d", *value)
}
