package store

import (
	"context"
	"encoding/json"

	"siracrm/internal/domain"
	"siracrm/internal/itemviews"
)

func (repository *Repository) ListSectionViews(ctx context.Context, sectionID string) ([]domain.SectionView, error) {
	rows, err := repository.pool.Query(ctx, "SELECT view_id,enabled,config FROM section_item_views WHERE section_id=$1 ORDER BY position,view_id", sectionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	views := itemviews.DefaultViews()
	for rows.Next() {
		var view domain.SectionView
		var config []byte
		if err = rows.Scan(&view.ID, &view.Enabled, &config); err != nil {
			return nil, err
		}
		if err = json.Unmarshal(config, &view.Config); err != nil {
			return nil, err
		}
		views = append(views, view)
	}
	return views, rows.Err()
}

// ReplaceSectionViews atomically replaces optional views. Info is never stored or
// deleted, so every section always returns it even if no optional views exist.
func (repository *Repository) ReplaceSectionViews(ctx context.Context, actor, sectionID string, views []domain.SectionView) error {
	normalized, err := itemviews.Validate(views)
	if err != nil {
		return err
	}
	transaction, err := repository.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = transaction.Rollback(ctx) }()
	var identifier string
	if err = transaction.QueryRow(ctx, "SELECT id FROM sections WHERE id=$1 FOR UPDATE", sectionID).Scan(&identifier); err != nil {
		return classifyMissingRow(err)
	}
	if _, err = transaction.Exec(ctx, "DELETE FROM section_item_views WHERE section_id=$1", sectionID); err != nil {
		return err
	}
	for position, view := range normalized {
		if view.ID == itemviews.Info {
			continue
		}
		config, err := json.Marshal(view.Config)
		if err != nil {
			return err
		}
		if _, err = transaction.Exec(ctx, "INSERT INTO section_item_views(section_id,view_id,enabled,config,position) VALUES($1,$2,$3,$4,$5)", sectionID, view.ID, view.Enabled, config, position); err != nil {
			return err
		}
	}
	if _, err = transaction.Exec(ctx, "INSERT INTO audit(actor,action,section_id) VALUES($1,'configure_item_views',$2)", actor, sectionID); err != nil {
		return err
	}
	return transaction.Commit(ctx)
}
