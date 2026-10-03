package itemviews

import (
	"strings"
	"testing"

	"siracrm/internal/domain"
)

func TestViewConfigurationInvariants(t *testing.T) {
	info := domain.SectionView{ID: Info, Enabled: true}
	for _, test := range []struct {
		name  string
		views []domain.SectionView
	}{
		{"missing info", []domain.SectionView{{ID: "activity", Enabled: true}}},
		{"disabled info", []domain.SectionView{{ID: Info, Enabled: false}}},
		{"duplicate view", []domain.SectionView{info, info}},
		{"unregistered renderer", []domain.SectionView{info, {ID: "agent", Enabled: true}}},
		{"unregistered configuration", []domain.SectionView{info, {ID: "activity", Enabled: true, Config: map[string]any{"system_prompt": "unexpected"}}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := Validate(test.views); !domain.IsValidationError(err) {
				t.Fatalf("expected validation error, got %v", err)
			}
		})
	}
	views, err := Validate([]domain.SectionView{{ID: "activity", Enabled: false}, info})
	if err != nil || len(views) != 2 || views[0].ID != Info || !views[0].Enabled || views[1].Enabled || views[1].Config == nil {
		t.Fatalf("unexpected normalized views: %+v %v", views, err)
	}
}

func TestConfigurationFieldValidation(t *testing.T) {
	definition := Definition{Label: "Assistant", ConfigFields: []ConfigField{{Key: "system_prompt", Label: "System prompt", Type: "textarea", Required: true, MaxLength: 5}}}
	for _, config := range []map[string]any{nil, {"system_prompt": "   "}, {"system_prompt": 123}, {"system_prompt": strings.Repeat("é", 6)}, {"secret": "key"}} {
		if err := validateConfig(definition, config); !domain.IsValidationError(err) {
			t.Fatalf("expected invalid config %#v, got %v", config, err)
		}
	}
	if err := validateConfig(definition, map[string]any{"system_prompt": "café"}); err != nil {
		t.Fatal(err)
	}
}
