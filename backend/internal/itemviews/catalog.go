// Package itemviews defines the supported item views and their configuration contract.
// Persistence and HTTP use this catalog; registering a view does not require a new route.
package itemviews

import (
	"strings"
	"unicode/utf8"

	"siracrm/internal/domain"
)

const Info = "info"

// ConfigField describes a non-secret section-level setting rendered by the web editor.
// Credentials belong in server configuration, never in section view configuration.
type ConfigField struct {
	Key       string `json:"key"`
	Label     string `json:"label"`
	Type      string `json:"type"`
	Required  bool   `json:"required"`
	MaxLength int    `json:"max_length"`
}

type Definition struct {
	ID           string        `json:"id"`
	Label        string        `json:"label"`
	Description  string        `json:"description"`
	Required     bool          `json:"required"`
	ConfigFields []ConfigField `json:"config_fields"`
}

// Catalog is the explicit extension point. Add metadata here and a frontend renderer
// with the same ID. Only Info is installed automatically on existing or new sections.
func Catalog() []Definition {
	return []Definition{
		{ID: Info, Label: "Info", Description: "Record fields, displayed in read-only mode until you choose Edit.", Required: true, ConfigFields: []ConfigField{}},
		{ID: "activity", Label: "Activity", Description: "Comments, mentions, timeline entries and related records.", ConfigFields: []ConfigField{}},
	}
}

func DefaultViews() []domain.SectionView {
	return []domain.SectionView{{ID: Info, Enabled: true, Config: map[string]any{}}}
}

// Validate rejects attempts to remove or disable Info, unknown views and unsupported
// configuration. Returning normalized copies prevents nil configs reaching JSONB.
func Validate(views []domain.SectionView) ([]domain.SectionView, error) {
	if len(views) > 32 {
		return nil, domain.Invalid("Too many item views")
	}
	definitions := map[string]Definition{}
	for _, definition := range Catalog() {
		definitions[definition.ID] = definition
	}
	seen := map[string]bool{}
	result := DefaultViews()
	for _, view := range views {
		definition, known := definitions[view.ID]
		if !known {
			return nil, domain.Invalid("Unknown item view: " + view.ID)
		}
		if seen[view.ID] {
			return nil, domain.Invalid("Duplicate item view: " + view.ID)
		}
		seen[view.ID] = true
		if definition.Required && !view.Enabled {
			return nil, domain.Invalid("Info is always enabled")
		}
		if err := validateConfig(definition, view.Config); err != nil {
			return nil, err
		}
		if view.ID == Info {
			continue
		}
		if view.Config == nil {
			view.Config = map[string]any{}
		}
		result = append(result, view)
	}
	if !seen[Info] {
		return nil, domain.Invalid("Info cannot be removed")
	}
	return result, nil
}

func validateConfig(definition Definition, config map[string]any) error {
	fields := map[string]ConfigField{}
	for _, field := range definition.ConfigFields {
		fields[field.Key] = field
	}
	for key, value := range config {
		field, known := fields[key]
		if !known {
			return domain.Invalid("Unknown setting for " + definition.Label + ": " + key)
		}
		text, valid := value.(string)
		if !valid {
			return domain.Invalid(field.Label + " must be text")
		}
		if field.MaxLength > 0 && utf8.RuneCountInString(text) > field.MaxLength {
			return domain.Invalid(field.Label + " is too long")
		}
	}
	for _, field := range definition.ConfigFields {
		value, _ := config[field.Key].(string)
		if field.Required && strings.TrimSpace(value) == "" {
			return domain.Invalid(field.Label + " is required")
		}
	}
	return nil
}
