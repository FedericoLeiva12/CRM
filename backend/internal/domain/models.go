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
