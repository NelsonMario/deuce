// Package testutil provides a real, ephemeral PostgreSQL instance (via
// testcontainers-go) for integration tests. If Docker is not available in
// the current environment, NewTestPool skips the calling test instead of
// failing the whole suite.
package testutil

import (
	"context"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

// NewTestPool starts a disposable Postgres container, applies all
// migrations, and returns a ready-to-use connection pool. The container and
// pool are torn down automatically via t.Cleanup.
func NewTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()

	if !dockerAvailable() {
		t.Skip("docker is not available; skipping integration test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	container, err := postgres.Run(ctx, "postgres:16-alpine",
		postgres.WithDatabase("deuce_test"),
		postgres.WithUsername("test"),
		postgres.WithPassword("test"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").WithOccurrence(2).WithStartupTimeout(60*time.Second),
		),
	)
	if err != nil {
		t.Skipf("could not start postgres testcontainer (docker unavailable?): %v", err)
	}
	t.Cleanup(func() {
		_ = container.Terminate(context.Background())
	})

	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("get connection string: %v", err)
	}

	if err := applyMigrations(dsn); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("create pool: %v", err)
	}
	t.Cleanup(pool.Close)

	return pool
}

func applyMigrations(dsn string) error {
	// The golang-migrate pgx/v5 driver registers itself under the "pgx5"
	// URL scheme, distinct from the "postgres" scheme pgxpool expects.
	migrateDSN := "pgx5://" + strings.TrimPrefix(strings.TrimPrefix(dsn, "postgres://"), "postgresql://")

	m, err := migrate.New("file://"+migrationsDir(), migrateDSN)
	if err != nil {
		return err
	}
	defer func() { _, _ = m.Close() }()

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return err
	}
	return nil
}

// migrationsDir resolves the repo's migrations/ directory relative to this
// source file, so tests work regardless of the caller's working directory.
func migrationsDir() string {
	_, thisFile, _, _ := runtime.Caller(0)
	return filepath.ToSlash(filepath.Join(filepath.Dir(thisFile), "..", "..", "migrations"))
}

func dockerAvailable() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	provider, err := testcontainers.NewDockerProvider()
	if err != nil {
		return false
	}
	defer provider.Close()
	return provider.Health(ctx) == nil
}
