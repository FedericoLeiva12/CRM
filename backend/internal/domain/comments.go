package domain

import (
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	CommentType = "comment"
	// MaxCommentRunes is the longest comment body, in characters.
	MaxCommentRunes = 4000
	// MaxMentionsPerComment bounds fan-out: each mention is a notification and a webhook event.
	MaxMentionsPerComment = 20
	// DefaultCommentPage and MaxCommentPage bound a page of top-level comments.
	DefaultCommentPage = 50
	MaxCommentPage     = 200
	// MentionBodyLimit is how much of a comment body notifications and events carry.
	MentionBodyLimit = 500
)

const (
	PrincipalUser  = "user"
	PrincipalAgent = "agent"
)

// Principal is a user or an agent with the handle people type after @.
type Principal struct {
	Kind   string `json:"kind"`
	ID     string `json:"id"`
	Handle string `json:"handle"`
	Name   string `json:"name,omitempty"`
}

// Comment is a timeline entry of type comment. Replies are one level deep and
// are present only on top-level comments, oldest first. A deleted comment that
// still has replies stays as a tombstone with an empty body.
type Comment struct {
	ID        string      `json:"id"`
	SectionID string      `json:"section_id"`
	RecordID  string      `json:"record_id"`
	ParentID  string      `json:"parent_id,omitempty"`
	Body      string      `json:"body"`
	Author    Author      `json:"author"`
	Mentions  []Principal `json:"mentions"`
	CreatedAt time.Time   `json:"created_at"`
	EditedAt  *time.Time  `json:"edited_at,omitempty"`
	Deleted   bool        `json:"deleted,omitempty"`
	Replies   []Comment   `json:"replies,omitempty"`
}

// CommentPage is one page of top-level comments, newest first.
type CommentPage struct {
	Comments   []Comment
	NextCursor string
}

// MentionNotification is one mention of the calling user or agent.
type MentionNotification struct {
	ID          string     `json:"id"`
	EntryID     string     `json:"entry_id"`
	SectionID   string     `json:"section_id"`
	SectionName string     `json:"section_name"`
	RecordID    string     `json:"record_id"`
	RecordName  string     `json:"record_name,omitempty"`
	ParentID    string     `json:"parent_id,omitempty"`
	Author      Author     `json:"author"`
	Body        string     `json:"body"`
	CreatedAt   time.Time  `json:"created_at"`
	ReadAt      *time.Time `json:"read_at"`
}

// ValidateCommentBody trims and checks a comment body. Bodies are plain text;
// line breaks are kept and rendering decides how to show them.
func ValidateCommentBody(body string) (string, error) {
	body = strings.TrimSpace(body)
	if body == "" {
		return "", Invalid("Write a comment")
	}
	if utf8.RuneCountInString(body) > MaxCommentRunes {
		return "", Invalid("Comments must be 4000 characters or fewer")
	}
	if strings.ContainsRune(body, 0) || !utf8.ValidString(body) {
		return "", Invalid("Comments cannot contain control characters")
	}
	return body, nil
}

var (
	fencedCode    = regexp.MustCompile("(?s)```.*?(```|$)")
	inlineCode    = regexp.MustCompile("`[^`\n]*`")
	mentionHandle = regexp.MustCompile(`(?:^|[^A-Za-z0-9_@/.+-])@([A-Za-z0-9][A-Za-z0-9_-]{0,31})`)
)

// ParseMentionHandles returns the distinct lowercase @handles in a comment
// body, in order of first appearance. A handle must start the text or follow
// a character that cannot be part of a word, so an email address is not a
// mention. Text inside code spans and fenced code is ignored. Whether a
// handle belongs to anyone is decided against the database, not here.
func ParseMentionHandles(body string) []string {
	body = fencedCode.ReplaceAllString(body, " ")
	body = inlineCode.ReplaceAllString(body, " ")
	seen := map[string]struct{}{}
	handles := []string{}
	for _, match := range mentionHandle.FindAllStringSubmatch(body, -1) {
		handle := strings.ToLower(strings.TrimRight(match[1], "-_"))
		if handle == "" {
			continue
		}
		if _, duplicate := seen[handle]; duplicate {
			continue
		}
		seen[handle] = struct{}{}
		handles = append(handles, handle)
	}
	return handles
}

// MentionRef is an explicit mention by principal id. Kind is empty when the
// caller did not say whether the id belongs to a user or an agent.
type MentionRef struct {
	Kind string
	ID   string
}

// ParseMentionRef accepts "<id>", "user:<id>", or "agent:<id>".
func ParseMentionRef(raw string) (MentionRef, error) {
	raw = strings.TrimSpace(raw)
	kind := ""
	if prefix, rest, found := strings.Cut(raw, ":"); found {
		if prefix != PrincipalUser && prefix != PrincipalAgent {
			return MentionRef{}, Invalid("Mentions must be user or agent ids")
		}
		kind, raw = prefix, rest
	}
	if raw == "" || len(raw) > 80 {
		return MentionRef{}, Invalid("Mentions must be user or agent ids")
	}
	for _, char := range raw {
		if char < 0x21 || char > 0x7e {
			return MentionRef{}, Invalid("Mentions must be user or agent ids")
		}
	}
	return MentionRef{Kind: kind, ID: raw}, nil
}
