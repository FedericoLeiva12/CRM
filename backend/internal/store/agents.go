package store

import (
	"context"
	"strings"

	"siracrm/internal/auth/token"
	"siracrm/internal/domain"
)

func (repository *Repository) AgentForToken(ctx context.Context, bearerToken string) (string, error) {
	var agentID string
	err := repository.pool.QueryRow(ctx, "SELECT id FROM agents WHERE token_hash=$1", token.Hash(bearerToken)).Scan(&agentID)
	return agentID, classifyMissingRow(err)
}
func (repository *Repository) CanAccess(ctx context.Context, agentID, sectionID string, access domain.Access) bool {
	var canRead, canWrite bool
	err := repository.pool.QueryRow(ctx, "SELECT p.can_read,p.can_write FROM permissions p JOIN agents a ON a.id=p.agent_id WHERE a.id=$1 AND p.section_id=$2", agentID, sectionID).Scan(&canRead, &canWrite)
	if err != nil {
		return false
	} // Missing grants and database failures both deny access.
	if access == domain.WriteAccess {
		return canWrite
	}
	return canRead
}

// CanManageSchema is independent of section read and write. Missing agents and database failures deny access.
func (repository *Repository) CanManageSchema(ctx context.Context, agentID string) bool {
	var allowed bool
	err := repository.pool.QueryRow(ctx, "SELECT can_manage_schema FROM agents WHERE id=$1", agentID).Scan(&allowed)
	if err != nil {
		return false
	}
	return allowed
}
func (repository *Repository) ListAgents(ctx context.Context) ([]domain.Agent, error) {
	rows, err := repository.pool.Query(ctx, "SELECT id,name,handle,can_manage_schema FROM agents ORDER BY created_at")
	if err != nil {
		return nil, err
	}
	agents := []domain.Agent{}
	for rows.Next() {
		var agent domain.Agent
		if err = rows.Scan(&agent.ID, &agent.Name, &agent.Handle, &agent.ManageSchema); err != nil {
			rows.Close()
			return nil, err
		}
		agents = append(agents, agent)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	for index := range agents {
		permissions, err := repository.agentPermissions(ctx, agents[index].ID)
		if err != nil {
			return nil, err
		}
		agents[index].Permissions = permissions
	}
	return agents, nil
}
func (repository *Repository) agentPermissions(ctx context.Context, agentID string) ([]domain.Permission, error) {
	// Derive rows from sections, not grants, so newly added sections appear immediately.
	rows, err := repository.pool.Query(ctx, `SELECT s.id,coalesce(p.can_read,false),coalesce(p.can_write,false)
 FROM sections s LEFT JOIN permissions p ON p.section_id=s.id AND p.agent_id=$1 ORDER BY s.created_at,s.id`, agentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	permissions := []domain.Permission{}
	for rows.Next() {
		var permission domain.Permission
		if err = rows.Scan(&permission.SectionID, &permission.Read, &permission.Write); err != nil {
			return nil, err
		}
		permissions = append(permissions, permission)
	}
	return permissions, rows.Err()
}

// CreateAgent returns the plaintext token once. Only its digest is persisted.
func (repository *Repository) CreateAgent(ctx context.Context, name string) (agentID, bearerToken string, err error) {
	if strings.TrimSpace(name) == "" || len(name) > 80 {
		return "", "", domain.Invalid("Agent name must contain 1–80 characters")
	}
	agentID, bearerToken = token.New(), "sira_"+token.New()
	_, err = repository.pool.Exec(ctx, "INSERT INTO agents(id,name,token_hash) VALUES($1,$2,$3)", agentID, name, token.Hash(bearerToken))
	if err != nil {
		return "", "", err
	}
	return agentID, bearerToken, nil
}
func (repository *Repository) RevokeAgent(ctx context.Context, agentID string) error {
	result, err := repository.pool.Exec(ctx, "DELETE FROM agents WHERE id=$1", agentID)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
func (repository *Repository) SetPermissions(ctx context.Context, agentID string, permissions []domain.Permission, manageSchema bool) error {
	transaction, err := repository.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = transaction.Rollback(ctx) }()
	if _, err = transaction.Exec(ctx, "UPDATE agents SET can_manage_schema=$2 WHERE id=$1", agentID, manageSchema); err != nil {
		return err
	}
	for _, permission := range permissions {
		_, err = transaction.Exec(ctx, `INSERT INTO permissions(agent_id,section_id,can_read,can_write) VALUES($1,$2,$3,$4)
 ON CONFLICT(agent_id,section_id) DO UPDATE SET can_read=excluded.can_read,can_write=excluded.can_write`, agentID, permission.SectionID, permission.Read, permission.Write)
		if err != nil {
			return classifyDatabaseError(err)
		}
	}
	return transaction.Commit(ctx)
}
