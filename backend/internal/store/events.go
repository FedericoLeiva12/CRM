package store

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"siracrm/internal/auth/token"
	"siracrm/internal/domain"
)

type outboundEvent struct {
	Type       string
	Actor      string
	SectionID  string
	RecordID   string
	Data       map[string]any
	EndpointID string
}

// TimelineEntry is the compact snapshot delivered with timeline.entry_created.
// Body is truncated to 500 characters in the webhook payload.
type TimelineEntry struct {
	ID   string
	Kind string
	Body string
}

// EmitTimelineEntry writes a timeline.entry_created event to the outbox inside tx.
// Timeline persistence should call this before committing so the event is durable
// with the entry. Kind is the activity type: 1–80 characters, without control
// characters. The hook does not interpret the body.
func EmitTimelineEntry(ctx context.Context, tx pgx.Tx, actor, sectionID, recordID string, entry TimelineEntry) error {
	if sectionID == "" || recordID == "" {
		return domain.Invalid("Timeline events require a section and record")
	}
	kind := strings.TrimSpace(entry.Kind)
	if !validTimelineID(entry.ID) || !validTimelineKind(kind) {
		return domain.Invalid("Timeline entry needs an id and a kind")
	}
	_, err := emit(ctx, tx, outboundEvent{
		Type:      domain.EventTimelineEntryCreated,
		Actor:     actor,
		SectionID: sectionID,
		RecordID:  recordID,
		Data: map[string]any{
			"entry_id": entry.ID,
			"kind":     kind,
			"body":     clipText(entry.Body, 500),
		},
	})
	return err
}

// CommentMention describes one mentioned principal on a comment.
type CommentMention struct {
	EntryID   string
	ParentID  string
	Body      string
	Mentioned domain.Principal
	Author    domain.Author
}

// EmitCommentMentioned writes one comment.mentioned event, for one mentioned user or agent,
// to the outbox inside tx. Body is truncated to 500 characters.
func EmitCommentMentioned(ctx context.Context, tx pgx.Tx, actor, sectionID, recordID string, mention CommentMention) error {
	if sectionID == "" || recordID == "" || !validTimelineID(mention.EntryID) {
		return domain.Invalid("Mention events require a section, record, and entry")
	}
	section, err := lookupSection(ctx, tx, sectionID)
	if err != nil {
		return err
	}
	var parentID any
	if mention.ParentID != "" {
		parentID = mention.ParentID
	}
	_, err = emit(ctx, tx, outboundEvent{
		Type:      domain.EventCommentMentioned,
		Actor:     actor,
		SectionID: sectionID,
		RecordID:  recordID,
		Data: map[string]any{
			"entry_id":  mention.EntryID,
			"section":   section,
			"record_id": recordID,
			"mentioned": map[string]any{"kind": mention.Mentioned.Kind, "id": mention.Mentioned.ID, "handle": mention.Mentioned.Handle},
			"author":    map[string]any{"kind": mention.Author.Kind, "id": mention.Author.ID, "name": mention.Author.Name, "handle": mention.Author.Handle},
			"body":      clipText(mention.Body, 500),
			"parent_id": parentID,
		},
	})
	return err
}

func emitRecord(ctx context.Context, tx pgx.Tx, eventType, actor, sectionID, recordID string, previous, values map[string]any) (string, error) {
	snapshot := values
	if snapshot == nil {
		snapshot = map[string]any{}
	}
	payload := map[string]any{"fields": snapshot}
	if eventType == domain.EventRecordUpdated {
		changedIDs, changedValues := domain.ChangedFields(previous, snapshot)
		payload = map[string]any{"changed_field_ids": changedIDs, "fields": changedValues}
	}
	return emit(ctx, tx, outboundEvent{Type: eventType, Actor: actor, SectionID: sectionID, RecordID: recordID, Data: payload})
}

