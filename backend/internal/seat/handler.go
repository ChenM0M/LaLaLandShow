package seat

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/gofiber/fiber/v3"
	fibersse "github.com/gofiber/fiber/v3/middleware/sse"

	"lalalandshow/backend/internal/auth"
)

type Handler struct {
	store   SnapshotStore
	service *Service
	auth    auth.Manager
	broker  *Broker
}

func NewHandler(store Store, authManager auth.Manager, broker *Broker) *Handler {
	return &Handler{
		store:   store,
		service: NewService(store),
		auth:    authManager,
		broker:  broker,
	}
}

func (h *Handler) RegisterRoutes(app *fiber.App) {
	group := app.Group("/api/seats", h.auth.Middleware())
	group.Get("/snapshot", h.snapshot)
	group.Post("/book", h.book)
	group.Post("/rebook", h.rebook)
	group.Post("/release", h.release)
	group.Post("/change", h.change)
	group.Get("/stream", fibersse.New(fibersse.Config{
		Retry:             2 * time.Second,
		HeartbeatInterval: 5 * time.Second,
		Handler:           h.stream,
	}))
}

func (h *Handler) snapshot(c fiber.Ctx) error {
	studentID, ok := auth.StudentID(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "missing authenticated student"})
	}

	snapshot, err := h.store.GetSnapshot(c.Context(), studentID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "internal server error"})
	}
	return c.JSON(snapshot)
}

func (h *Handler) book(c fiber.Ctx) error {
	studentID, ok := auth.StudentID(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "missing authenticated student"})
	}

	entries, err := parseBookEntries(c.Body())
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid JSON body"})
	}

	result, err := h.service.Book(c.Context(), studentID, entries)
	if err != nil {
		return writeBookingError(c, err)
	}
	for _, booking := range result.Bookings {
		seatID := booking.SeatID
		h.broker.Publish(StreamEvent{
			Type:   "seat_changed",
			SeatID: &seatID,
			State:  StateOthers,
		})
	}
	return c.Status(fiber.StatusCreated).JSON(result)
}

func (h *Handler) release(c fiber.Ctx) error {
	studentID, ok := auth.StudentID(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "missing authenticated student"})
	}

	result, err := h.service.Release(c.Context(), studentID)
	if err != nil {
		return writeBookingError(c, err)
	}
	for _, released := range result.ReleasedBookings {
		seatID := released.SeatID
		h.broker.Publish(StreamEvent{
			Type:   "seat_changed",
			SeatID: &seatID,
			State:  StateFree,
		})
	}
	return c.JSON(result)
}

func (h *Handler) rebook(c fiber.Ctx) error {
	studentID, ok := auth.StudentID(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "missing authenticated student"})
	}

	entries, err := parseBookEntries(c.Body())
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid JSON body"})
	}

	result, err := h.service.Rebook(c.Context(), studentID, entries)
	if err != nil {
		return writeBookingError(c, err)
	}
	for _, released := range result.ReleasedBookings {
		seatID := released.SeatID
		h.broker.Publish(StreamEvent{
			Type:   "seat_changed",
			SeatID: &seatID,
			State:  StateFree,
		})
	}
	for _, booking := range result.Bookings {
		seatID := booking.SeatID
		h.broker.Publish(StreamEvent{
			Type:   "seat_changed",
			SeatID: &seatID,
			State:  StateOthers,
		})
	}
	return c.JSON(result)
}

func (h *Handler) change(c fiber.Ctx) error {
	studentID, ok := auth.StudentID(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "missing authenticated student"})
	}

	var input struct {
		SeatID int64 `json:"seat_id"`
	}
	if err := json.Unmarshal(c.Body(), &input); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid JSON body"})
	}

	result, err := h.service.Change(c.Context(), studentID, input.SeatID)
	if err != nil {
		return writeBookingError(c, err)
	}
	previousSeatID := result.Previous.SeatID
	h.broker.Publish(StreamEvent{
		Type:   "seat_changed",
		SeatID: &previousSeatID,
		State:  StateFree,
	})
	newSeatID := result.Booking.SeatID
	h.broker.Publish(StreamEvent{
		Type:   "seat_changed",
		SeatID: &newSeatID,
		State:  StateOthers,
	})
	return c.JSON(result)
}

