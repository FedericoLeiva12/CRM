package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"siracrm/internal/auth/token"
	"siracrm/internal/domain"
)

const InviteLifetime = 7 * 24 * time.Hour

func (repository *Repository) ListUsers(ctx context.Context) ([]domain.WorkspaceUser, error) {
	rows, err := repository.pool.Query(ctx, "SELECT id,email,name,role,created_at FROM users ORDER BY created_at,email")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	users := []domain.WorkspaceUser{}
	for rows.Next() {
		var user domain.WorkspaceUser
		if err = rows.Scan(&user.ID, &user.Email, &user.Name, &user.Role, &user.CreatedAt); err != nil {
			return nil, err
		}
		users = append(users, user)
	}
	return users, rows.Err()
}
func (repository *Repository) UserByID(ctx context.Context, userID string) (domain.WorkspaceUser, error) {
	var user domain.WorkspaceUser
	err := repository.pool.QueryRow(ctx, "SELECT id,email,name,role,created_at FROM users WHERE id=$1", userID).Scan(&user.ID, &user.Email, &user.Name, &user.Role, &user.CreatedAt)
	return user, classifyMissingRow(err)
}
func (repository *Repository) ListInvites(ctx context.Context) ([]domain.Invite, error) {
	rows, err := repository.pool.Query(ctx, "SELECT id,email,role,created_at,expires_at FROM invites ORDER BY created_at,email")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	invites := []domain.Invite{}
	for rows.Next() {
		var invite domain.Invite
		if err = rows.Scan(&invite.ID, &invite.Email, &invite.Role, &invite.CreatedAt, &invite.ExpiresAt); err != nil {
			return nil, err
		}
		invites = append(invites, invite)
	}
	return invites, rows.Err()
}
func (repository *Repository) CreateInvite(ctx context.Context, actorID, email, role string) (domain.CreatedInvite, error) {
	if !domain.ValidRole(role) {
		return domain.CreatedInvite{}, domain.Invalid("Choose an administrator or member role")
	}
	transaction, err := repository.pool.Begin(ctx)
	if err != nil {
		return domain.CreatedInvite{}, err
	}
	defer func() { _ = transaction.Rollback(ctx) }()
	var accountExists bool
	if err = transaction.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM users WHERE email=$1)", email).Scan(&accountExists); err != nil {
		return domain.CreatedInvite{}, err
	}
	if accountExists {
		return domain.CreatedInvite{}, domain.Invalid("An account with this email already exists")
	}
	var expiresAt time.Time
	err = transaction.QueryRow(ctx, "SELECT expires_at FROM invites WHERE email=$1 FOR UPDATE", email).Scan(&expiresAt)
	if err == nil {
		if expiresAt.After(time.Now()) {
			return domain.CreatedInvite{}, domain.Invalid("An invitation for this email is already pending")
		}
		// An expired invitation no longer grants access, so a new link can replace it.
		if _, err = transaction.Exec(ctx, "DELETE FROM invites WHERE email=$1", email); err != nil {
			return domain.CreatedInvite{}, err
		}
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return domain.CreatedInvite{}, err
	}
	created := domain.CreatedInvite{Invite: domain.Invite{ID: token.New(), Email: email, Role: role, ExpiresAt: time.Now().Add(InviteLifetime)}, Token: "sira_inv_" + token.New()}
	if _, err = transaction.Exec(ctx, "INSERT INTO invites(id,email,role,token_hash,created_by,expires_at) VALUES($1,$2,$3,$4,$5,$6)", created.ID, created.Email, created.Role, token.Hash(created.Token), actorID, created.ExpiresAt); err != nil {
		return domain.CreatedInvite{}, classifyDatabaseError(err)
	}
	if err = writeAudit(ctx, transaction, "user:"+actorID, "invite_create", email+" "+role); err != nil {
		return domain.CreatedInvite{}, err
	}
	if err = transaction.QueryRow(ctx, "SELECT created_at FROM invites WHERE id=$1", created.ID).Scan(&created.CreatedAt); err != nil {
		return domain.CreatedInvite{}, err
	}
	return created, transaction.Commit(ctx)
}
func (repository *Repository) RevokeInvite(ctx context.Context, actorID, inviteID string) error {
	transaction, err := repository.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = transaction.Rollback(ctx) }()
	var email string
	if err = transaction.QueryRow(ctx, "SELECT email FROM invites WHERE id=$1 FOR UPDATE", inviteID).Scan(&email); err != nil {
		return classifyMissingRow(err)
	}
	if _, err = transaction.Exec(ctx, "DELETE FROM invites WHERE id=$1", inviteID); err != nil {
		return err
	}
	if err = writeAudit(ctx, transaction, "user:"+actorID, "invite_revoke", email); err != nil {
		return err
	}
	return transaction.Commit(ctx)
}
func (repository *Repository) InviteByToken(ctx context.Context, plainToken string) (domain.Invite, error) {
	var invite domain.Invite
	err := repository.pool.QueryRow(ctx, "SELECT id,email,role,created_at,expires_at FROM invites WHERE token_hash=$1", token.Hash(plainToken)).Scan(&invite.ID, &invite.Email, &invite.Role, &invite.CreatedAt, &invite.ExpiresAt)
	if err != nil {
		return domain.Invite{}, classifyMissingRow(err)
	}
	if !invite.ExpiresAt.After(time.Now()) {
		return domain.Invite{}, domain.Invalid("This invitation has expired")
	}
	return invite, nil
}

