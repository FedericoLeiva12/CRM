package store

import (
	"context"

	"siracrm/internal/domain"
)

func (repository *Repository) ListSections(ctx context.Context) ([]domain.Section, error) {
	rows, err := repository.pool.Query(ctx, "SELECT id,name FROM sections ORDER BY created_at,id")
	if err != nil {
		return nil, err
	}
	sections := []domain.Section{}
	for rows.Next() {
		var section domain.Section
		if err = rows.Scan(&section.ID, &section.Name); err != nil {
			rows.Close()
			return nil, err
		}
		section.Fields = []domain.Field{}
		sections = append(sections, section)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	for index := range sections {
		fields, err := repository.ListFields(ctx, sections[index].ID)
		if err != nil {
			return nil, err
		}
		sections[index].Fields = fields
	}
	return sections, nil
}
func (repository *Repository) ListFields(ctx context.Context, sectionID string) ([]domain.Field, error) {
	rows, err := repository.pool.Query(ctx, "SELECT id,label,type,required FROM fields WHERE section_id=$1 ORDER BY id", sectionID)
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

// CreateSection inserts a section and its required name field. It does not grant any agent read or write.
func (repository *Repository) CreateSection(ctx context.Context, actor, identifier, name string) error {
	if err := domain.ValidateSection(identifier, name); err != nil {
		return err
	}
	transaction, err := repository.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = transaction.Rollback(ctx) }()
	if _, err = transaction.Exec(ctx, "INSERT INTO sections(id,name) VALUES($1,$2)", identifier, name); err != nil {
		return classifyDatabaseError(err)
	}
	if _, err = transaction.Exec(ctx, "INSERT INTO fields(id,section_id,label,type,required) VALUES('name',$1,'Name','text',true)", identifier); err != nil {
		return err
	}
	if _, err = transaction.Exec(ctx, "INSERT INTO audit(actor,action,section_id) VALUES($1,'create_section',$2)", actor, identifier); err != nil {
		return err
	}
	return transaction.Commit(ctx)
}
func (repository *Repository) AddField(ctx context.Context, actor, sectionID string, field domain.Field) error {
	if err := domain.ValidateField(field); err != nil {
		return err
	}
	transaction, err := repository.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = transaction.Rollback(ctx) }()
	// Record writes acquire this same section lock. Required-field changes cannot race a save.
	var exists bool
	if err = transaction.QueryRow(ctx, "SELECT true FROM sections WHERE id=$1 FOR UPDATE", sectionID).Scan(&exists); err != nil {
		return classifyMissingRow(err)
	}
	if field.Required {
		var hasRecords bool
		if err = transaction.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM records WHERE section_id=$1)", sectionID).Scan(&hasRecords); err != nil {
			return err
		}
		if hasRecords {
			return domain.Invalid("New fields must be optional while records exist")
		}
	}
	_, err = transaction.Exec(ctx, "INSERT INTO fields(id,section_id,label,type,required) VALUES($1,$2,$3,$4,$5)", field.ID, sectionID, field.Label, field.Type, field.Required)
	if err != nil {
		return classifyDatabaseError(err)
	}
	// record_id stores the field identifier; schema rows are not record mutations.
	if _, err = transaction.Exec(ctx, "INSERT INTO audit(actor,action,section_id,record_id) VALUES($1,'add_field',$2,$3)", actor, sectionID, field.ID); err != nil {
		return err
	}
	return transaction.Commit(ctx)
}