func emit(ctx context.Context, tx pgx.Tx, event outboundEvent) (string, error) {
	if event.Data == nil {
		event.Data = map[string]any{}
	}
	actor, err := lookupActor(ctx, tx, event.Actor)
	if err != nil {
		return "", err
	}
	section, err := lookupSection(ctx, tx, event.SectionID)
	if err != nil {
		return "", err
	}
	identifier := "evt_" + token.New()
	var recordID *string
	if event.RecordID != "" {
		recordID = &event.RecordID
	}
	encoded, err := json.Marshal(domain.EventEnvelope{
		SchemaVersion: domain.EventSchemaVersion,
		ID:            identifier,
		Type:          event.Type,
		OccurredAt:    time.Now().UTC().Truncate(time.Millisecond),
		Actor:         actor,
		Section:       section,
		RecordID:      recordID,
		Data:          event.Data,
	})
	if err != nil {
		return "", err
	}
	// The outbox row commits with the change that caused it. Delivery happens later.
	if _, err = tx.Exec(ctx, "INSERT INTO webhook_outbox(id,event_type,section_id,payload) VALUES($1,$2,NULLIF($3,''),$4)", identifier, event.Type, event.SectionID, string(encoded)); err != nil {
		return "", err
	}
	if event.EndpointID != "" {
		tag, insertErr := tx.Exec(ctx, "INSERT INTO webhook_deliveries(outbox_id,endpoint_id,event_type) SELECT $1,id,$2 FROM webhook_endpoints WHERE id=$3", identifier, event.Type, event.EndpointID)
		if insertErr != nil {
			return "", insertErr
		}
		if tag.RowsAffected() == 0 {
			return "", ErrNotFound
		}
		return identifier, nil
	}
	actorKind, actorID := splitActorRef(event.Actor)
	mentionedKind, mentionedID := mentionPrincipal(event)
	fanout := `WITH subscribed AS (
  SELECT e.id FROM webhook_endpoints e
  WHERE e.enabled AND NOT e.auto_disabled AND $1 = ANY(e.event_types)
  AND (e.section_id IS NULL OR e.section_id = NULLIF($2,''))
), deliver AS (
  SELECT s.id FROM subscribed s
  WHERE NOT (
    $3 IN ('user','agent') AND EXISTS (
      SELECT 1 FROM webhook_excluded_actors x
      WHERE x.endpoint_id = s.id AND x.kind = $3 AND x.actor_id = $4
    ) AND NOT (
      $1 = 'comment.mentioned' AND $5 IN ('user','agent') AND $6 <> '' AND EXISTS (
        SELECT 1 FROM webhook_excluded_actors x
        WHERE x.endpoint_id = s.id AND x.kind = $5 AND x.actor_id = $6
      )
    )
  )
)`
	fanoutArgs := []any{event.Type, event.SectionID, actorKind, actorID, mentionedKind, mentionedID}
	if _, err = tx.Exec(ctx, fanout+`
INSERT INTO webhook_deliveries(outbox_id,endpoint_id,event_type)
SELECT $7,d.id,$1 FROM deliver d`, append(fanoutArgs, identifier)...); err != nil {
		return "", err
	}
	_, err = tx.Exec(ctx, fanout+`
UPDATE webhook_endpoints e
SET skipped_events = e.skipped_events + 1, updated_at = now()
FROM subscribed s
WHERE e.id = s.id
AND NOT EXISTS (SELECT 1 FROM deliver d WHERE d.id = s.id)`, fanoutArgs...)
	return identifier, err
}

func splitActorRef(actor string) (kind, id string) {
	kind, id, ok := strings.Cut(actor, ":")
	if !ok || id == "" || (kind != "user" && kind != "agent") {
		return "", ""
	}
	return kind, id
}

func mentionPrincipal(event outboundEvent) (kind, id string) {
	if event.Type != domain.EventCommentMentioned {
		return "", ""
	}
	mentioned, ok := event.Data["mentioned"].(map[string]any)
	if !ok {
		return "", ""
	}
	kind, _ = mentioned["kind"].(string)
	id, _ = mentioned["id"].(string)
	return kind, id
}

func lookupActor(ctx context.Context, tx pgx.Tx, actor string) (domain.EventActor, error) {
	kind, id, ok := strings.Cut(actor, ":")
	if !ok || id == "" || (kind != "user" && kind != "agent") {
		return domain.EventActor{Kind: "system", ID: actor}, nil
	}
	var name string
	var err error
	if kind == "user" {
		err = tx.QueryRow(ctx, "SELECT coalesce(nullif(btrim(name),''), email) FROM users WHERE id=$1", id).Scan(&name)
	} else {
		err = tx.QueryRow(ctx, "SELECT name FROM agents WHERE id=$1", id).Scan(&name)
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.EventActor{Kind: kind, ID: id}, nil
	}
	if err != nil {
		return domain.EventActor{}, err
	}
	return domain.EventActor{Kind: kind, ID: id, Name: name}, nil
}

func lookupSection(ctx context.Context, tx pgx.Tx, sectionID string) (*domain.EventSection, error) {
	if sectionID == "" {
		return nil, nil
	}
	var name string
	err := tx.QueryRow(ctx, "SELECT name FROM sections WHERE id=$1", sectionID).Scan(&name)
	if errors.Is(err, pgx.ErrNoRows) {
		return &domain.EventSection{ID: sectionID}, nil
	}
	if err != nil {
		return nil, err
	}
	return &domain.EventSection{ID: sectionID, Name: name}, nil
}

func validTimelineKind(kind string) bool {
	if kind == "" || len(kind) > 80 {
		return false
	}
	for _, char := range kind {
		if char < 0x20 || char == 0x7f {
			return false
		}
	}
	return true
}

func validTimelineID(id string) bool {
	if id == "" || len(id) > 80 {
		return false
	}
	for _, char := range id {
		if char < 0x21 || char > 0x7e {
			return false
		}
	}
	return true
}

func clipText(value string, limit int) string {
	if utf8.RuneCountInString(value) <= limit {
		return value
	}
	runes := []rune(value)
	return string(runes[:limit])
}
