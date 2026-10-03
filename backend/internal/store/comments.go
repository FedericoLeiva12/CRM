package store

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"siracrm/internal/auth/token"
	"siracrm/internal/domain"
)

// reader is the read side shared by the pool and a transaction.
type reader interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

type scanner interface{ Scan(dest ...any) error }

// activityColumns and activityFrom read a timeline entry together with its
// author's current name and handle.
const activityColumns = `a.id,a.type,a.occurred_at,a.summary,COALESCE(a.channel,''),COALESCE(a.ref,''),a.author_kind,a.author_id,
	COALESCE(CASE WHEN a.author_kind='user' THEN COALESCE(NULLIF(btrim(u.name),''),u.handle) ELSE g.name END,''),
	COALESCE(u.handle,g.handle,''),a.created_at,COALESCE(a.parent_id,''),a.edited_at,a.deleted_at IS NOT NULL`

const activityFrom = `FROM activities a
	LEFT JOIN users u ON a.author_kind='user' AND u.id=a.author_id
	LEFT JOIN agents g ON a.author_kind='agent' AND g.id=a.author_id`

func scanActivity(row scanner, extra ...any) (domain.Activity, error) {
	var activity domain.Activity
	destinations := []any{&activity.ID, &activity.Type, &activity.Date, &activity.Summary, &activity.Channel, &activity.Ref,
		&activity.Author.Kind, &activity.Author.ID, &activity.Author.Name, &activity.Author.Handle,
		&activity.CreatedAt, &activity.ParentID, &activity.EditedAt, &activity.Deleted}
	if err := row.Scan(append(destinations, extra...)...); err != nil {
		return domain.Activity{}, err
	}
	return activity, nil
}

// attachMentions fills Mentions on comment entries in one query.
func attachMentions(ctx context.Context, source reader, activities []domain.Activity) error {
	identifiers := []string{}
	for _, activity := range activities {
		if activity.Type == domain.CommentType {
			identifiers = append(identifiers, activity.ID)
		}
	}
	if len(identifiers) == 0 {
		return nil
	}
	rows, err := source.Query(ctx, `SELECT m.entry_id,m.kind,m.principal_id,m.handle,
		COALESCE(CASE WHEN m.kind='user' THEN COALESCE(NULLIF(btrim(u.name),''),u.handle) ELSE g.name END,'')
		FROM comment_mentions m
		LEFT JOIN users u ON m.kind='user' AND u.id=m.principal_id
		LEFT JOIN agents g ON m.kind='agent' AND g.id=m.principal_id
		WHERE m.entry_id = ANY($1) ORDER BY m.entry_id, m.handle`, identifiers)
	if err != nil {
		return err
	}
	defer rows.Close()
	byEntry := map[string][]domain.Principal{}
	for rows.Next() {
		var entryID string
		var principal domain.Principal
		if err = rows.Scan(&entryID, &principal.Kind, &principal.ID, &principal.Handle, &principal.Name); err != nil {
			return err
		}
		byEntry[entryID] = append(byEntry[entryID], principal)
	}
	if err = rows.Err(); err != nil {
		return err
	}
	for index := range activities {
		if activities[index].Type == domain.CommentType {
			activities[index].Mentions = byEntry[activities[index].ID]
			if activities[index].Mentions == nil {
				activities[index].Mentions = []domain.Principal{}
			}
		}
	}
	return nil
}

func commentFromActivity(activity domain.Activity, sectionID, recordID string) domain.Comment {
	mentions := activity.Mentions
	if mentions == nil {
		mentions = []domain.Principal{}
	}
	return domain.Comment{
		ID: activity.ID, SectionID: sectionID, RecordID: recordID, ParentID: activity.ParentID,
		Body: activity.Summary, Author: activity.Author, Mentions: mentions,
		CreatedAt: activity.CreatedAt, EditedAt: activity.EditedAt, Deleted: activity.Deleted,
	}
}

// fetchComment reads one comment entry with names and mentions.
func fetchComment(ctx context.Context, source reader, commentID string) (domain.Comment, error) {
	var sectionID, recordID string
	activity, err := scanActivity(source.QueryRow(ctx, "SELECT "+activityColumns+",a.section_id,a.record_id "+activityFrom+" WHERE a.id=$1 AND a.type='comment'", commentID), &sectionID, &recordID)
	if err != nil {
		return domain.Comment{}, classifyMissingRow(err)
	}
	entries := []domain.Activity{activity}
	if err = attachMentions(ctx, source, entries); err != nil {
		return domain.Comment{}, err
	}
	return commentFromActivity(entries[0], sectionID, recordID), nil
}

