package admin

import (
	"encoding/csv"
	"encoding/json"
	"errors"
	"strconv"

	"github.com/gofiber/fiber/v3"

	"lalalandshow/backend/internal/auth"
	"lalalandshow/backend/internal/student"
)

type Handler struct {
	service *Service
	auth    auth.Manager
}

func NewHandler(service *Service, authManager auth.Manager) *Handler {
	return &Handler{service: service, auth: authManager}
}

func (h *Handler) RegisterRoutes(app *fiber.App) {
	app.Get("/api/public/config", h.publicConfig)
	app.Get("/api/public/window-state", h.windowState)
	app.Post("/api/admin/login", h.login)

	group := app.Group("/api/admin", h.auth.StaffMiddleware("admin"))
	group.Get("/seats", h.listSeats)
	group.Post("/seats/:id/assign", h.assignSeat)
	group.Post("/seats/:id/lock", h.lockSeat)
	group.Get("/students", h.listStudents)
	group.Get("/audit-logs", h.listAuditLogs)
	group.Get("/checkin-events", h.listCheckinEvents)
	group.Get("/dashboard", h.dashboard)
	group.Get("/config/:key", h.getConfig)
	group.Put("/config/:key", h.setConfig)
	group.Get("/export/:type.csv", h.exportCSV)
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type assignSeatRequest struct {
	StudentID            string `json:"student_id"`
	CustomLabel          string `json:"custom_label"`
	Release              bool   `json:"release"`
	ForceReleaseExisting bool   `json:"force_release_existing"`
}

type lockSeatRequest struct {
	Lock        bool    `json:"lock"`
	BlockReason *string `json:"block_reason"`
}

type configRequest struct {
	ValueJSON json.RawMessage `json:"value_json"`
	Value     json.RawMessage `json:"value"`
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

func (h *Handler) listSeats(c fiber.Ctx) error {
	seats, err := h.service.ListSeats(c.Context())
	if err != nil {
		return writeServiceError(c, err)
	}
	return c.JSON(fiber.Map{"seats": seats})
}

func (h *Handler) assignSeat(c fiber.Ctx) error {
	staff, ok := auth.Staff(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "missing authenticated staff"})
	}
	seatID, err := parseSeatID(c)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": ErrInvalidSeatID.Error()})
	}

	var req assignSeatRequest
	if err := c.Bind().Body(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid JSON body"})
	}
	seat, err := h.service.AssignSeat(c.Context(), AssignSeatInput{
		SeatID:               seatID,
		StudentID:            req.StudentID,
		CustomLabel:          req.CustomLabel,
		Release:              req.Release,
		ForceReleaseExisting: req.ForceReleaseExisting,
		Actor:                staff.Username,
	})
	if err != nil {
		return writeServiceError(c, err)
	}
	return c.JSON(fiber.Map{"seat": seat})
}

func (h *Handler) lockSeat(c fiber.Ctx) error {
	staff, ok := auth.Staff(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "missing authenticated staff"})
	}
	seatID, err := parseSeatID(c)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": ErrInvalidSeatID.Error()})
	}

	var req lockSeatRequest
	if err := c.Bind().Body(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid JSON body"})
	}
	seat, err := h.service.LockSeat(c.Context(), LockSeatInput{
		SeatID:      seatID,
		Locked:      req.Lock,
		BlockReason: req.BlockReason,
		Actor:       staff.Username,
	})
	if err != nil {
		return writeServiceError(c, err)
	}
	return c.JSON(fiber.Map{"seat": seat})
}

func (h *Handler) listStudents(c fiber.Ctx) error {
	list, err := h.service.ListStudents(c.Context(), StudentFilter{
		Page:      queryInt(c, "page", 1),
		PageSize:  queryInt(c, "page_size", 50),
		ClassCode: c.Query("class_code"),
		Search:    c.Query("search"),
	})
	if err != nil {
		return writeServiceError(c, err)
	}
	return c.JSON(list)
}

