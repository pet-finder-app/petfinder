package api

import (
	"context"
	"errors"
	"log/slog"

	"github.com/jackc/pgx/v5"
)

// rollbackTransaction cleans up uncommitted transactions. A committed or already
// rolled-back transaction returns ErrTxClosed, which is expected in deferred cleanup.
func rollbackTransaction(ctx context.Context, tx pgx.Tx) {
	if err := tx.Rollback(ctx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
		slog.Error("rollback transaction", "error", err)
	}
}
