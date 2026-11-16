package whatsapp

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net/url"

	_ "github.com/jackc/pgx/v5/stdlib"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/store/sqlstore"
	waLog "go.mau.fi/whatsmeow/util/log"
)

// InitStore initializes the whatsmeow PostgreSQL storage container.
// It uses the standard database/sql driver with pgx, running migrations
// in its own isolated search path if needed.
func InitStore(ctx context.Context, dbURL string, logger *slog.Logger) (*sqlstore.Container, *store.Device, error) {
	storeURL, err := whatsmeowDatabaseURL(dbURL)
	if err != nil {
		return nil, nil, err
	}
	db, err := sql.Open("pgx", storeURL)
	if err != nil {
		return nil, nil, fmt.Errorf("open sql db: %w", err)
	}

	// Ensure whatsmeow has its own schema
	if _, err := db.ExecContext(ctx, "CREATE SCHEMA IF NOT EXISTS whatsmeow;"); err != nil {
		db.Close()
		return nil, nil, fmt.Errorf("create whatsmeow schema: %w", err)
	}

	waLogger := waLog.Stdout("whatsmeow", "INFO", true)
	container := sqlstore.NewWithDB(db, "postgres", waLogger)

	// In whatsmeow, sqlstore.NewWithDB requires calling Upgrade explicitly
	if err := container.Upgrade(ctx); err != nil {
		container.Close()
		return nil, nil, fmt.Errorf("upgrade whatsmeow schema: %w", err)
	}

	deviceStore, err := container.GetFirstDevice(ctx)
	if err != nil {
		container.Close()
		return nil, nil, fmt.Errorf("get device store: %w", err)
	}

	return container, deviceStore, nil
}

// whatsmeowDatabaseURL keeps third-party tables out of Deuce's public schema.
func whatsmeowDatabaseURL(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("parse database url: %w", err)
	}
	q := u.Query()
	q.Set("search_path", "whatsmeow,public")
	u.RawQuery = q.Encode()
	return u.String(), nil
}