type commentInput struct {
	Body       string
	ParentID   string
	Mentions   []string
	OccurredAt time.Time
	Channel    string
	Ref        string
	// AuditAction names the audit entry; the activity tool keeps its historical "log_activity".
	AuditAction string
}

// CommentResult is a saved comment plus the mentions that could not be applied.
// Unresolved entries are @handles that match nobody, agents that cannot read
// the section, and ids that do not exist. They are reported, not rejected.
type CommentResult struct {
	Comment            domain.Comment
	UnresolvedMentions []string
}

// CreateComment appends a comment, or a reply when parentID is set, to a record's timeline.
// Mentions come from @handles in the body and from explicit user or agent ids.
// Callers enforce section write access for agents.
func (repository *Repository) CreateComment(ctx context.Context, actor, sectionID, recordID, body, parentID string, mentions []string) (CommentResult, error) {
	activity, unresolved, err := repository.createComment(ctx, actor, sectionID, recordID, commentInput{Body: body, ParentID: parentID, Mentions: mentions})
	if err != nil {
		return CommentResult{}, err
	}
	return CommentResult{Comment: commentFromActivity(activity, sectionID, recordID), UnresolvedMentions: unresolved}, nil
}

func parseMentionRefs(raw []string) ([]domain.MentionRef, error) {
	refs := make([]domain.MentionRef, 0, len(raw))
	for _, value := range raw {
		ref, err := domain.ParseMentionRef(value)
		if err != nil {
			return nil, err
		}
		refs = append(refs, ref)
	}
	return refs, nil
}

func (repository *Repository) createComment(ctx context.Context, actor, sectionID, recordID string, input commentInput) (domain.Activity, []string, error) {
	body, err := domain.ValidateCommentBody(input.Body)
	if err != nil {
		return domain.Activity{}, nil, err
	}
	refs, err := parseMentionRefs(input.Mentions)
	if err != nil {
		return domain.Activity{}, nil, err
	}
	author, err := authorFromActor(actor)
	if err != nil {
		return domain.Activity{}, nil, err
	}
	transaction, err := repository.pool.Begin(ctx)
	if err != nil {
		return domain.Activity{}, nil, err
	}
	defer func() { _ = transaction.Rollback(ctx) }()
	var exists bool
	if err = transaction.QueryRow(ctx, "SELECT true FROM records WHERE section_id=$1 AND id=$2", sectionID, recordID).Scan(&exists); err != nil {
		return domain.Activity{}, nil, classifyMissingRow(err)
	}
	if input.ParentID != "" {
		if err = checkReplyParent(ctx, transaction, sectionID, recordID, input.ParentID); err != nil {
			return domain.Activity{}, nil, err
		}
	}
	identifier := token.New()
	occurredAt := input.OccurredAt
	if occurredAt.IsZero() {
		occurredAt = time.Now().UTC()
	}
	if _, err = transaction.Exec(ctx, `INSERT INTO activities(id,section_id,record_id,type,occurred_at,summary,channel,ref,author_kind,author_id,parent_id)
		VALUES($1,$2,$3,'comment',$4,$5,NULLIF($6,''),NULLIF($7,''),$8,$9,NULLIF($10,''))`,
		identifier, sectionID, recordID, occurredAt, body, input.Channel, input.Ref, author.Kind, author.ID, input.ParentID); err != nil {
		return domain.Activity{}, nil, classifyDatabaseError(err)
	}
	resolved, unresolved, err := resolveMentions(ctx, transaction, sectionID, author, domain.ParseMentionHandles(body), refs)
	if err != nil {
		return domain.Activity{}, nil, err
	}
	if err = insertMentions(ctx, transaction, identifier, resolved); err != nil {
		return domain.Activity{}, nil, err
	}
	auditAction := input.AuditAction
	if auditAction == "" {
		auditAction = "comment"
	}
	if _, err = transaction.Exec(ctx, "INSERT INTO audit(actor,action,section_id,record_id,detail) VALUES($1,$2,$3,$4,$5)", actor, auditAction, sectionID, recordID, identifier); err != nil {
		return domain.Activity{}, nil, err
	}
	if err = EmitTimelineEntry(ctx, transaction, actor, sectionID, recordID, TimelineEntry{ID: identifier, Kind: domain.CommentType, Body: body}); err != nil {
		return domain.Activity{}, nil, err
	}
	if err = emitMentions(ctx, transaction, actor, sectionID, recordID, identifier, input.ParentID, body, resolved); err != nil {
		return domain.Activity{}, nil, err
	}
	comment, err := fetchComment(ctx, transaction, identifier)
	if err != nil {
		return domain.Activity{}, nil, err
	}
	if err = transaction.Commit(ctx); err != nil {
		return domain.Activity{}, nil, err
	}
	activity := domain.Activity{
		ID: comment.ID, Type: domain.CommentType, Date: occurredAt, Summary: comment.Body, Channel: input.Channel, Ref: input.Ref,
		Author: comment.Author, CreatedAt: comment.CreatedAt, ParentID: comment.ParentID, Mentions: comment.Mentions,
	}
	return activity, unresolved, nil
}

