package auth

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
)

func TestManagerGenerateAndParseStudentToken(t *testing.T) {
	now := time.Unix(1000, 0)
	manager := NewManager("test-secret", time.Hour)

	token, err := manager.GenerateStudentToken("1234567", now)
	if err != nil {
		t.Fatalf("GenerateStudentToken() error = %v", err)
	}

	claims, err := manager.ParseStudentToken(token, now.Add(30*time.Minute))
	if err != nil {
		t.Fatalf("ParseStudentToken() error = %v", err)
	}
	if claims.Subject != "1234567" {
		t.Fatalf("claims.Subject = %q, want 1234567", claims.Subject)
	}
	if claims.Role != "student" {
		t.Fatalf("claims.Role = %q, want student", claims.Role)
	}
}

func TestManagerGenerateAndParseStaffToken(t *testing.T) {
	now := time.Unix(1000, 0)
	manager := NewManager("test-secret", time.Hour)

	token, err := manager.GenerateStaffToken("checker", "checker", now)
	if err != nil {
		t.Fatalf("GenerateStaffToken() error = %v", err)
	}

	claims, err := manager.ParseStaffToken(token, now.Add(30*time.Minute), "checker", "admin")
	if err != nil {
		t.Fatalf("ParseStaffToken() error = %v", err)
	}
	if claims.Subject != "checker" {
		t.Fatalf("claims.Subject = %q, want checker", claims.Subject)
	}
	if claims.Role != "checker" {
		t.Fatalf("claims.Role = %q, want checker", claims.Role)
	}
}

func TestManagerRejectsDisallowedStaffRole(t *testing.T) {
	now := time.Unix(1000, 0)
	manager := NewManager("test-secret", time.Hour)

	token, err := manager.GenerateStaffToken("checker", "checker", now)
	if err != nil {
		t.Fatalf("GenerateStaffToken() error = %v", err)
	}

	if _, err := manager.ParseStaffToken(token, now, "admin"); err != ErrInvalidToken {
		t.Fatalf("ParseStaffToken(disallowed) error = %v, want %v", err, ErrInvalidToken)
	}
}

func TestManagerRejectsTamperedToken(t *testing.T) {
	now := time.Unix(1000, 0)
	manager := NewManager("test-secret", time.Hour)

	token, err := manager.GenerateStudentToken("1234567", now)
	if err != nil {
		t.Fatalf("GenerateStudentToken() error = %v", err)
	}
	tampered := strings.TrimSuffix(token, token[len(token)-1:]) + "x"

	if _, err := manager.ParseStudentToken(tampered, now); err != ErrInvalidToken {
		t.Fatalf("ParseStudentToken(tampered) error = %v, want %v", err, ErrInvalidToken)
	}
}

func TestManagerRejectsExpiredToken(t *testing.T) {
	now := time.Unix(1000, 0)
	manager := NewManager("test-secret", time.Hour)

	token, err := manager.GenerateStudentToken("1234567", now)
	if err != nil {
		t.Fatalf("GenerateStudentToken() error = %v", err)
	}

	if _, err := manager.ParseStudentToken(token, now.Add(time.Hour)); err != ErrExpiredToken {
		t.Fatalf("ParseStudentToken(expired) error = %v, want %v", err, ErrExpiredToken)
	}
}

func TestStudentMiddlewareAcceptsStreamQueryToken(t *testing.T) {
	manager := NewManager("test-secret", time.Hour)
	token, err := manager.GenerateStudentToken("1234567", time.Now())
	if err != nil {
		t.Fatalf("GenerateStudentToken() error = %v", err)
	}

	app := fiber.New()
	app.Get("/api/seats/stream", manager.Middleware(), func(c fiber.Ctx) error {
		studentID, ok := StudentID(c)
		if !ok {
			return c.SendStatus(fiber.StatusUnauthorized)
		}
		return c.SendString(studentID)
	})

	req, err := http.NewRequest(http.MethodGet, "/api/seats/stream?token="+token, nil)
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test() error = %v", err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, fiber.StatusOK)
	}
}

func TestStudentMiddlewareRejectsQueryTokenOutsideStream(t *testing.T) {
	manager := NewManager("test-secret", time.Hour)
	token, err := manager.GenerateStudentToken("1234567", time.Now())
	if err != nil {
		t.Fatalf("GenerateStudentToken() error = %v", err)
	}

	app := fiber.New()
	app.Get("/api/seats/snapshot", manager.Middleware(), func(c fiber.Ctx) error {
		return c.SendStatus(fiber.StatusNoContent)
	})

	req, err := http.NewRequest(http.MethodGet, "/api/seats/snapshot?token="+token, nil)
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test() error = %v", err)
	}
	if resp.StatusCode != fiber.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", resp.StatusCode, fiber.StatusUnauthorized)
	}
}
