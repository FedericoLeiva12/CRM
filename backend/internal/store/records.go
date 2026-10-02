package store

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
	"siracrm/internal/auth/token"
	"siracrm/internal/domain"
)

const RecordListLimit = 500

func classifyMissingRow(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return err
}
func (repository *Repository) ListRecords(ctx context.Context, sectionID string) ([]domain.Record, error) {
	rows, err := repository.pool.Query(ctx, "SELECT id,data,updated_at FROM records WHERE section_id=$1 ORDER BY updated_at DESC LIMIT $2", sectionID, RecordListLimit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	records := []domain.Record{}
	for rows.Next() {
		var record domain.Record
		if err = rows.Scan(&record.ID, &record.Data, &record.UpdatedAt); err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, rows.Err()
}

// SaveRecord replaces complete record values. A missing ID creates a new record.
func (repository *Repository) SaveRecord(ctx context.Context, actor, sectionID, recordID string, values map[string]any) (string, error) {
	transaction, err := repository.pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer func() { _ = transaction.Rollback(ctx) }()
	var exists bool
	if err = transaction.QueryRow(ctx, "SELECT true FROM sections WHERE id=$1 FOR UPDATE", sectionID).Scan(&exists); err != nil {
		return "", classifyMissingRow(err)
	}
	rows, err := transaction.Query(ctx, "SELECT id,label,type,required FROM fields WHERE section_id=$1", sectionID)
	if err != nil {
		return "", err
	}
	fields := []domain.Field{}
	for rows.Next() {
		var field domain.Field
		if err = rows.Scan(&field.ID, &field.Label, &field.Type, &field.Required); err != nil {
			rows.Close()
			return "", err
		}
		fields = append(fields, field)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return "", err
	}
	if err = domain.ValidateRecord(fields, values); err != nil {
		return "", err
	}
	encoded, err := json.Marshal(values)
	if err != nil {
		return "", err
	}
	action := "create"
	if recordID == "" {
		recordID = token.New()
		_, err = transaction.Exec(ctx, "INSERT INTO records(id,section_id,data) VALUES($1,$2,$3)", recordID, sectionID, encoded)
	} else {
		action = "update"
		result, updateErr := transaction.Exec(ctx, "UPDATE records SET data=$3,updated_at=now() WHERE id=$1 AND section_id=$2", recordID, sectionID, encoded)
		err = updateErr
		if err == nil && result.RowsAffected() == 0 {
			return "", ErrNotFound
		}
	}
	if err != nil {
		return "", err
	}
	// Keep the write and its audit entry atomic: neither may succeed independently.
	if _, err = transaction.Exec(ctx, "INSERT INTO audit(actor,action,section_id,record_id) VALUES($1,$2,$3,$4)", actor, action, sectionID, recordID); err != nil {
		return "", err
	}
	return recordID, transaction.Commit(ctx)
}
func (repository *Repository) DeleteRecord(ctx context.Context, actor, sectionID, recordID string) error {
	transaction, err := repository.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = transaction.Rollback(ctx) }()
	result, err := transaction.Exec(ctx, "DELETE FROM records WHERE section_id=$1 AND id=$2", sectionID, recordID)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return ErrNotFound
	}
	if _, err = transaction.Exec(ctx, "INSERT INTO audit(actor,action,section_id,record_id) VALUES($1,'delete',$2,$3)", actor, sectionID, recordID); err != nil {
		return err
	}
	return transaction.Commit(ctx)
}
