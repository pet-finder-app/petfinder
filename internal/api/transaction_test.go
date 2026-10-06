package api

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
)

type rollbackStub struct {
	pgx.Tx
	err   error
	calls int
	ctx   context.Context
}

func (tx *rollbackStub) Rollback(ctx context.Context) error {
	tx.calls++
	tx.ctx = ctx
	return tx.err
}

func TestRollbackTransaction(t *testing.T) {
	for _, test := range []struct {
		name string
		err  error
		log  bool
	}{
		{"success", nil, false},
		{"already closed", pgx.ErrTxClosed, false},
		{"wrapped closed", fmt.Errorf("rollback: %w", pgx.ErrTxClosed), false},
		{"unexpected failure", errors.New("connection failed"), true},
	} {
		t.Run(test.name, func(t *testing.T) {
			var output bytes.Buffer
			previous := slog.Default()
			slog.SetDefault(slog.New(slog.NewJSONHandler(&output, nil)))
			t.Cleanup(func() { slog.SetDefault(previous) })
			ctx := context.Background()
			tx := &rollbackStub{err: test.err}
			rollbackTransaction(ctx, tx)
			if tx.calls != 1 || tx.ctx != ctx {
				t.Fatal("rollback must run once with the supplied context")
			}
			if logged := strings.Contains(output.String(), "rollback transaction"); logged != test.log {
				t.Fatalf("logged = %v, want %v: %s", logged, test.log, output.String())
			}
		})
	}
}