func (h *Handler) listAuditLogs(c fiber.Ctx) error {
	list, err := h.service.ListAuditLogs(c.Context(), AuditFilter{
		Page:     queryInt(c, "page", 1),
		PageSize: queryInt(c, "page_size", 50),
		Action:   c.Query("action"),
		Actor:    c.Query("actor"),
	})
	if err != nil {
		return writeServiceError(c, err)
	}
	return c.JSON(list)
}

func (h *Handler) listCheckinEvents(c fiber.Ctx) error {
	list, err := h.service.ListCheckinEvents(c.Context(), CheckinFilter{
		Page:      queryInt(c, "page", 1),
		PageSize:  queryInt(c, "page_size", 50),
		StudentID: c.Query("student_id"),
		Method:    c.Query("method"),
	})
	if err != nil {
		return writeServiceError(c, err)
	}
	return c.JSON(list)
}

func (h *Handler) dashboard(c fiber.Ctx) error {
	stats, err := h.service.DashboardStats(c.Context())
	if err != nil {
		return writeServiceError(c, err)
	}
	return c.JSON(stats)
}

func (h *Handler) getConfig(c fiber.Ctx) error {
	entry, err := h.service.GetConfig(c.Context(), c.Params("key"))
	if err != nil {
		return writeServiceError(c, err)
	}
	return c.JSON(entry)
}

func (h *Handler) setConfig(c fiber.Ctx) error {
	staff, ok := auth.Staff(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "missing authenticated staff"})
	}
	var req configRequest
	if err := c.Bind().Body(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid JSON body"})
	}
	value := req.ValueJSON
	if len(value) == 0 {
		value = req.Value
	}
	entry, err := h.service.SetConfig(c.Context(), ConfigInput{
		Key:       c.Params("key"),
		ValueJSON: value,
		Actor:     staff.Username,
	})
	if err != nil {
		return writeServiceError(c, err)
	}
	return c.JSON(entry)
}

func (h *Handler) exportCSV(c fiber.Ctx) error {
	export, err := h.service.ExportCSV(c.Context(), c.Params("type"))
	if err != nil {
		return writeServiceError(c, err)
	}

	c.Set(fiber.HeaderContentType, "text/csv; charset=utf-8")
	c.Set("Content-Disposition", `attachment; filename="`+export.Filename+`"`)
	writer := csv.NewWriter(c.Response().BodyWriter())
	if err := writer.Write(export.Header); err != nil {
		return err
	}
	for _, row := range export.Rows {
		if err := writer.Write(row); err != nil {
			return err
		}
	}
	writer.Flush()
	return writer.Error()
}

func (h *Handler) publicConfig(c fiber.Ctx) error {
	config, err := h.service.PublicConfig(c.Context())
	if err != nil {
		return writeServiceError(c, err)
	}
	return c.JSON(config)
}

func (h *Handler) windowState(c fiber.Ctx) error {
	state, err := h.service.WindowState(c.Context())
	if err != nil {
		return writeServiceError(c, err)
	}
	return c.JSON(state)
}

func parseSeatID(c fiber.Ctx) (int64, error) {
	seatID, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil || seatID <= 0 {
		return 0, ErrInvalidSeatID
	}
	return seatID, nil
}

func queryInt(c fiber.Ctx, key string, fallback int) int {
	value := c.Query(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func writeServiceError(c fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, ErrInvalidCredentials):
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": ErrInvalidCredentials.Error()})
	case errors.Is(err, ErrForbidden):
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": ErrForbidden.Error()})
	case errors.Is(err, ErrInvalidSeatID),
		errors.Is(err, ErrInvalidAssignment),
		errors.Is(err, ErrInvalidConfigKey),
		errors.Is(err, ErrInvalidJSON),
		errors.Is(err, ErrInvalidExportType),
		errors.Is(err, student.ErrInvalidStudentID):
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	case errors.Is(err, ErrSeatNotFound), errors.Is(err, ErrConfigNotFound):
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": err.Error()})
	case errors.Is(err, ErrAssignmentConflict):
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": ErrAssignmentConflict.Error()})
	default:
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "internal server error"})
	}
}