// checkReplyParent allows one level of replies to a live top-level comment on the same record.
func checkReplyParent(ctx context.Context, tx pgx.Tx, sectionID, recordID, parentID string) error {
	var parentSection, parentRecord, parentType, grandparent string
	var deleted bool
	err := tx.QueryRow(ctx, "SELECT section_id,record_id,type,COALESCE(parent_id,''),deleted_at IS NOT NULL FROM activities WHERE id=$1 FOR SHARE", parentID).
		Scan(&parentSection, &parentRecord, &parentType, &grandparent, &deleted)
	if err != nil {
		return classifyMissingRow(err)
	}
	if parentSection != sectionID || parentRecord != recordID || parentType != domain.CommentType {
		return ErrNotFound
	}
	if grandparent != "" {
		return domain.Invalid("Replies can only be added to a top-level comment")
	}
	if deleted {
		return domain.Invalid("This comment was deleted")
	}
	return nil
}

type mentionTarget struct {
	principal domain.Principal
	canRead   bool
}

// resolveMentions maps @handles and explicit ids to principals who may be told about
// the comment. The author is skipped silently. Users can see every section; an agent
// needs read on the section. Results keep the order they were written in.
func resolveMentions(ctx context.Context, source reader, sectionID string, author domain.Author, handles []string, refs []domain.MentionRef) ([]domain.Principal, []string, error) {
	if len(handles) == 0 && len(refs) == 0 {
		return nil, nil, nil
	}
	userIDs, agentIDs := []string{}, []string{}
	for _, ref := range refs {
		if ref.Kind != domain.PrincipalAgent {
			userIDs = append(userIDs, ref.ID)
		}
		if ref.Kind != domain.PrincipalUser {
			agentIDs = append(agentIDs, ref.ID)
		}
	}
	byHandle := map[string]mentionTarget{}
	byID := map[string]mentionTarget{}
	index := func(target mentionTarget) {
		byHandle[target.principal.Handle] = target
		byID[target.principal.Kind+":"+target.principal.ID] = target
	}
	userRows, err := source.Query(ctx, "SELECT id,handle,COALESCE(NULLIF(btrim(name),''),handle) FROM users WHERE handle = ANY($1) OR id = ANY($2)", handles, userIDs)
	if err != nil {
		return nil, nil, err
	}
	for userRows.Next() {
		target := mentionTarget{canRead: true}
		target.principal.Kind = domain.PrincipalUser
		if err = userRows.Scan(&target.principal.ID, &target.principal.Handle, &target.principal.Name); err != nil {
			userRows.Close()
			return nil, nil, err
		}
		index(target)
	}
	userRows.Close()
	if err = userRows.Err(); err != nil {
		return nil, nil, err
	}
	agentRows, err := source.Query(ctx, `SELECT a.id,a.handle,a.name,COALESCE(p.can_read,false) FROM agents a
		LEFT JOIN permissions p ON p.agent_id=a.id AND p.section_id=$3
		WHERE a.handle = ANY($1) OR a.id = ANY($2)`, handles, agentIDs, sectionID)
	if err != nil {
		return nil, nil, err
	}
	for agentRows.Next() {
		var target mentionTarget
		target.principal.Kind = domain.PrincipalAgent
		if err = agentRows.Scan(&target.principal.ID, &target.principal.Handle, &target.principal.Name, &target.canRead); err != nil {
			agentRows.Close()
			return nil, nil, err
		}
		index(target)
	}
	agentRows.Close()
	if err = agentRows.Err(); err != nil {
		return nil, nil, err
	}
	resolved := []domain.Principal{}
	unresolved := []string{}
	seen := map[string]bool{}
	consider := func(target mentionTarget, found bool, label string) {
		if !found {
			unresolved = append(unresolved, label)
			return
		}
		key := target.principal.Kind + ":" + target.principal.ID
		if key == author.Kind+":"+author.ID || seen[key] {
			return
		}
		if !target.canRead {
			unresolved = append(unresolved, label)
			return
		}
		seen[key] = true
		resolved = append(resolved, target.principal)
	}
	for _, handle := range handles {
		target, found := byHandle[handle]
		consider(target, found, "@"+handle)
	}
	for _, ref := range refs {
		var target mentionTarget
		found := false
		for _, kind := range []string{domain.PrincipalUser, domain.PrincipalAgent} {
			if ref.Kind != "" && ref.Kind != kind {
				continue
			}
			if candidate, ok := byID[kind+":"+ref.ID]; ok {
				target, found = candidate, true
				break
			}
		}
		consider(target, found, ref.Kind+":"+ref.ID)
	}
	for index, label := range unresolved {
		unresolved[index] = strings.TrimPrefix(label, ":")
	}
	if len(resolved) > domain.MaxMentionsPerComment {
		return nil, nil, domain.Invalid("Mention at most 20 people in one comment")
	}
	return resolved, unresolved, nil
}

