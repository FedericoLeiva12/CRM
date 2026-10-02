package domain

import "testing"

func TestFieldValidation(t *testing.T) {
	fields := []Field{{ID: "name", Label: "Name", Type: "text", Required: true}, {ID: "value", Label: "Value", Type: "number"}, {ID: "email", Label: "Email", Type: "email"}, {ID: "date", Label: "Date", Type: "date"}, {ID: "active", Label: "Active", Type: "boolean"}}
	for _, input := range []map[string]any{{}, {"name": "a", "unknown": "x"}, {"name": "a", "value": "bad"}, {"name": "a", "email": "bad"}, {"name": "a", "date": "2026-99-99"}, {"name": "a", "active": "yes"}} {
		if ValidateRecord(fields, input) == nil {
			t.Fatalf("Accepted invalid input: %v", input)
		}
	}
	if e := ValidateRecord(fields, map[string]any{"name": "Tess Ward", "value": float64(2500), "email": "tess@example.test", "date": "2026-10-02", "active": true}); e != nil {
		t.Fatal(e)
	}
}
