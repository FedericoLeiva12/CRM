// Package domain defines transport-independent CRM models and validation rules.
package domain

import "time"

type FieldType string

const (
	FieldText    FieldType = "text"
	FieldEmail   FieldType = "email"
	FieldNumber  FieldType = "number"
	FieldDate    FieldType = "date"
	FieldBoolean FieldType = "boolean"
)

type Field struct {
	ID       string    `json:"id"`
	Label    string    `json:"label"`
	Type     FieldType `json:"type"`
	Required bool      `json:"required"`
}
type Section struct {
	ID     string  `json:"id"`
	Name   string  `json:"name"`
	Fields []Field `json:"fields"`
}
type Record struct {
	ID        string         `json:"id"`
	Data      map[string]any `json:"data"`
	UpdatedAt time.Time      `json:"updated_at"`
}
type Permission struct {
	SectionID string `json:"section_id"`
	Read      bool   `json:"read"`
	Write     bool   `json:"write"`
}
type Agent struct {
	ID           string       `json:"id"`
	Name         string       `json:"name"`
	Handle       string       `json:"handle"`
	ManageSchema bool         `json:"manage_schema"`
	Permissions  []Permission `json:"permissions"`
}

const (
	RoleAdmin  = "admin"
	RoleMember = "member"
)

func ValidRole(role string) bool {
	return role == RoleAdmin || role == RoleMember
}

type WorkspaceUser struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	Name      string    `json:"name"`
	Handle    string    `json:"handle"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"created_at"`
}
type Invite struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

// CreatedInvite is returned once. Token is the secret path segment; only its hash is stored.
type CreatedInvite struct {
	Invite
	Token string `json:"token"`
}

// Access distinguishes independent read and write grants; write does not imply read.
type Access int

const (
	ReadAccess Access = iota
	WriteAccess
)

// MaxPageSize is the largest record page agents and the repository will return.
const MaxPageSize = 500

// Activity is one timeline entry. Type is an open string; a comment is type
// "comment" and keeps its text in Summary. ParentID, EditedAt, Deleted, and
// Mentions are set only on comments.
type Activity struct {
	ID        string      `json:"id"`
	Type      string      `json:"type"`
	Date      time.Time   `json:"date"`
	Summary   string      `json:"summary"`
	Channel   string      `json:"channel,omitempty"`
	Ref       string      `json:"ref,omitempty"`
	Author    Author      `json:"author"`
	CreatedAt time.Time   `json:"created_at"`
	ParentID  string      `json:"parent_id,omitempty"`
	EditedAt  *time.Time  `json:"edited_at,omitempty"`
	Deleted   bool        `json:"deleted,omitempty"`
	Mentions  []Principal `json:"mentions,omitempty"`
}

// Author is the user or agent who appended a timeline entry. Name and Handle
// are filled when the account still exists.
type Author struct {
	Kind   string `json:"kind"`
	ID     string `json:"id"`
	Name   string `json:"name,omitempty"`
	Handle string `json:"handle,omitempty"`
}

// RecordLink is the other side of a generic record-to-record relationship.
type RecordLink struct {
	SectionID   string `json:"section_id"`
	SectionName string `json:"section_name"`
	RecordID    string `json:"record_id"`
	Direction   string `json:"direction"`
	Name        string `json:"name,omitempty"`
}

const (
	LinkOutgoing = "outgoing"
	LinkIncoming = "incoming"
)

// Filter is one comparison. Filters on a list are combined with AND.
type Filter struct {
	Field string `json:"field"`
	Op    string `json:"op"`
	Value any    `json:"value,omitempty"`
}

// Sort selects one field and an asc or desc direction.
type Sort struct {
	Field     string `json:"field"`
	Direction string `json:"direction"`
}

// ListQuery is the optional search contract. Limit 0 means the caller has not chosen one yet.
type ListQuery struct {
	Filters []Filter
	Search  string
	Sort    *Sort
	Limit   int
	Cursor  string
}

// ListPage is one filtered, ordered page. NextCursor is empty on the last page.
type ListPage struct {
	Records    []Record
	Limit      int
	NextCursor string
	Total      int
}