func insertMentions(ctx context.Context, tx pgx.Tx, entryID string, principals []domain.Principal) error {
	for _, principal := range principals {
		if _, err := tx.Exec(ctx, "INSERT INTO comment_mentions(id,entry_id,kind,principal_id,handle) VALUES($1,$2,$3,$4,$5) ON CONFLICT(entry_id,kind,principal_id) DO NOTHING",
			token.New(), entryID, principal.Kind, principal.ID, principal.Handle); err != nil {
			return err
		}
	}
	return nil
}

func emitMentions(ctx context.Context, tx pgx.Tx, actor, sectionID, recordID, entryID, parentID, body string, principals []domain.Principal) error {
	if len(principals) == 0 {
		return nil
	}
	author, err := lookupAuthor(ctx, tx, actor)
	if err != nil {
		return err
	}
	for _, principal := range principals {
		if err = EmitCommentMentioned(ctx, tx, actor, sectionID, recordID, CommentMention{
			EntryID: entryID, ParentID: parentID, Body: body, Mentioned: principal, Author: author,
		}); err != nil {
			return err
		}
	}
	return nil
}

func lookupAuthor(ctx context.Context, source reader, actor string) (domain.Author, error) {
	author, err := authorFromActor(actor)
	if err != nil {
		return domain.Author{}, err
	}
	query := "SELECT COALESCE(NULLIF(btrim(name),''),handle),handle FROM users WHERE id=$1"
	if author.Kind == domain.PrincipalAgent {
		query = "SELECT name,handle FROM agents WHERE id=$1"
	}
	err = source.QueryRow(ctx, query, author.ID).Scan(&author.Name, &author.Handle)
	if errors.Is(err, pgx.ErrNoRows) {
		return author, nil
	}
	return author, err
}

type commentRow struct {
	sectionID, recordID, parentID string
	authorKind, authorID          string
	body                          string
	deleted                       bool
}

func lockComment(ctx context.Context, tx pgx.Tx, commentID string) (commentRow, error) {
	var row commentRow
	var kind string
	err := tx.QueryRow(ctx, `SELECT section_id,record_id,type,COALESCE(parent_id,''),author_kind,author_id,summary,deleted_at IS NOT NULL
		FROM activities WHERE id=$1 FOR UPDATE`, commentID).Scan(&row.sectionID, &row.recordID, &kind, &row.parentID, &row.authorKind, &row.authorID, &row.body, &row.deleted)
	if err != nil {
		return commentRow{}, classifyMissingRow(err)
	}
	if kind != domain.CommentType || row.deleted {
		return commentRow{}, ErrNotFound
	}
	return row, nil
}

