package seat

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

func SeedDefaultLayout(ctx context.Context, db *pgxpool.Pool) error {
	defs := DefaultLayout()
	tx, err := db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin seat seed: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	for _, def := range defs {
		if _, err := tx.Exec(ctx, `
INSERT INTO seats (zone, row_no, col_no, is_blocked, block_reason)
VALUES ($1, $2, $3, $4, NULLIF($5, ''))
ON CONFLICT (zone, row_no, col_no) DO NOTHING`,
			def.Zone, def.RowNo, def.ColNo, def.IsBlocked, def.BlockReason,
		); err != nil {
			return fmt.Errorf("insert seat %s/%d/%d: %w", def.Zone, def.RowNo, def.ColNo, err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit seat seed: %w", err)
	}
	return nil
}
