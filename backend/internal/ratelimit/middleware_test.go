package ratelimit

import (
	"net/http"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"

	"lalalandshow/backend/internal/config"
)

func TestMiddlewareLimitsGenericRequests(t *testing.T) {
	app := fiber.New()
	app.Use(New(config.RateLimitConfig{
		Window:     time.Minute,
		GenericMax: 1,
		BookingMax: 2,
	}, nil))
	app.Get("/api/example", func(c fiber.Ctx) error {
		return c.SendStatus(fiber.StatusNoContent)
	})

	req, err := http.NewRequest(http.MethodGet, "/api/example", nil)
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("first app.Test() error = %v", err)
	}
	if resp.StatusCode != fiber.StatusNoContent {
		t.Fatalf("first status = %d, want %d", resp.StatusCode, fiber.StatusNoContent)
	}

	req, err = http.NewRequest(http.MethodGet, "/api/example", nil)
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	resp, err = app.Test(req)
	if err != nil {
		t.Fatalf("second app.Test() error = %v", err)
	}
	if resp.StatusCode != fiber.StatusTooManyRequests {
		t.Fatalf("second status = %d, want %d", resp.StatusCode, fiber.StatusTooManyRequests)
	}
}

func TestMiddlewareSkipsHealthz(t *testing.T) {
	app := fiber.New()
	app.Use(New(config.RateLimitConfig{
		Window:     time.Minute,
		GenericMax: 1,
		BookingMax: 1,
	}, nil))
	app.Get("/healthz", func(c fiber.Ctx) error {
		return c.SendStatus(fiber.StatusNoContent)
	})

	for i := 0; i < 3; i++ {
		req, err := http.NewRequest(http.MethodGet, "/healthz", nil)
		if err != nil {
			t.Fatalf("NewRequest() error = %v", err)
		}
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("app.Test() error = %v", err)
		}
		if resp.StatusCode != fiber.StatusNoContent {
			t.Fatalf("status = %d, want %d", resp.StatusCode, fiber.StatusNoContent)
		}
	}
}

func TestMiddlewareLimitsPerPath(t *testing.T) {
	app := fiber.New()
	app.Use(New(config.RateLimitConfig{
		Window:     time.Minute,
		GenericMax: 1,
		BookingMax: 1,
	}, nil))
	app.Get("/api/one", func(c fiber.Ctx) error {
		return c.SendStatus(fiber.StatusNoContent)
	})
	app.Get("/api/two", func(c fiber.Ctx) error {
		return c.SendStatus(fiber.StatusNoContent)
	})

	for _, path := range []string{"/api/one", "/api/two"} {
		req, err := http.NewRequest(http.MethodGet, path, nil)
		if err != nil {
			t.Fatalf("NewRequest(%s) error = %v", path, err)
		}
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("app.Test(%s) error = %v", path, err)
		}
		if resp.StatusCode != fiber.StatusNoContent {
			t.Fatalf("status for %s = %d, want %d", path, resp.StatusCode, fiber.StatusNoContent)
		}
	}
}

func TestMiddlewareUsesBookingBurstLimitForSeatMutations(t *testing.T) {
	app := fiber.New()
	app.Use(New(config.RateLimitConfig{
		Window:     time.Minute,
		GenericMax: 1,
		BookingMax: 2,
	}, nil))
	for _, path := range []string{"/api/seats/book", "/api/seats/rebook", "/api/seats/change"} {
		app.Post(path, func(c fiber.Ctx) error {
			return c.SendStatus(fiber.StatusNoContent)
		})
	}

	for _, path := range []string{"/api/seats/book", "/api/seats/rebook", "/api/seats/change"} {
		for i := 0; i < 2; i++ {
			req, err := http.NewRequest(http.MethodPost, path, nil)
			if err != nil {
				t.Fatalf("NewRequest(%s) error = %v", path, err)
			}
			resp, err := app.Test(req)
			if err != nil {
				t.Fatalf("app.Test(%s) error = %v", path, err)
			}
			if resp.StatusCode != fiber.StatusNoContent {
				t.Fatalf("status for %s request %d = %d, want %d", path, i+1, resp.StatusCode, fiber.StatusNoContent)
			}
		}
	}
}