// EditComment changes a comment's text. Only its author may edit. The mention set becomes the
// @handles in the new text plus any explicit ids; people newly mentioned are notified once.
func (repository *Repository) EditComment(ctx context.Context, actor, commentID, body string, mentions []string) (CommentResult, error) {
	return repository.editComment(ctx, actor, "", commentID, body, mentions)
}

// EditSectionComment binds an agent operation to its authorized section inside the
// transaction. A comment ID from another section cannot bypass the tool's grant.
func (repository *Repository) EditSectionComment(ctx context.Context, actor, sectionID, commentID, body string, mentions []string) (CommentResult, error) {
	return repository.editComment(ctx, actor, sectionID, commentID, body, mentions)
}

func (repository *Repository) editComment(ctx context.Context, actor, sectionID, commentID, body string, mentions []string) (CommentResult, error) {
	body, err := domain.ValidateCommentBody(body)
	if err != nil {
		return CommentResult{}, err
	}
	refs, err := parseMentionRefs(mentions)
	if err != nil {
		return CommentResult{}, err
	}
	author, err := authorFromActor(actor)
	if err != nil {
		return CommentResult{}, err
	}
	transaction, err := repository.pool.Begin(ctx)
	if err != nil {
		return CommentResult{}, err
	}
	defer func() { _ = transaction.Rollback(ctx) }()
	row, err := lockComment(ctx, transaction, commentID)
	if err != nil {
		return CommentResult{}, err
	}
	if sectionID != "" && row.sectionID != sectionID {
		return CommentResult{}, ErrNotFound
	}
	if row.authorKind != author.Kind || row.authorID != author.ID {
		return CommentResult{}, ErrForbidden
	}
	resolved, unresolved, err := resolveMentions(ctx, transaction, row.sectionID, author, domain.ParseMentionHandles(body), refs)
	if err != nil {
		return CommentResult{}, err
	}
	existing := map[string]bool{}
	existingRows, err := transaction.Query(ctx, "SELECT kind,principal_id FROM comment_mentions WHERE entry_id=$1", commentID)
	if err != nil {
		return CommentResult{}, err
	}
	for existingRows.Next() {
		var kind, identifier string
		if err = existingRows.Scan(&kind, &identifier); err != nil {
			existingRows.Close()
			return CommentResult{}, err
		}
		existing[kind+":"+identifier] = true
	}
	existingRows.Close()
	if err = existingRows.Err(); err != nil {
		return CommentResult{}, err
	}
	keep := map[string]bool{}
	added := []domain.Principal{}
	for _, principal := range resolved {
		key := principal.Kind + ":" + principal.ID
		keep[key] = true
		if !existing[key] {
			added = append(added, principal)
		}
	}
	removed := 0
	for key := range existing {
		if keep[key] {
			continue
		}
		kind, identifier, _ := strings.Cut(key, ":")
		if _, err = transaction.Exec(ctx, "DELETE FROM comment_mentions WHERE entry_id=$1 AND kind=$2 AND principal_id=$3", commentID, kind, identifier); err != nil {
			return CommentResult{}, err
		}
		removed++
	}
	if body != row.body || len(added) > 0 || removed > 0 {
		if _, err = transaction.Exec(ctx, "UPDATE activities SET summary=$2,edited_at=now() WHERE id=$1", commentID, body); err != nil {
			return CommentResult{}, err
		}
		if err = insertMentions(ctx, transaction, commentID, added); err != nil {
			return CommentResult{}, err
		}
		if _, err = transaction.Exec(ctx, "INSERT INTO audit(actor,action,section_id,record_id,detail) VALUES($1,'comment_edit',$2,$3,$4)", actor, row.sectionID, row.recordID, commentID); err != nil {
			return CommentResult{}, err
		}
		if err = emitMentions(ctx, transaction, actor, row.sectionID, row.recordID, commentID, row.parentID, body, added); err != nil {
			return CommentResult{}, err
		}
	}
	comment, err := fetchComment(ctx, transaction, commentID)
	if err != nil {
		return CommentResult{}, err
	}
	if err = transaction.Commit(ctx); err != nil {
		return CommentResult{}, err
	}
	return CommentResult{Comment: comment, UnresolvedMentions: unresolved}, nil
}

