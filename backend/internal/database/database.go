// Package database owns PostgreSQL connections and versioned schema migrations.
package database

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"sort"

	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed migrations/*.sql
var migrations embed.FS

func Open(ctx context.Context, connectionURL string) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, connectionURL)
	if err != nil {
		return nil, fmt.Errorf("configure database: %w", err)
	}
	if err = pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("connect to database: %w", err)
	}
	return pool, nil
}

// Migrate serializes startup migrations across replicas. Each file runs exactly once.
func Migrate(ctx context.Context, pool *pgxpool.Pool) error {
	transaction, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = transaction.Rollback(ctx) }()
	// A transaction-scoped advisory lock prevents concurrent schema/bootstrap changes.
	if _, err = transaction.Exec(ctx, "SELECT pg_advisory_xact_lock(73142001)"); err != nil {
		return err
	}
	if _, err = transaction.Exec(ctx, "CREATE TABLE IF NOT EXISTS schema_migrations(version text PRIMARY KEY,applied_at timestamptz NOT NULL DEFAULT now())"); err != nil {
		return err
	}
	paths, err := fs.Glob(migrations, "migrations/*.sql")
	if err != nil {
		return err
	}
	sort.Strings(paths)
	for _, path := range paths {
		var applied bool
		if err = transaction.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version=$1)", path).Scan(&applied); err != nil {
			return err
		}
		if applied {
			continue
		}
		contents, err := migrations.ReadFile(path)
		if err != nil {
			return err
		}
		if _, err = transaction.Exec(ctx, string(contents)); err != nil {
			return fmt.Errorf("migration %s: %w", path, err)
		}
		if _, err = transaction.Exec(ctx, "INSERT INTO schema_migrations(version) VALUES($1)", path); err != nil {
			return err
		}
	}
	return transaction.Commit(ctx)
}
