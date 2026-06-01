package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/recover"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"lalalandshow/backend/internal/admin"
	"lalalandshow/backend/internal/auth"
	"lalalandshow/backend/internal/checker"
	"lalalandshow/backend/internal/config"
	"lalalandshow/backend/internal/migrate"
	"lalalandshow/backend/internal/ratelimit"
	"lalalandshow/backend/internal/seat"
	"lalalandshow/backend/internal/student"
	"lalalandshow/backend/internal/ticket"
)

func main() {
	ctx := context.Background()

	cfg := config.Load()
	if err := cfg.Validate(); err != nil {
		log.Fatalf("invalid config: %v", err)
	}

	logger, err := zap.NewProduction()
	if err != nil {
		log.Fatalf("create logger: %v", err)
	}
	defer func() {
		_ = logger.Sync()
	}()

	var db *pgxpool.Pool
	if cfg.DatabaseURL != "" {
		dbConfig, err := pgxpool.ParseConfig(cfg.DatabaseURL)
		if err != nil {
			logger.Fatal("parse postgres config", zap.Error(err))
		}
		dbConfig.MaxConns = cfg.Database.MaxConns
		dbConfig.MinConns = cfg.Database.MinConns
		dbConfig.MaxConnLifetime = cfg.Database.MaxConnLifetime
		dbConfig.MaxConnIdleTime = cfg.Database.MaxConnIdleTime

		db, err = pgxpool.NewWithConfig(ctx, dbConfig)
		if err != nil {
			logger.Fatal("connect postgres", zap.Error(err))
		}
		defer db.Close()

		if cfg.RunMigrations {
			if err := migrate.Up(ctx, db, cfg.MigrationsDir); err != nil {
				logger.Fatal("run migrations", zap.Error(err))
			}
		}
		if cfg.SeedSeats {
			if err := seat.SeedDefaultLayout(ctx, db); err != nil {
				logger.Fatal("seed seats", zap.Error(err))
			}
		}
		staffSeeds := make([]checker.AccountSeed, 0, len(cfg.StaffAccounts))
		for _, account := range cfg.StaffAccounts {
			staffSeeds = append(staffSeeds, checker.AccountSeed{
				Username: account.Username,
				Password: account.Password,
				Role:     account.Role,
			})
		}
		if err := checker.SeedAccounts(ctx, db, staffSeeds); err != nil {
			logger.Fatal("seed staff accounts", zap.Error(err))
		}
	}

	var redisClient *redis.Client
	if cfg.RedisURL != "" {
		redisOptions, err := redis.ParseURL(cfg.RedisURL)
		if err != nil {
			logger.Fatal("parse redis url", zap.Error(err))
		}
		redisClient = redis.NewClient(redisOptions)
		defer func() {
			_ = redisClient.Close()
		}()
		if err := redisClient.Ping(ctx).Err(); err != nil {
			logger.Fatal("connect redis", zap.Error(err))
		}
	}

	app := fiber.New(fiber.Config{
		AppName:      "LaLaLandShow API",
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	})
	app.Use(recover.New())
	app.Use(requestLogger(logger))
	if cfg.RateLimit.Enabled {
		app.Use(ratelimit.New(cfg.RateLimit, redisClient))
	}

	app.Get("/healthz", func(c fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"status":  "ok",
			"service": "lalalandshow-backend",
		})
	})

	if db != nil {
		authManager := auth.NewManager(cfg.JWTSecret, cfg.JWTTTL)
		studentStore := student.NewPostgresStore(db)
		studentService := student.NewService(studentStore, authManager)
		student.NewHandler(studentService, authManager).RegisterRoutes(app)

		seatStore := seat.NewPostgresSnapshotStore(db)
		seatBroker := seat.NewBroker()
		seat.NewHandler(seatStore, authManager, seatBroker).RegisterRoutes(app)

		ticketStore := ticket.NewPostgresStore(db)
		ticketService := ticket.NewService(ticketStore, cfg.HMACSecret)
		ticket.NewHandler(ticketService, authManager).RegisterRoutes(app)

		checkerStore := checker.NewPostgresStore(db)
		checkerService := checker.NewService(checkerStore, authManager, cfg.HMACSecret)
		checker.NewHandler(checkerService, authManager).RegisterRoutes(app)

		adminStore := admin.NewPostgresStore(db)
		adminService := admin.NewService(adminStore, authManager, cfg.HMACSecret)
		admin.NewHandler(adminService, authManager).RegisterRoutes(app)
	}

	serverErr := make(chan error, 1)
	go func() {
		serverErr <- app.Listen(cfg.HTTPAddr)
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	select {
	case sig := <-stop:
		logger.Info("shutdown signal received", zap.String("signal", sig.String()))
	case err := <-serverErr:
		logger.Fatal("listen", zap.Error(err))
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := app.ShutdownWithContext(shutdownCtx); err != nil {
		logger.Error("shutdown", zap.Error(err))
	}
}

func requestLogger(logger *zap.Logger) fiber.Handler {
	return func(c fiber.Ctx) error {
		start := time.Now()
		err := c.Next()
		status := c.Response().StatusCode()
		fields := []zap.Field{
			zap.String("method", c.Method()),
			zap.String("path", c.Path()),
			zap.Int("status", status),
			zap.Duration("latency", time.Since(start)),
			zap.String("ip", c.IP()),
		}
		if err != nil {
			fields = append(fields, zap.Error(err))
			logger.Warn("http request failed", fields...)
			return err
		}
		if status >= fiber.StatusInternalServerError {
			logger.Error("http request completed", fields...)
		} else {
			logger.Info("http request completed", fields...)
		}
		return nil
	}
}