// DeleteComment removes a comment. Its author may delete it, and so may an administrator.
// A top-level comment that still has replies stays as a tombstone so the thread keeps its place.
func (repository *Repository) DeleteComment(ctx context.Context, actor, commentID string) error {
	return repository.deleteComment(ctx, actor, "", commentID)
}

// DeleteSectionComment applies the same author/tombstone rules as the browser and
// requires the locked comment to belong to the section authorized by the MCP tool.
func (repository *Repository) DeleteSectionComment(ctx context.Context, actor, sectionID, commentID string) error {
	return repository.deleteComment(ctx, actor, sectionID, commentID)
}

func (repository *Repository) deleteComment(ctx context.Context, actor, sectionID, commentID string) error {
	author, err := authorFromActor(actor)
	if err != nil {
		return err
	}
	transaction, err := repository.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = transaction.Rollback(ctx) }()
	row, err := lockComment(ctx, transaction, commentID)
	if err != nil {
		return err
	}
	if sectionID != "" && row.sectionID != sectionID {
		return ErrNotFound
	}
	if row.authorKind != author.Kind || row.authorID != author.ID {
		var role string
		if author.Kind != domain.PrincipalUser {
			return ErrForbidden
		}
		if err = transaction.QueryRow(ctx, "SELECT role FROM users WHERE id=$1", author.ID).Scan(&role); err != nil || role != domain.RoleAdmin {
			if err != nil && !errors.Is(err, pgx.ErrNoRows) {
				return err
			}
			return ErrForbidden
		}
	}
	var hasReplies bool
	if err = transaction.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM activities WHERE parent_id=$1 AND deleted_at IS NULL)", commentID).Scan(&hasReplies); err != nil {
		return err
	}
	if hasReplies {
		if _, err = transaction.Exec(ctx, "UPDATE activities SET summary='',deleted_at=now() WHERE id=$1", commentID); err != nil {
			return err
		}
		if _, err = transaction.Exec(ctx, "DELETE FROM comment_mentions WHERE entry_id=$1", commentID); err != nil {
			return err
		}
	} else {
		if _, err = transaction.Exec(ctx, "DELETE FROM activities WHERE id=$1", commentID); err != nil {
			return err
		}
		if row.parentID != "" {
			// The last reply under a tombstone takes the tombstone with it.
			if _, err = transaction.Exec(ctx, `DELETE FROM activities WHERE id=$1 AND deleted_at IS NOT NULL
				AND NOT EXISTS(SELECT 1 FROM activities WHERE parent_id=$1 AND deleted_at IS NULL)`, row.parentID); err != nil {
				return err
			}
		}
	}
	if _, err = transaction.Exec(ctx, "INSERT INTO audit(actor,action,section_id,record_id,detail) VALUES($1,'comment_delete',$2,$3,$4)", actor, row.sectionID, row.recordID, commentID); err != nil {
		return err
	}
	return transaction.Commit(ctx)
}

type commentCursor struct {
	CreatedAt time.Time `json:"t"`
	ID        string    `json:"i"`
}

func encodeCommentCursor(createdAt time.Time, id string) string {
	encoded, _ := json.Marshal(commentCursor{CreatedAt: createdAt, ID: id})
	return base64.RawURLEncoding.EncodeToString(encoded)
}

func decodeCommentCursor(value string) (commentCursor, error) {
	var cursor commentCursor
	raw, err := base64.RawURLEncoding.DecodeString(value)
	if err == nil {
		err = json.Unmarshal(raw, &cursor)
	}
	if err != nil || cursor.ID == "" || cursor.CreatedAt.IsZero() {
		return commentCursor{}, domain.Invalid("Invalid cursor")
	}
	return cursor, nil
}

