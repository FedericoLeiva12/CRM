package store

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
	"siracrm/internal/auth/token"
	"siracrm/internal/domain"
)

const RecordListLimit = domain.MaxPageSize

func classifyMissingRow(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

func loadFields(ctx context.Context, tx pgx.Tx, sectionID string) ([]domain.Field, error) {
	rows, err := tx.Query(ctx, "SELECT id,label,type,required FROM fields WHERE section_id=$1", sectionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	fields := []domain.Field{}
	for rows.Next() {
		var field domain.Field
		if err = rows.Scan(&field.ID, &field.Label, &field.Type, &field.Required); err != nil {
			return nil, err
		}
		fields = append(fields, field)
	}
	return fields, rows.Err()
}

func encodeData(values map[string]any) ([]byte, error) {
	if values == nil {
		values = map[string]any{}
	}
	return json.Marshal(values)
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

func (repository *Repository) GetRecord(ctx context.Context, sectionID, recordID string) (domain.Record, error) {
	var record domain.Record
	err := repository.pool.QueryRow(ctx, "SELECT id,data,updated_at FROM records WHERE section_id=$1 AND id=$2", sectionID, recordID).Scan(&record.ID, &record.Data, &record.UpdatedAt)
	if err != nil {
		return domain.Record{}, classifyMissingRow(err)
	}
	if record.Data == nil {
		record.Data = map[string]any{}
	}
	return record, nil
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
	fields, err := loadFields(ctx, transaction, sectionID)
	if err != nil {
		return "", err
	}
	if err = domain.ValidateRecord(fields, values); err != nil {
		return "", err
	}
	encoded, err := encodeData(values)
	if err != nil {
		return "", err
	}
	action := "create"
	eventType := domain.EventRecordCreated
	var previous map[string]any
	if recordID == "" {
		recordID = token.New()
		_, err = transaction.Exec(ctx, "INSERT INTO records(id,section_id,data) VALUES($1,$2,$3)", recordID, sectionID, encoded)
	} else {
		action = "update"
		eventType = domain.EventRecordUpdated
		if err = transaction.QueryRow(ctx, "SELECT data FROM records WHERE id=$1 AND section_id=$2 FOR UPDATE", recordID, sectionID).Scan(&previous); err != nil {
			return "", classifyMissingRow(err)
		}
		_, err = transaction.Exec(ctx, "UPDATE records SET data=$3,updated_at=now() WHERE id=$1 AND section_id=$2", recordID, sectionID, encoded)
	}
	if err != nil {
		return "", err
	}
	if err = replaceRecordValues(ctx, transaction, sectionID, recordID, fields, values); err != nil {
		return "", err
	}
	if action == "update" {
		if err = logStatusChange(ctx, transaction, fields, actor, sectionID, recordID, previous, values); err != nil {
			return "", err
		}
	}
	// Keep the write, its audit entry, and the webhook outbox atomic.
	if _, err = transaction.Exec(ctx, "INSERT INTO audit(actor,action,section_id,record_id) VALUES($1,$2,$3,$4)", actor, action, sectionID, recordID); err != nil {
		return "", err
	}
	if _, err = emitRecord(ctx, transaction, eventType, actor, sectionID, recordID, previous, values); err != nil {
		return "", err
	}
	return recordID, transaction.Commit(ctx)
}

// UpdateRecord changes only the fields present in patch. A nil value clears a field.
func (repository *Repository) UpdateRecord(ctx context.Context, actor, sectionID, recordID string, patch map[string]any) (domain.Record, error) {
	transaction, err := repository.pool.Begin(ctx)
	if err != nil {
		return domain.Record{}, err
	}
	defer func() { _ = transaction.Rollback(ctx) }()
	var exists bool
	if err = transaction.QueryRow(ctx, "SELECT true FROM sections WHERE id=$1 FOR UPDATE", sectionID).Scan(&exists); err != nil {
		return domain.Record{}, classifyMissingRow(err)
	}
	fields, err := loadFields(ctx, transaction, sectionID)
	if err != nil {
		return domain.Record{}, err
	}
	record := domain.Record{ID: recordID}
	if err = transaction.QueryRow(ctx, "SELECT data, updated_at FROM records WHERE id=$1 AND section_id=$2 FOR UPDATE", recordID, sectionID).Scan(&record.Data, &record.UpdatedAt); err != nil {
		return domain.Record{}, classifyMissingRow(err)
	}
	if record.Data == nil {
		record.Data = map[string]any{}
	}
	merged := domain.MergeRecord(record.Data, patch)
	if err = domain.ValidateRecord(fields, merged); err != nil {
		return domain.Record{}, err
	}
	if domain.ValuesEqual(record.Data, merged) {
		if err = transaction.Commit(ctx); err != nil {
			return domain.Record{}, err
		}
		return record, nil
	}
	encoded, err := encodeData(merged)
	if err != nil {
		return domain.Record{}, err
	}
	if err = transaction.QueryRow(ctx, "UPDATE records SET data=$3, updated_at=now() WHERE id=$1 AND section_id=$2 RETURNING updated_at", recordID, sectionID, encoded).Scan(&record.UpdatedAt); err != nil {
		return domain.Record{}, err
	}
	if err = replaceRecordValues(ctx, transaction, sectionID, recordID, fields, merged); err != nil {
		return domain.Record{}, err
	}
	if err = logStatusChange(ctx, transaction, fields, actor, sectionID, recordID, record.Data, merged); err != nil {
		return domain.Record{}, err
	}
	if _, err = transaction.Exec(ctx, "INSERT INTO audit(actor,action,section_id,record_id,detail) VALUES($1,'update',$2,$3,'partial')", actor, sectionID, recordID); err != nil {
		return domain.Record{}, err
	}
	if _, err = emitRecord(ctx, transaction, domain.EventRecordUpdated, actor, sectionID, recordID, record.Data, merged); err != nil {
		return domain.Record{}, err
	}
	if err = transaction.Commit(ctx); err != nil {
		return domain.Record{}, err
	}
	record.Data = merged
	return record, nil
}

func (repository *Repository) DeleteRecord(ctx context.Context, actor, sectionID, recordID string) error {
	transaction, err := repository.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = transaction.Rollback(ctx) }()
	var data map[string]any
	err = transaction.QueryRow(ctx, "SELECT data FROM records WHERE section_id=$1 AND id=$2 FOR UPDATE", sectionID, recordID).Scan(&data)
	if err != nil {
		return classifyMissingRow(err)
	}
	if _, err = transaction.Exec(ctx, "DELETE FROM activities WHERE section_id=$1 AND record_id=$2", sectionID, recordID); err != nil {
		return err
	}
	if _, err = transaction.Exec(ctx, "DELETE FROM records WHERE section_id=$1 AND id=$2", sectionID, recordID); err != nil {
		return err
	}
	if _, err = transaction.Exec(ctx, "INSERT INTO audit(actor,action,section_id,record_id) VALUES($1,'delete',$2,$3)", actor, sectionID, recordID); err != nil {
		return err
	}
	if data == nil {
		data = map[string]any{}
	}
	if _, err = emit(ctx, transaction, outboundEvent{Type: domain.EventRecordDeleted, Actor: actor, SectionID: sectionID, RecordID: recordID, Data: map[string]any{"fields": data}}); err != nil {
		return err
	}
	return transaction.Commit(ctx)
}