// AcceptInvite creates the account, signs them in, and consumes the invite in one transaction.
func (repository *Repository) AcceptInvite(ctx context.Context, plainToken, name, passwordHash, sessionToken string, sessionExpires time.Time) (string, error) {
	transaction, err := repository.pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer func() { _ = transaction.Rollback(ctx) }()
	var invite domain.Invite
	err = transaction.QueryRow(ctx, "SELECT id,email,role,created_at,expires_at FROM invites WHERE token_hash=$1 FOR UPDATE", token.Hash(plainToken)).Scan(&invite.ID, &invite.Email, &invite.Role, &invite.CreatedAt, &invite.ExpiresAt)
	if err != nil {
		return "", classifyMissingRow(err)
	}
	if !invite.ExpiresAt.After(time.Now()) {
		return "", domain.Invalid("This invitation has expired")
	}
	userID := token.New()
	if _, err = transaction.Exec(ctx, "INSERT INTO users(id,email,password_hash,name,role) VALUES($1,$2,$3,$4,$5)", userID, invite.Email, passwordHash, name, invite.Role); err != nil {
		return "", classifyDatabaseError(err)
	}
	if _, err = transaction.Exec(ctx, "INSERT INTO sessions(hash,user_id,expires_at) VALUES($1,$2,$3)", token.Hash(sessionToken), userID, sessionExpires); err != nil {
		return "", err
	}
	if _, err = transaction.Exec(ctx, "DELETE FROM invites WHERE id=$1", invite.ID); err != nil {
		return "", err
	}
	if err = writeAudit(ctx, transaction, "user:"+userID, "invite_accept", invite.Email); err != nil {
		return "", err
	}
	return userID, transaction.Commit(ctx)
}
func (repository *Repository) SetUserRole(ctx context.Context, actorID, targetID, role string) error {
	if !domain.ValidRole(role) {
		return domain.Invalid("Choose an administrator or member role")
	}
	transaction, err := repository.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = transaction.Rollback(ctx) }()
	if err = lockAdministrators(ctx, transaction); err != nil {
		return err
	}
	var current domain.WorkspaceUser
	if err = transaction.QueryRow(ctx, "SELECT id,email,name,role,created_at FROM users WHERE id=$1 FOR UPDATE", targetID).Scan(&current.ID, &current.Email, &current.Name, &current.Role, &current.CreatedAt); err != nil {
		return classifyMissingRow(err)
	}
	if current.Role == domain.RoleAdmin && role != domain.RoleAdmin {
		if err = keepAdministrator(ctx, transaction, targetID); err != nil {
			return err
		}
	}
	if _, err = transaction.Exec(ctx, "UPDATE users SET role=$2 WHERE id=$1", targetID, role); err != nil {
		return err
	}
	if err = writeAudit(ctx, transaction, "user:"+actorID, "user_role", current.Email+" "+role); err != nil {
		return err
	}
	return transaction.Commit(ctx)
}
func (repository *Repository) RemoveUser(ctx context.Context, actorID, targetID string) error {
	if actorID == targetID {
		return domain.Invalid("You cannot remove your own account")
	}
	transaction, err := repository.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = transaction.Rollback(ctx) }()
	if err = lockAdministrators(ctx, transaction); err != nil {
		return err
	}
	var current domain.WorkspaceUser
	if err = transaction.QueryRow(ctx, "SELECT id,email,name,role,created_at FROM users WHERE id=$1 FOR UPDATE", targetID).Scan(&current.ID, &current.Email, &current.Name, &current.Role, &current.CreatedAt); err != nil {
		return classifyMissingRow(err)
	}
	if current.Role == domain.RoleAdmin {
		if err = keepAdministrator(ctx, transaction, targetID); err != nil {
			return err
		}
	}
	// Sessions reference users with ON DELETE CASCADE, so removal signs them out immediately.
	if _, err = transaction.Exec(ctx, "DELETE FROM users WHERE id=$1", targetID); err != nil {
		return err
	}
	if err = writeAudit(ctx, transaction, "user:"+actorID, "user_remove", current.Email); err != nil {
		return err
	}
	return transaction.Commit(ctx)
}
func lockAdministrators(ctx context.Context, transaction pgx.Tx) error {
	rows, err := transaction.Query(ctx, "SELECT id FROM users WHERE role='admin' FOR UPDATE")
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
	}
	return rows.Err()
}
func keepAdministrator(ctx context.Context, transaction pgx.Tx, targetID string) error {
	var others int
	if err := transaction.QueryRow(ctx, "SELECT count(*) FROM users WHERE role='admin' AND id<>$1", targetID).Scan(&others); err != nil {
		return err
	}
	if others == 0 {
		return domain.Invalid("The workspace must keep one administrator")
	}
	return nil
}
func writeAudit(ctx context.Context, transaction pgx.Tx, actor, action, detail string) error {
	_, err := transaction.Exec(ctx, "INSERT INTO audit(actor,action,detail) VALUES($1,$2,$3)", actor, action, detail)
	return err
}
