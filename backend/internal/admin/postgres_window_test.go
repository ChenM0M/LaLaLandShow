package admin

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPostgresWindowStateIntegration(t *testing.T) {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Skip("DATABASE_URL is not set")
	}

	ctx := context.Background()
	db, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect postgres: %v", err)
	}
	defer db.Close()

	store := NewPostgresStore(db)
	now := time.Date(2026, 6, 2, 12, 0, 0, 0, time.UTC)
	defer setWindowConfigTime(t, ctx, db, "booking_open_at", now.Add(-time.Hour))
	defer setWindowConfigTime(t, ctx, db, "booking_lock_at", now.Add(time.Hour))
	defer setWindowConfigTime(t, ctx, db, "show_start_at", now.Add(2*time.Hour))

	tests := []struct {
		name      string
		openAt    time.Time
		lockAt    time.Time
		showAt    time.Time
		at        time.Time
		wantState string
	}{
		{
			name:      "before open",
			openAt:    now.Add(time.Hour),
			lockAt:    now.Add(2 * time.Hour),
			showAt:    now.Add(3 * time.Hour),
			at:        now,
			wantState: "before_open",
		},
		{
			name:      "open at exact open time",
			openAt:    now,
			lockAt:    now.Add(time.Hour),
			showAt:    now.Add(2 * time.Hour),
			at:        now,
			wantState: "open",
		},
		{
			name:      "locked at exact lock time",
			openAt:    now.Add(-time.Hour),
			lockAt:    now,
			showAt:    now.Add(time.Hour),
			at:        now,
			wantState: "locked",
		},
		{
			name:      "show started at exact show time",
			openAt:    now.Add(-2 * time.Hour),
			lockAt:    now.Add(-time.Hour),
			showAt:    now,
			at:        now,
			wantState: "show_started",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setWindowConfigTime(t, ctx, db, "booking_open_at", tt.openAt)
			setWindowConfigTime(t, ctx, db, "booking_lock_at", tt.lockAt)
			setWindowConfigTime(t, ctx, db, "show_start_at", tt.showAt)

			state, err := store.WindowState(ctx, tt.at)
			if err != nil {
				t.Fatalf("WindowState() error = %v", err)
			}
			if state.State != tt.wantState {
				t.Fatalf("WindowState() = %q, want %q", state.State, tt.wantState)
			}
		})
	}
}

func setWindowConfigTime(t *testing.T, ctx context.Context, db *pgxpool.Pool, key string, value time.Time) {
	t.Helper()

	if _, err := db.Exec(ctx, `
INSERT INTO system_config (key, value_json, updated_by, updated_at)
VALUES ($1, to_jsonb($2::text), 'test', now())
ON CONFLICT (key) DO UPDATE
SET value_json = excluded.value_json,
    updated_by = excluded.updated_by,
    updated_at = now()`, key, value.UTC().Format(time.RFC3339)); err != nil {
		t.Fatalf("set config %s: %v", key, err)
	}
}
