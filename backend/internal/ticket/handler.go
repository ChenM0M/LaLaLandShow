package ticket

import (
	"errors"

	"github.com/gofiber/fiber/v3"

	"lalalandshow/backend/internal/auth"
	"lalalandshow/backend/internal/student"
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
	group := app.Group("/api/ticket", h.auth.Middleware())
	group.Get("/me", h.me)
	group.Get("/me/image", h.image)
}

func (h *Handler) me(c fiber.Ctx) error {
	studentID, ok := auth.StudentID(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "missing authenticated student"})
	}

	result, err := h.service.Get(c.Context(), studentID)
	if err != nil {
		return writeError(c, err)
	}
	return c.JSON(result)
}

func (h *Handler) image(c fiber.Ctx) error {
	studentID, ok := auth.StudentID(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "missing authenticated student"})
	}

	pngBytes, err := h.service.Image(c.Context(), studentID)
	if err != nil {
		return writeError(c, err)
	}
	c.Set(fiber.HeaderContentType, "image/png")
	c.Set("Content-Disposition", `attachment; filename="lalaland-ticket.png"`)
	return c.Send(pngBytes)
}

func writeError(c fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, student.ErrInvalidStudentID):
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": student.ErrInvalidStudentID.Error()})
	case errors.Is(err, ErrTicketNotFound):
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": ErrTicketNotFound.Error()})
	case errors.Is(err, ErrTicketNotAvailable):
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": ErrTicketNotAvailable.Error()})
	default:
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "internal server error"})
	}
}
