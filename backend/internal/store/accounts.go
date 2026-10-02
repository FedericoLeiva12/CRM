package store

import (
	"context"
	"time"

	"siracrm/internal/auth/token"
)

type Credentials struct {
	UserID       string
	PasswordHash string
}

func (repository *Repository) HasUsers(ctx context.Context) (bool, error) {
	var exists bool
	err := repository.pool.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM users)").Scan(&exists)
	return exists, err
}

// BootstrapAdmin rechecks under a lock so two first-start replicas cannot create two administrators.
func (repository *Repository) BootstrapAdmin(ctx context.Context, email, passwordHash string) error {
	transaction, err := repository.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = transaction.Rollback(ctx) }()
	if _, err = transaction.Exec(ctx, "SELECT pg_advisory_xact_lock(73142002)"); err != nil {
		return err
	}
	var exists bool
	if err = transaction.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM users)").Scan(&exists); err != nil {
		return err
	}
	if !exists {
		if _, err = transaction.Exec(ctx, "INSERT INTO users(id,email,password_hash,role) VALUES($1,$2,$3,'admin')", token.New(), email, passwordHash); err != nil {
			return err
		}
	}
	return transaction.Commit(ctx)
}
func (repository *Repository) CredentialsByEmail(ctx context.Context, email string) (Credentials, error) {
	var credentials Credentials
	err := repository.pool.QueryRow(ctx, "SELECT id,password_hash FROM users WHERE email=$1", email).Scan(&credentials.UserID, &credentials.PasswordHash)
	return credentials, classifyMissingRow(err)
}
func (repository *Repository) CredentialsByID(ctx context.Context, userID string) (Credentials, error) {
	var credentials Credentials
	err := repository.pool.QueryRow(ctx, "SELECT id,password_hash FROM users WHERE id=$1", userID).Scan(&credentials.UserID, &credentials.PasswordHash)
	return credentials, classifyMissingRow(err)
}
func (repository *Repository) CreateSession(ctx context.Context, userID, sessionToken string, expiresAt time.Time) error {
	_, err := repository.pool.Exec(ctx, "INSERT INTO sessions(hash,user_id,expires_at) VALUES($1,$2,$3)", token.Hash(sessionToken), userID, expiresAt)
	return err
}

type Identity struct {
	UserID string
	Role   string
}

func (repository *Repository) SessionUser(ctx context.Context, sessionToken string) (Identity, error) {
	var identity Identity
	err := repository.pool.QueryRow(ctx, "SELECT s.user_id,u.role FROM sessions s JOIN users u ON u.id=s.user_id WHERE s.hash=$1 AND s.expires_at>now()", token.Hash(sessionToken)).Scan(&identity.UserID, &identity.Role)
	return identity, classifyMissingRow(err)
}
func (repository *Repository) DeleteSession(ctx context.Context, sessionToken string) error {
	_, err := repository.pool.Exec(ctx, "DELETE FROM sessions WHERE hash=$1", token.Hash(sessionToken))
	return err
}
func (repository *Repository) ReplacePassword(ctx context.Context, userID, previousHash, newHash string) error {
	transaction, err := repository.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = transaction.Rollback(ctx) }()
	// Compare-and-swap prevents two concurrent password changes from overwriting each other.
	result, err := transaction.Exec(ctx, "UPDATE users SET password_hash=$1 WHERE id=$2 AND password_hash=$3", newHash, userID, previousHash)
	if err != nil {
		return err
	}
	if result.RowsAffected() != 1 {
		return ErrConflict
	}
	if _, err = transaction.Exec(ctx, "DELETE FROM sessions WHERE user_id=$1", userID); err != nil {
		return err
	}
	return transaction.Commit(ctx)
}