// ListComments returns one page of top-level comments, newest first, each with its replies oldest first.
// Callers enforce section read access for agents.
func (repository *Repository) ListComments(ctx context.Context, sectionID, recordID string, limit int, cursor string) (domain.CommentPage, error) {
	if limit == 0 {
		limit = domain.DefaultCommentPage
	}
	if limit < 0 || limit > domain.MaxCommentPage {
		return domain.CommentPage{}, domain.Invalid("Limit must be between 1 and 200")
	}
	var exists bool
	if err := repository.pool.QueryRow(ctx, "SELECT true FROM records WHERE section_id=$1 AND id=$2", sectionID, recordID).Scan(&exists); err != nil {
		return domain.CommentPage{}, classifyMissingRow(err)
	}
	query := "SELECT " + activityColumns + " " + activityFrom + " WHERE a.section_id=$1 AND a.record_id=$2 AND a.type='comment' AND a.parent_id IS NULL"
	arguments := []any{sectionID, recordID}
	if cursor != "" {
		position, err := decodeCommentCursor(cursor)
		if err != nil {
			return domain.CommentPage{}, err
		}
		query += " AND (a.created_at, a.id) < ($3::timestamptz, $4::text)"
		arguments = append(arguments, position.CreatedAt, position.ID)
	}
	query += " ORDER BY a.created_at DESC, a.id DESC LIMIT " + strconv.Itoa(limit+1)
	rows, err := repository.pool.Query(ctx, query, arguments...)
	if err != nil {
		return domain.CommentPage{}, err
	}
	defer rows.Close()
	entries := []domain.Activity{}
	for rows.Next() {
		activity, scanErr := scanActivity(rows)
		if scanErr != nil {
			return domain.CommentPage{}, scanErr
		}
		entries = append(entries, activity)
	}
	if err = rows.Err(); err != nil {
		return domain.CommentPage{}, err
	}
	rows.Close()
	page := domain.CommentPage{Comments: []domain.Comment{}}
	if len(entries) > limit {
		entries = entries[:limit]
		last := entries[len(entries)-1]
		page.NextCursor = encodeCommentCursor(last.CreatedAt, last.ID)
	}
	if len(entries) == 0 {
		return page, nil
	}
	parents := make([]string, len(entries))
	for index, entry := range entries {
		parents[index] = entry.ID
	}
	replyRows, err := repository.pool.Query(ctx, "SELECT "+activityColumns+" "+activityFrom+" WHERE a.parent_id = ANY($1) AND a.type='comment' ORDER BY a.created_at, a.id", parents)
	if err != nil {
		return domain.CommentPage{}, err
	}
	defer replyRows.Close()
	replies := []domain.Activity{}
	for replyRows.Next() {
		activity, scanErr := scanActivity(replyRows)
		if scanErr != nil {
			return domain.CommentPage{}, scanErr
		}
		replies = append(replies, activity)
	}
	if err = replyRows.Err(); err != nil {
		return domain.CommentPage{}, err
	}
	replyRows.Close()
	everything := append(append([]domain.Activity{}, entries...), replies...)
	if err = attachMentions(ctx, repository.pool, everything); err != nil {
		return domain.CommentPage{}, err
	}
	byParent := map[string][]domain.Comment{}
	for _, reply := range everything[len(entries):] {
		byParent[reply.ParentID] = append(byParent[reply.ParentID], commentFromActivity(reply, sectionID, recordID))
	}
	for _, entry := range everything[:len(entries)] {
		comment := commentFromActivity(entry, sectionID, recordID)
		comment.Replies = byParent[entry.ID]
		page.Comments = append(page.Comments, comment)
	}
	return page, nil
}

// mentionVisible limits an agent's notifications to sections it can currently read.
const mentionVisible = `m.kind=$1 AND m.principal_id=$2 AND ($1='user' OR EXISTS(
	SELECT 1 FROM permissions p WHERE p.agent_id=$2 AND p.section_id=a.section_id AND p.can_read))`

