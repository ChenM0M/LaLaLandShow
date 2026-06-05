package student

import (
	"errors"

	"github.com/gofiber/fiber/v3"

	"lalalandshow/backend/internal/auth"
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
	group := app.Group("/api/student")
	group.Post("/login", h.login)
	group.Post("/register", h.auth.Middleware(), h.register)
	group.Get("/me", h.auth.Middleware(), h.me)
	group.Post("/notice/ack", h.auth.Middleware(), h.acknowledgeNotice)
}

type loginRequest struct {
	StudentID string `json:"student_id"`
}

func (h *Handler) login(c fiber.Ctx) error {
	var req loginRequest
	if err := c.Bind().Body(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid JSON body"})
	}

	result, err := h.service.Login(c.Context(), req.StudentID)
	if err != nil {
		return writeServiceError(c, err)
	}
	return c.JSON(result)
}

func (h *Handler) register(c fiber.Ctx) error {
	studentID, ok := auth.StudentID(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "missing authenticated student"})
	}

	var req RegisterInput
	if err := c.Bind().Body(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid JSON body"})
	}

	student, err := h.service.Register(c.Context(), studentID, req)
	if err != nil {
		return writeServiceError(c, err)
	}
	return c.JSON(fiber.Map{
		"student":       student,
		"need_register": student.NeedRegister,
	})
}

func (h *Handler) me(c fiber.Ctx) error {
	studentID, ok := auth.StudentID(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "missing authenticated student"})
	}

	overview, err := h.service.Me(c.Context(), studentID)
	if err != nil {
		return writeServiceError(c, err)
	}
	return c.JSON(overview)
}

func (h *Handler) acknowledgeNotice(c fiber.Ctx) error {
	studentID, ok := auth.StudentID(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "missing authenticated student"})
	}

	student, err := h.service.AcknowledgeNotice(c.Context(), studentID)
	if err != nil {
		return writeServiceError(c, err)
	}
	return c.JSON(fiber.Map{"student": student})
}

func writeServiceError(c fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, ErrInvalidStudentID), errors.Is(err, ErrInvalidProfile):
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	case errors.Is(err, ErrNotFound):
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": err.Error()})
	case errors.Is(err, ErrAccountBanned):
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": ErrAccountBanned.Error()})
	default:
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "internal server error"})
	}
}
