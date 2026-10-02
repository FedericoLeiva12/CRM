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
	ID          string       `json:"id"`
	Name        string       `json:"name"`
	Permissions []Permission `json:"permissions"`
}

// Access distinguishes independent read and write grants; write does not imply read.
type Access int

const (
	ReadAccess Access = iota
	WriteAccess
)
