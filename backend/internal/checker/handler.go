package checker

import (
	"encoding/csv"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v3"

	"lalalandshow/backend/internal/auth"
	"lalalandshow/backend/internal/student"
	"lalalandshow/backend/internal/ticket"
)

type Handler struct {
	service *Service
	auth    auth.Manager
}

func NewHandler(service *Service, authManager auth.Manager) *Handler {
	return &Handler{
		service: service,
		auth:    authManager,
	}
}

func (h *Handler) RegisterRoutes(app *fiber.App) {
	app.Post("/api/checker/login", h.login)

	group := app.Group("/api/checker", h.auth.StaffMiddleware("checker", "admin"))
	group.Post("/verify-qr", h.verifyQR)
	group.Post("/lookup", h.lookup)
	group.Post("/checkin", h.checkin)
	group.Get("/stats", h.stats)
	group.Get("/export-roster.csv", h.exportRoster)
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type verifyQRRequest struct {
	QRToken string `json:"qr_token"`
}

type lookupRequest struct {
	StudentID string `json:"student_id"`
}

func (h *Handler) login(c fiber.Ctx) error {
	var req loginRequest
	if err := c.Bind().Body(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid JSON body"})
	}

	result, err := h.service.Login(c.Context(), req.Username, req.Password)
	if err != nil {
		return writeServiceError(c, err)
	}
	return c.JSON(result)
}

func (h *Handler) verifyQR(c fiber.Ctx) error {
	var req verifyQRRequest
	if err := c.Bind().Body(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid JSON body"})
	}
	result, err := h.service.VerifyQR(c.Context(), req.QRToken)
	if err != nil {
		return writeServiceError(c, err)
	}
	return c.JSON(result)
}

func (h *Handler) lookup(c fiber.Ctx) error {
	var req lookupRequest
	if err := c.Bind().Body(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid JSON body"})
	}
	result, err := h.service.Lookup(c.Context(), req.StudentID)
	if err != nil {
		return writeServiceError(c, err)
	}
	return c.JSON(result)
}

func (h *Handler) checkin(c fiber.Ctx) error {
	staff, ok := auth.Staff(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "missing authenticated staff"})
	}

	var req CheckInRequest
	if err := c.Bind().Body(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid JSON body"})
	}

	result, err := h.service.CheckIn(c.Context(), req, staff, c.IP())
	if err != nil {
		return writeServiceError(c, err)
	}
	return c.JSON(result)
}

func (h *Handler) stats(c fiber.Ctx) error {
	stats, err := h.service.Stats(c.Context())
	if err != nil {
		return writeServiceError(c, err)
	}
	return c.JSON(stats)
}

func (h *Handler) exportRoster(c fiber.Ctx) error {
	roster, err := h.service.Roster(c.Context())
	if err != nil {
		return writeServiceError(c, err)
	}

	c.Set(fiber.HeaderContentType, "text/csv; charset=utf-8")
	c.Set("Content-Disposition", `attachment; filename="lalaland-checkin-roster.csv"`)
	writer := csv.NewWriter(c.Response().BodyWriter())
	if err := writer.Write([]string{
		"student_id",
		"class_code",
		"seat_no_in_class",
		"display_name",
		"seat_id",
		"zone",
		"row_no",
		"col_no",
		"ticket_version",
		"checked_in",
		"first_scanned_at",
		"last_scanned_at",
		"scan_count",
	}); err != nil {
		return err
	}
	for _, row := range roster {
		if err := writer.Write(rosterRecord(row)); err != nil {
			return err
		}
	}
	writer.Flush()
	return writer.Error()
}

func writeServiceError(c fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, ErrInvalidCredentials):
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": ErrInvalidCredentials.Error()})
	case errors.Is(err, ErrInvalidRole):
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": ErrInvalidRole.Error()})
	case errors.Is(err, ErrInvalidMethod),
		errors.Is(err, ErrMissingQRToken),
		errors.Is(err, student.ErrInvalidStudentID),
		errors.Is(err, ticket.ErrInvalidQR),
		errors.Is(err, ticket.ErrInvalidSecret):
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	case errors.Is(err, ticket.ErrTicketNotFound):
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": ticket.ErrTicketNotFound.Error()})
	case errors.Is(err, ErrStaleTicket):
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": ErrStaleTicket.Error()})
	default:
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "internal server error"})
	}
}

func rosterRecord(row RosterRow) []string {
	seatNo := ""
	if row.SeatNoInClass != nil {
		seatNo = strconv.Itoa(*row.SeatNoInClass)
	}
	firstScannedAt := formatOptionalTime(row.FirstScannedAt)
	lastScannedAt := formatOptionalTime(row.LastScannedAt)
	return []string{
		row.StudentID,
		row.ClassCode,
		seatNo,
		row.DisplayName,
		strconv.FormatInt(row.SeatID, 10),
		row.Zone,
		strconv.Itoa(row.RowNo),
		strconv.Itoa(row.ColNo),
		strconv.Itoa(row.TicketVersion),
		fmt.Sprintf("%t", row.CheckedIn),
		firstScannedAt,
		lastScannedAt,
		strconv.Itoa(row.ScanCount),
	}
}

func formatOptionalTime(value *time.Time) string {
	if value == nil {
		return ""
	}
	return value.UTC().Format(time.RFC3339)
}
