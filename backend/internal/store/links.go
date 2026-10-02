package store

import (
	"context"
	"sort"

	"github.com/jackc/pgx/v5"
	"siracrm/internal/auth/token"
	"siracrm/internal/domain"
)

func lockSections(ctx context.Context, tx pgx.Tx, sectionIDs ...string) error {
	identifiers := append([]string(nil), sectionIDs...)
	sort.Strings(identifiers)
	seen := map[string]bool{}
	for _, identifier := range identifiers {
		if identifier == "" || seen[identifier] {
			continue
		}
		seen[identifier] = true
		var exists bool
		if err := tx.QueryRow(ctx, "SELECT true FROM sections WHERE id=$1 FOR UPDATE", identifier).Scan(&exists); err != nil {
			return classifyMissingRow(err)
		}
	}
	return nil
}

func (repository *Repository) ListLinks(ctx context.Context, sectionID, recordID string) ([]domain.RecordLink, error) {
	rows, err := repository.pool.Query(ctx, `SELECT l.source_section_id, ss.name, l.source_record_id, COALESCE(sr.data->>'name', ''),
		l.target_section_id, ts.name, l.target_record_id, COALESCE(tr.data->>'name', '')
		FROM record_links l
		JOIN sections ss ON ss.id = l.source_section_id
		JOIN sections ts ON ts.id = l.target_section_id
		LEFT JOIN records sr ON sr.id = l.source_record_id
		LEFT JOIN records tr ON tr.id = l.target_record_id
		WHERE (l.source_section_id=$1 AND l.source_record_id=$2) OR (l.target_section_id=$1 AND l.target_record_id=$2)
		ORDER BY l.created_at, l.id`, sectionID, recordID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	links := []domain.RecordLink{}
	for rows.Next() {
		var sourceSection, sourceName, sourceID, sourceRecordName, targetSection, targetName, targetID, targetRecordName string
		if err = rows.Scan(&sourceSection, &sourceName, &sourceID, &sourceRecordName, &targetSection, &targetName, &targetID, &targetRecordName); err != nil {
			return nil, err
		}
		link := domain.RecordLink{Direction: domain.LinkOutgoing, SectionID: targetSection, SectionName: targetName, RecordID: targetID, Name: targetRecordName}
		if sourceSection != sectionID || sourceID != recordID {
			link = domain.RecordLink{Direction: domain.LinkIncoming, SectionID: sourceSection, SectionName: sourceName, RecordID: sourceID, Name: sourceRecordName}
		}
		links = append(links, link)
	}
	return links, rows.Err()
}

// ConvertRecord creates a target record from a source record and stores one link.
// A second conversion of the same source into the same target section is rejected.
func (repository *Repository) ConvertRecord(ctx context.Context, actor, sourceSection, sourceID, targetSection string, mapping map[string]string, overrides map[string]any, status *string) (source domain.Record, target domain.Record, err error) {
	transaction, err := repository.pool.Begin(ctx)
	if err != nil {
		return domain.Record{}, domain.Record{}, err
	}
	defer func() { _ = transaction.Rollback(ctx) }()
	if err = lockSections(ctx, transaction, sourceSection, targetSection); err != nil {
		return domain.Record{}, domain.Record{}, err
	}
	sourceFields, err := loadFields(ctx, transaction, sourceSection)
	if err != nil {
		return domain.Record{}, domain.Record{}, err
	}
	targetFields, err := loadFields(ctx, transaction, targetSection)
	if err != nil {
		return domain.Record{}, domain.Record{}, err
	}
	var previous map[string]any
	if err = transaction.QueryRow(ctx, "SELECT data, updated_at FROM records WHERE id=$1 AND section_id=$2 FOR UPDATE", sourceID, sourceSection).Scan(&previous, &source.UpdatedAt); err != nil {
		return domain.Record{}, domain.Record{}, classifyMissingRow(err)
	}
	if previous == nil {
		previous = map[string]any{}
	}
	var already bool
	if err = transaction.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM record_links WHERE source_section_id=$1 AND source_record_id=$2 AND target_section_id=$3)", sourceSection, sourceID, targetSection).Scan(&already); err != nil {
		return domain.Record{}, domain.Record{}, err
	}
	if already {
		return domain.Record{}, domain.Record{}, domain.Invalid("This record is already linked to that section")
	}
	targetValues, err := domain.ConvertValues(sourceFields, targetFields, previous, mapping, overrides)
	if err != nil {
		return domain.Record{}, domain.Record{}, err
	}
	nextSource, err := domain.ApplyStatus(sourceFields, previous, status)
	if err != nil {
		return domain.Record{}, domain.Record{}, err
	}
	targetID := token.New()
	encodedTarget, err := encodeData(targetValues)
	if err != nil {
		return domain.Record{}, domain.Record{}, err
	}
	if err = transaction.QueryRow(ctx, "INSERT INTO records(id,section_id,data) VALUES($1,$2,$3) RETURNING updated_at", targetID, targetSection, encodedTarget).Scan(&target.UpdatedAt); err != nil {
		return domain.Record{}, domain.Record{}, err
	}
	if err = replaceRecordValues(ctx, transaction, targetSection, targetID, targetFields, targetValues); err != nil {
		return domain.Record{}, domain.Record{}, err
	}
	if status != nil && !domain.ValuesEqual(previous, nextSource) {
		encodedSource, encodeErr := encodeData(nextSource)
		if encodeErr != nil {
			return domain.Record{}, domain.Record{}, encodeErr
		}
		if err = transaction.QueryRow(ctx, "UPDATE records SET data=$3, updated_at=now() WHERE id=$1 AND section_id=$2 RETURNING updated_at", sourceID, sourceSection, encodedSource).Scan(&source.UpdatedAt); err != nil {
			return domain.Record{}, domain.Record{}, err
		}
		if err = replaceRecordValues(ctx, transaction, sourceSection, sourceID, sourceFields, nextSource); err != nil {
			return domain.Record{}, domain.Record{}, err
		}
		if err = logStatusChange(ctx, transaction, sourceFields, actor, sourceSection, sourceID, previous, nextSource); err != nil {
			return domain.Record{}, domain.Record{}, err
		}
	}
	if _, err = transaction.Exec(ctx, "INSERT INTO record_links(id,source_section_id,source_record_id,target_section_id,target_record_id) VALUES($1,$2,$3,$4,$5)", token.New(), sourceSection, sourceID, targetSection, targetID); err != nil {
		if classifyDatabaseError(err) == ErrConflict {
			return domain.Record{}, domain.Record{}, domain.Invalid("This record is already linked to that section")
		}
		return domain.Record{}, domain.Record{}, err
	}
	if _, err = transaction.Exec(ctx, "INSERT INTO audit(actor,action,section_id,record_id,detail) VALUES($1,'convert',$2,$3,$4)", actor, sourceSection, sourceID, targetSection+":"+targetID); err != nil {
		return domain.Record{}, domain.Record{}, err
	}
	if _, err = transaction.Exec(ctx, "INSERT INTO audit(actor,action,section_id,record_id) VALUES($1,'create',$2,$3)", actor, targetSection, targetID); err != nil {
		return domain.Record{}, domain.Record{}, err
	}
	if err = transaction.Commit(ctx); err != nil {
		return domain.Record{}, domain.Record{}, err
	}
	source.ID = sourceID
	source.Data = nextSource
	target.ID = targetID
	target.Data = targetValues
	return source, target, nil
}
