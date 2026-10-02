package domain

import "testing"

func TestMergeAndConvert(t *testing.T) {
	fields := []Field{{ID: "name", Label: "Name", Type: FieldText, Required: true}, {ID: "status", Label: "Status", Type: FieldText}, {ID: "notes", Label: "Notes", Type: FieldText}}
	existing := map[string]any{"name": "Mara", "status": "Contactado", "notes": "Keep"}
	merged := MergeRecord(existing, map[string]any{"status": "Ganado", "notes": nil})
	if merged["name"] != "Mara" || merged["status"] != "Ganado" {
		t.Fatalf("partial merge changed the wrong fields: %#v", merged)
	}
	if _, present := merged["notes"]; present {
		t.Fatal("null did not clear notes")
	}
	if existing["notes"] != "Keep" {
		t.Fatal("merge mutated the original record")
	}
	if err := ValidateRecord(fields, merged); err != nil {
		t.Fatal(err)
	}
	if err := ValidateRecord(fields, MergeRecord(existing, map[string]any{"name": nil})); err == nil {
		t.Fatal("clearing a required field was accepted")
	}
	sourceFields := []Field{{ID: "name", Label: "Name", Type: FieldText, Required: true}, {ID: "email", Label: "Email", Type: FieldEmail}, {ID: "source_note", Label: "Source", Type: FieldText}}
	targetFields := []Field{{ID: "name", Label: "Name", Type: FieldText, Required: true}, {ID: "email", Label: "Email", Type: FieldEmail}, {ID: "alias", Label: "Alias", Type: FieldText}}
	copied, err := ConvertValues(sourceFields, targetFields, map[string]any{"name": "Mara", "email": "mara@example.test", "source_note": "From the fair"}, map[string]string{"source_note": "alias"}, map[string]any{"email": "desk@example.test"})
	if err != nil {
		t.Fatal(err)
	}
	if copied["name"] != "Mara" || copied["alias"] != "From the fair" || copied["email"] != "desk@example.test" {
		t.Fatalf("conversion copy was wrong: %#v", copied)
	}
	if _, present := copied["source_note"]; present {
		t.Fatal("unmatched field was copied")
	}
	withStatus, err := ApplyStatus(fields, existing, strPtr("Ganado"))
	if err != nil || withStatus["status"] != "Ganado" || existing["status"] != "Contactado" {
		t.Fatal(withStatus, err)
	}
}

func TestListAndActivityValidation(t *testing.T) {
	fields := []Field{{ID: "status", Label: "Status", Type: FieldText}, {ID: "next_action_at", Label: "Next", Type: FieldDate}, {ID: "value", Label: "Value", Type: FieldNumber}}
	query := ListQuery{Limit: MaxPageSize, Filters: []Filter{{Field: "status", Op: OpEq, Value: "Contactado"}, {Field: "next_action_at", Op: OpLte, Value: "2026-10-05"}}, Sort: &Sort{Field: "value", Direction: "desc"}}
	if err := ValidateListQuery(fields, query); err != nil {
		t.Fatal(err)
	}
	for _, query := range []ListQuery{
		{Limit: MaxPageSize + 1},
		{Limit: MaxPageSize, Filters: []Filter{{Field: "missing", Op: OpEq, Value: "x"}}},
		{Limit: MaxPageSize, Filters: []Filter{{Field: "status", Op: OpGt, Value: "x"}}},
		{Limit: MaxPageSize, Filters: []Filter{{Field: "value", Op: OpContains, Value: "1"}}},
		{Limit: MaxPageSize, Sort: &Sort{Field: "status", Direction: "sideways"}},
	} {
		if err := ValidateListQuery(fields, query); err == nil {
			t.Fatalf("accepted invalid query %#v", query)
		}
	}
	if _, err := ValidateActivity("comment", "2026-10-02", "Called back", "phone", "thread-1"); err != nil {
		t.Fatal(err)
	}
	if _, err := ValidateActivity("", "2026-10-02", "Missing type", "", ""); err == nil {
		t.Fatal("blank activity type was accepted")
	}
}

func strPtr(value string) *string { return &value }
