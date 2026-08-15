package database

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"deuce/backend/internal/database/db"
)

// RunInTx runs fn inside a serializable-safe transaction using pgx's default
// (read committed) isolation plus explicit row locking (SELECT ... FOR
// UPDATE) performed by the callers that need it — this is what actually
// guarantees match-generation and rating-update safety under concurrency.
// On any error the transaction is rolled back; on success it is committed.
func RunInTx(ctx context.Context, pool *pgxpool.Pool, fn func(q *db.Queries) error) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }() // no-op if already committed

	q := db.New(pool).WithTx(tx)
	if err := fn(q); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
}
