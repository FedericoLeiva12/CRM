package store

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"siracrm/internal/domain"
)

// replaceRecordValues rebuilds the typed projection for one record.
// records.data stays the source of truth; filters read this table.
func replaceRecordValues(ctx context.Context, tx pgx.Tx, sectionID, recordID string, fields []domain.Field, values map[string]any) error {
	if _, err := tx.Exec(ctx, "DELETE FROM record_values WHERE record_id=$1", recordID); err != nil {
		return err
	}
	for _, field := range fields {
		value, present := values[field.ID]
		if !present || value == nil {
			continue
		}
		switch field.Type {
		case domain.FieldText, domain.FieldEmail:
			text, ok := value.(string)
			if !ok {
				return fmt.Errorf("record %s field %s has an unexpected stored type", recordID, field.ID)
			}
			if text == "" {
				continue
			}
			if _, err := tx.Exec(ctx, "INSERT INTO record_values(record_id,section_id,field_id,text_value) VALUES($1,$2,$3,$4)", recordID, sectionID, field.ID, text); err != nil {
				return err
			}
		case domain.FieldNumber:
			number, ok := value.(float64)
			if !ok {
				return fmt.Errorf("record %s field %s has an unexpected stored type", recordID, field.ID)
			}
			if _, err := tx.Exec(ctx, "INSERT INTO record_values(record_id,section_id,field_id,number_value) VALUES($1,$2,$3,$4)", recordID, sectionID, field.ID, number); err != nil {
				return err
			}
		case domain.FieldDate:
			text, ok := value.(string)
			if !ok {
				return fmt.Errorf("record %s field %s has an unexpected stored type", recordID, field.ID)
			}
			if text == "" {
				continue
			}
			if _, err := tx.Exec(ctx, "INSERT INTO record_values(record_id,section_id,field_id,date_value) VALUES($1,$2,$3,$4)", recordID, sectionID, field.ID, text); err != nil {
				return err
			}
		case domain.FieldBoolean:
			flag, ok := value.(bool)
			if !ok {
				return fmt.Errorf("record %s field %s has an unexpected stored type", recordID, field.ID)
			}
			if _, err := tx.Exec(ctx, "INSERT INTO record_values(record_id,section_id,field_id,bool_value) VALUES($1,$2,$3,$4)", recordID, sectionID, field.ID, flag); err != nil {
				return err
			}
		default:
			return fmt.Errorf("record %s field %s has an unsupported type", recordID, field.Type)
		}
	}
	return nil
}