// ListMentions returns the notifications for one user or agent, newest first, and the unread count.
// An agent only sees mentions in sections it can read now.
func (repository *Repository) ListMentions(ctx context.Context, kind, principalID string, unreadOnly bool, limit int) ([]domain.MentionNotification, int, error) {
	if limit == 0 {
		limit = 100
	}
	if limit < 0 || limit > domain.MaxPageSize {
		return nil, 0, domain.Invalid("Limit must be between 1 and 500")
	}
	rows, err := repository.pool.Query(ctx, `SELECT m.id,m.entry_id,a.section_id,s.name,a.record_id,COALESCE(r.data->>'name',''),COALESCE(a.parent_id,''),
		a.author_kind,a.author_id,
		COALESCE(CASE WHEN a.author_kind='user' THEN COALESCE(NULLIF(btrim(u.name),''),u.handle) ELSE g.name END,''),
		COALESCE(u.handle,g.handle,''),a.summary,m.created_at,m.read_at
		FROM comment_mentions m
		JOIN activities a ON a.id=m.entry_id
		JOIN sections s ON s.id=a.section_id
		LEFT JOIN records r ON r.id=a.record_id AND r.section_id=a.section_id
		LEFT JOIN users u ON a.author_kind='user' AND u.id=a.author_id
		LEFT JOIN agents g ON a.author_kind='agent' AND g.id=a.author_id
		WHERE `+mentionVisible+` AND ($3 OR m.read_at IS NULL)
		ORDER BY m.created_at DESC, m.id DESC LIMIT $4`, kind, principalID, !unreadOnly, limit)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	mentions := []domain.MentionNotification{}
	for rows.Next() {
		var mention domain.MentionNotification
		if err = rows.Scan(&mention.ID, &mention.EntryID, &mention.SectionID, &mention.SectionName, &mention.RecordID, &mention.RecordName, &mention.ParentID,
			&mention.Author.Kind, &mention.Author.ID, &mention.Author.Name, &mention.Author.Handle, &mention.Body, &mention.CreatedAt, &mention.ReadAt); err != nil {
			return nil, 0, err
		}
		mention.Body = clipText(mention.Body, domain.MentionBodyLimit)
		mentions = append(mentions, mention)
	}
	if err = rows.Err(); err != nil {
		return nil, 0, err
	}
	rows.Close()
	var unread int
	err = repository.pool.QueryRow(ctx, `SELECT count(*) FROM comment_mentions m JOIN activities a ON a.id=m.entry_id
		WHERE `+mentionVisible+` AND m.read_at IS NULL`, kind, principalID).Scan(&unread)
	return mentions, unread, err
}

// MarkMentionsRead marks the caller's own notifications as read and reports how many changed.
// Ids that belong to someone else or are already read are ignored.
func (repository *Repository) MarkMentionsRead(ctx context.Context, kind, principalID string, ids []string, all bool) (int, error) {
	if !all && len(ids) == 0 {
		return 0, domain.Invalid("Choose mentions to mark as read")
	}
	if len(ids) > domain.MaxPageSize {
		return 0, domain.Invalid("Mark at most 500 mentions at a time")
	}
	tag, err := repository.pool.Exec(ctx, "UPDATE comment_mentions SET read_at=now() WHERE kind=$1 AND principal_id=$2 AND read_at IS NULL AND ($3 OR id = ANY($4))", kind, principalID, all, ids)
	if err != nil {
		return 0, err
	}
	return int(tag.RowsAffected()), nil
}

// MentionCandidates lists who can be @mentioned on a section: every team member and
// every agent with read access to it.
func (repository *Repository) MentionCandidates(ctx context.Context, sectionID string) (users, agents []domain.Principal, err error) {
	var exists bool
	if err = repository.pool.QueryRow(ctx, "SELECT true FROM sections WHERE id=$1", sectionID).Scan(&exists); err != nil {
		return nil, nil, classifyMissingRow(err)
	}
	collect := func(kind, query string, arguments ...any) ([]domain.Principal, error) {
		rows, queryErr := repository.pool.Query(ctx, query, arguments...)
		if queryErr != nil {
			return nil, queryErr
		}
		defer rows.Close()
		principals := []domain.Principal{}
		for rows.Next() {
			principal := domain.Principal{Kind: kind}
			if scanErr := rows.Scan(&principal.ID, &principal.Handle, &principal.Name); scanErr != nil {
				return nil, scanErr
			}
			principals = append(principals, principal)
		}
		return principals, rows.Err()
	}
	if users, err = collect(domain.PrincipalUser, "SELECT id,handle,COALESCE(NULLIF(btrim(name),''),handle) FROM users ORDER BY lower(COALESCE(NULLIF(btrim(name),''),handle)),handle"); err != nil {
		return nil, nil, err
	}
	agents, err = collect(domain.PrincipalAgent, `SELECT a.id,a.handle,a.name FROM agents a
		JOIN permissions p ON p.agent_id=a.id AND p.section_id=$1 AND p.can_read ORDER BY lower(a.name),a.handle`, sectionID)
	return users, agents, err
}
