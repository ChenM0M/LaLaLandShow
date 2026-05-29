package ratelimit

import (
	"strconv"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/limiter"
	"github.com/redis/go-redis/v9"

	"lalalandshow/backend/internal/config"
)

func New(cfg config.RateLimitConfig, redisClient *redis.Client) fiber.Handler {
	if cfg.GenericMax <= 0 {
		cfg.GenericMax = 1
	}
	if cfg.BookingMax <= 0 {
		cfg.BookingMax = cfg.GenericMax
	}
	limitConfig := limiter.Config{
		Max:        cfg.GenericMax,
		Expiration: cfg.Window,
		Next: func(c fiber.Ctx) bool {
			path := c.Path()
			return path == "/healthz" || path == "/api/seats/stream"
		},
		MaxFunc: func(c fiber.Ctx) int {
			if c.Path() == "/api/seats/book" {
				return cfg.BookingMax
			}
			return cfg.GenericMax
		},
		LimitReached: func(c fiber.Ctx) error {
			c.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)
			return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
				"error": "rate limit exceeded",
			})
		},
	}
	if redisClient != nil {
		limitConfig.Storage = NewRedisStorage(redisClient, "lalaland:rate:")
	}
	limitConfig.KeyGenerator = func(c fiber.Ctx) string {
		return c.IP() + ":" + c.Method() + ":" + strconv.Itoa(limitConfig.MaxFunc(c))
	}
	return limiter.New(limitConfig)
}
