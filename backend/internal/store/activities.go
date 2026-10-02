package store

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"siracrm/internal/auth/token"
	"siracrm/internal/domain"
)

func authorFromActor(actor string) (domain.Author, error) {
	kind, identifier, ok := strings.Cut(actor, ":")
	if !ok || identifier == "" || (kind != "user" && kind != "agent") {
		return domain.Author{}, fmt.Errorf("invalid actor")
	}
	return domain.Author{Kind: kind, ID: identifier}, nil
}

func insertActivity(ctx context.Context, tx pgx.Tx, actor, sectionID, recordID string, author domain.Author, activityType string, occurredAt time.Time, summary, channel, ref string) error {
	identifier := token.New()
	_, err := tx.Exec(ctx, `INSERT INTO activities(id,section_id,record_id,type,occurred_at,summary,channel,ref,author_kind,author_id)
		VALUES($1,$2,$3,$4,$5,$6,NULLIF($7,''),NULLIF($8,''),$9,$10)`,
		identifier, sectionID, recordID, activityType, occurredAt, summary, channel, ref, author.Kind, author.ID)
	if err != nil {
		return err
	}
	return EmitTimelineEntry(ctx, tx, actor, sectionID, recordID, TimelineEntry{ID: identifier, Kind: activityType, Body: summary})
}

// logStatusChange appends a timeline entry when an existing record's status field changes.
func logStatusChange(ctx context.Context, tx pgx.Tx, fields []domain.Field, actor, sectionID, recordID string, before, after map[string]any) error {
	if !domain.HasField(fields, "status") {
		return nil
	}
	previous := domain.StatusText(before)
	next := domain.StatusText(after)
	if previous == next {
		return nil
	}
	author, err := authorFromActor(actor)
	if err != nil {
		return err
	}
	summary := fmt.Sprintf("Status changed from %s to %s", previous, next)
	if previous == "" {
		summary = fmt.Sprintf("Status set to %s", next)
	}
	if next == "" {
		summary = fmt.Sprintf("Status cleared from %s", previous)
	}
	return insertActivity(ctx, tx, actor, sectionID, recordID, author, "status_change", time.Now().UTC(), summary, "", "")
}

func (repository *Repository) LogActivity(ctx context.Context, actor, sectionID, recordID, activityType, date, summary, channel, ref string) (domain.Activity, error) {
	occurredAt, err := domain.ValidateActivity(activityType, date, summary, channel, ref)
	if err != nil {
		return domain.Activity{}, err
	}
	author, err := authorFromActor(actor)
	if err != nil {
		return domain.Activity{}, err
	}
	transaction, err := repository.pool.Begin(ctx)
	if err != nil {
		return domain.Activity{}, err
	}
	defer func() { _ = transaction.Rollback(ctx) }()
	var exists bool
	if err = transaction.QueryRow(ctx, "SELECT true FROM records WHERE section_id=$1 AND id=$2", sectionID, recordID).Scan(&exists); err != nil {
		return domain.Activity{}, classifyMissingRow(err)
	}
	activity := domain.Activity{
		ID: token.New(), Type: strings.TrimSpace(activityType), Date: occurredAt,
		Summary: strings.TrimSpace(summary), Channel: channel, Ref: ref, Author: author,
	}
	if err = transaction.QueryRow(ctx, `INSERT INTO activities(id,section_id,record_id,type,occurred_at,summary,channel,ref,author_kind,author_id)
		VALUES($1,$2,$3,$4,$5,$6,NULLIF($7,''),NULLIF($8,''),$9,$10) RETURNING created_at`,
		activity.ID, sectionID, recordID, activity.Type, activity.Date, activity.Summary, activity.Channel, activity.Ref, author.Kind, author.ID).Scan(&activity.CreatedAt); err != nil {
		return domain.Activity{}, err
	}
	if _, err = transaction.Exec(ctx, "INSERT INTO audit(actor,action,section_id,record_id,detail) VALUES($1,'log_activity',$2,$3,$4)", actor, sectionID, recordID, activity.ID); err != nil {
		return domain.Activity{}, err
	}
	if err = EmitTimelineEntry(ctx, transaction, actor, sectionID, recordID, TimelineEntry{ID: activity.ID, Kind: activity.Type, Body: activity.Summary}); err != nil {
		return domain.Activity{}, err
	}
	if err = transaction.Commit(ctx); err != nil {
		return domain.Activity{}, err
	}
	return activity, nil
}

func (repository *Repository) ListActivities(ctx context.Context, sectionID, recordID string) ([]domain.Activity, error) {
	var exists bool
	if err := repository.pool.QueryRow(ctx, "SELECT true FROM records WHERE section_id=$1 AND id=$2", sectionID, recordID).Scan(&exists); err != nil {
		return nil, classifyMissingRow(err)
	}
	rows, err := repository.pool.Query(ctx, `SELECT id,type,occurred_at,summary,COALESCE(channel,''),COALESCE(ref,''),author_kind,author_id,created_at
		FROM activities WHERE section_id=$1 AND record_id=$2 ORDER BY occurred_at DESC, created_at DESC, id DESC`, sectionID, recordID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	activities := []domain.Activity{}
	for rows.Next() {
		var activity domain.Activity
		if err = rows.Scan(&activity.ID, &activity.Type, &activity.Date, &activity.Summary, &activity.Channel, &activity.Ref, &activity.Author.Kind, &activity.Author.ID, &activity.CreatedAt); err != nil {
			return nil, err
		}
		activities = append(activities, activity)
	}
	return activities, rows.Err()
}
