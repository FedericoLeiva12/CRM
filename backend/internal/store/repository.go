// Package store implements CRM persistence. HTTP and MCP share this repository.
package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotFound = errors.New("resource does not exist")
var ErrConflict = errors.New("resource already exists or was changed elsewhere")
var ErrForbidden = errors.New("you are not allowed to do this")

type Repository struct{ pool *pgxpool.Pool }

func New(pool *pgxpool.Pool) *Repository                        { return &Repository{pool: pool} }
func (repository *Repository) Health(ctx context.Context) error { return repository.pool.Ping(ctx) }
func classifyDatabaseError(err error) error {
	var postgresError *pgconn.PgError
	if errors.As(err, &postgresError) {
		switch postgresError.Code {
		case "23505":
			return ErrConflict
		case "23503":
			return ErrNotFound
		}
	}
	return err
}