func (h *Handler) stream(c fiber.Ctx, stream *fibersse.Stream) error {
	if _, ok := auth.StudentID(c); !ok {
		return stream.Event(fibersse.Event{
			Name: "error",
			Data: fiber.Map{"error": "missing authenticated student"},
		})
	}

	if err := stream.Event(fibersse.Event{
		Name: "ready",
		Data: StreamEvent{Type: "ready"},
	}); err != nil {
		return err
	}

	events, unsubscribe := h.broker.Subscribe()
	defer unsubscribe()

	for {
		select {
		case <-stream.Context().Done():
			return nil
		case event, ok := <-events:
			if !ok {
				return nil
			}
			if err := stream.Event(fibersse.Event{
				Name: event.Type,
				Data: event,
			}); err != nil {
				return err
			}
		}
	}
}

func parseBookEntries(body []byte) ([]BookEntry, error) {
	var entries []BookEntry
	if err := json.Unmarshal(body, &entries); err == nil {
		return entries, nil
	}

	var envelope struct {
		Bookings []BookEntry `json:"bookings"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, err
	}
	if envelope.Bookings == nil {
		return nil, errors.New("missing bookings")
	}
	return envelope.Bookings, nil
}

func writeBookingError(c fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, ErrInvalidBookingBatch),
		errors.Is(err, ErrInvalidBookingStudentID),
		errors.Is(err, ErrInvalidSeatID),
		errors.Is(err, ErrDuplicateSeat),
		errors.Is(err, ErrDuplicateStudent),
		errors.Is(err, ErrMissingSelfBooking),
		errors.Is(err, ErrSameSeatChange):
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": bookingErrorMessage(err)})
	case errors.Is(err, ErrSeatNotFound), errors.Is(err, ErrBookingNotFound):
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": bookingErrorMessage(err)})
	case errors.Is(err, ErrBookingConflict),
		errors.Is(err, ErrBookingNotOpen),
		errors.Is(err, ErrBookingLocked),
		errors.Is(err, ErrSeatLocked),
		errors.Is(err, ErrChangeLocked),
		errors.Is(err, ErrBookingFrozen),
		errors.Is(err, ErrIncompleteRegistration):
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": bookingErrorMessage(err)})
	default:
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "internal server error"})
	}
}

func bookingErrorMessage(err error) string {
	switch {
	case errors.Is(err, ErrInvalidBookingBatch):
		return ErrInvalidBookingBatch.Error()
	case errors.Is(err, ErrInvalidBookingStudentID):
		return ErrInvalidBookingStudentID.Error()
	case errors.Is(err, ErrInvalidSeatID):
		return ErrInvalidSeatID.Error()
	case errors.Is(err, ErrDuplicateSeat):
		return ErrDuplicateSeat.Error()
	case errors.Is(err, ErrDuplicateStudent):
		return ErrDuplicateStudent.Error()
	case errors.Is(err, ErrMissingSelfBooking):
		return ErrMissingSelfBooking.Error()
	case errors.Is(err, ErrSeatNotFound):
		return ErrSeatNotFound.Error()
	case errors.Is(err, ErrSeatLocked):
		return ErrSeatLocked.Error()
	case errors.Is(err, ErrBookingConflict):
		return ErrBookingConflict.Error()
	case errors.Is(err, ErrBookingNotOpen):
		return ErrBookingNotOpen.Error()
	case errors.Is(err, ErrBookingLocked):
		return ErrBookingLocked.Error()
	case errors.Is(err, ErrBookingNotFound):
		return ErrBookingNotFound.Error()
	case errors.Is(err, ErrSameSeatChange):
		return ErrSameSeatChange.Error()
	case errors.Is(err, ErrChangeLocked):
		return ErrChangeLocked.Error()
	case errors.Is(err, ErrBookingFrozen):
		return ErrBookingFrozen.Error()
	case errors.Is(err, ErrIncompleteRegistration):
		return ErrIncompleteRegistration.Error()
	default:
		return err.Error()
	}
}
