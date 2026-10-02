package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"reflect"
	"testing"
)

type queryPage struct {
	Records []struct {
		ID   string         `json:"id"`
		Data map[string]any `json:"data"`
	} `json:"records"`
	Limit      int     `json:"limit"`
	NextCursor *string `json:"next_cursor"`
	Total      int     `json:"total"`
}

func (page queryPage) names() []string {
	names := []string{}
	for _, record := range page.Records {
		names = append(names, fmt.Sprint(record.Data["name"]))
	}
	return names
}

// TestWebRecordQuery covers the browser path to the shared query layer using a
// section and fields created at runtime, so no section-specific behavior is relied on.
func TestWebRecordQuery(t *testing.T) {
	ctx := context.Background()
	pool, handler, cookie := startCRM(t, ctx)
	defer pool.Close()
	post := func(path string, body any, status int) *httptest.ResponseRecorder {
		t.Helper()
		response := apiCall(t, handler, cookie, "", "POST", path, body)
		expectStatus(t, response, status)
		return response
	}
	post("/api/sections", map[string]string{"id": "deals", "name": "Deals"}, 201)
	for _, field := range []map[string]any{
		{"id": "stage", "label": "Stage", "type": "text"},
		{"id": "amount", "label": "Amount", "type": "number"},
		{"id": "close_on", "label": "Close date", "type": "date"},
		{"id": "won", "label": "Won", "type": "boolean"},
		{"id": "contact", "label": "Contact", "type": "email"},
	} {
		field["required"] = false
		post("/api/sections/deals/fields", field, 201)
	}
	rows := []map[string]any{
		{"name": "Alpha 100% deal", "stage": "open", "amount": 100.0, "close_on": "2026-01-10", "won": false, "contact": "a@acme.test"},
		{"name": "Bravo", "stage": "won", "amount": 300.0, "close_on": "2026-02-10", "won": true, "contact": "b@globex.test"},
		{"name": "Charlie", "stage": "lost", "amount": 200.0, "close_on": "2026-03-10", "won": false},
		{"name": "Delta under_score", "stage": "open"},
		{"name": "Echo", "stage": "won", "amount": 400.0, "close_on": "2026-04-10", "won": true, "contact": "e@acme.test"},
	}
	for _, row := range rows {
		post("/api/sections/deals/records", map[string]any{"data": row}, 200)
	}
	query := func(body map[string]any) queryPage {
		t.Helper()
		response := post("/api/sections/deals/records/query", body, 200)
		var page queryPage
		if err := json.Unmarshal(response.Body.Bytes(), &page); err != nil {
			t.Fatal(err, response.Body.String())
		}
		return page
	}
	filter := func(field, op string, value any) map[string]any {
		return map[string]any{"field": field, "op": op, "value": value}
	}
	sortBy := func(field, direction string) map[string]any {
		return map[string]any{"field": field, "direction": direction}
	}
	expectNames := func(label string, page queryPage, want ...string) {
		t.Helper()
		if got := page.names(); len(got) != len(want) || (len(got) > 0 && !reflect.DeepEqual(got, want)) {
			t.Fatalf("%s: got %v, want %v", label, got, want)
		}
	}

	t.Run("requires a session", func(t *testing.T) {
		expectStatus(t, apiCall(t, handler, "", "", "POST", "/api/sections/deals/records/query", map[string]any{}), 401)
	})
	t.Run("members can query", func(t *testing.T) {
		invite := post("/api/invites", map[string]string{"email": "member@example.test", "role": "member"}, 201)
		var created map[string]string
		if err := json.Unmarshal(invite.Body.Bytes(), &created); err != nil {
			t.Fatal(err)
		}
		accepted := apiCall(t, handler, "", "", "POST", "/api/invite/"+created["token"], map[string]string{"name": "Member", "password": "member-long-password"})
		expectStatus(t, accepted, 201)
		memberCookie := accepted.Result().Cookies()[0].Name + "=" + accepted.Result().Cookies()[0].Value
		expectStatus(t, apiCall(t, handler, memberCookie, "", "POST", "/api/sections/deals/records/query", map[string]any{}), 200)
	})
	t.Run("empty query returns every record with a total", func(t *testing.T) {
		page := query(map[string]any{})
		if page.Total != 5 || len(page.Records) != 5 || page.NextCursor != nil || page.Limit != defaultPageSize {
			t.Fatalf("unexpected page: %+v", page)
		}
	})
	t.Run("search matches every word in any text field and escapes wildcards", func(t *testing.T) {
		expectNames("single word", query(map[string]any{"search": "ACME", "sort": sortBy("name", "asc")}), "Alpha 100% deal", "Echo")
		expectNames("words across fields", query(map[string]any{"search": "echo won"}), "Echo")
		expectNames("percent is literal", query(map[string]any{"search": "100%"}), "Alpha 100% deal")
		expectNames("underscore is literal", query(map[string]any{"search": "under_score"}), "Delta under_score")
		expectNames("no wildcard expansion", query(map[string]any{"search": "%"}), "Alpha 100% deal")
		expectNames("no match", query(map[string]any{"search": "zzz"}))
	})
	t.Run("filters by field type", func(t *testing.T) {
		sorted := func(filters ...map[string]any) queryPage {
			return query(map[string]any{"filters": filters, "sort": sortBy("name", "asc")})
		}
		expectNames("text contains", sorted(filter("stage", "contains", "OP")), "Alpha 100% deal", "Delta under_score")
		expectNames("text equals", sorted(filter("stage", "eq", "won")), "Bravo", "Echo")
		expectNames("text not equals", sorted(filter("stage", "neq", "open")), "Bravo", "Charlie", "Echo")
		expectNames("is any of", sorted(filter("stage", "in", []string{"lost", "won"})), "Bravo", "Charlie", "Echo")
		expectNames("number is any of", sorted(filter("amount", "in", []float64{100, 200})), "Alpha 100% deal", "Charlie")
		expectNames("number range", sorted(filter("amount", "gte", 200), filter("amount", "lte", 300)), "Bravo", "Charlie")
		expectNames("number greater", sorted(filter("amount", "gt", 200)), "Bravo", "Echo")
		expectNames("date range", sorted(filter("close_on", "gte", "2026-02-01"), filter("close_on", "lt", "2026-03-10")), "Bravo")
		expectNames("date equals", sorted(filter("close_on", "eq", "2026-03-10")), "Charlie")
		expectNames("boolean true", sorted(filter("won", "eq", true)), "Bravo", "Echo")
		expectNames("boolean false", sorted(filter("won", "eq", false)), "Alpha 100% deal", "Charlie")
		expectNames("empty", sorted(filter("amount", "is_empty", nil)), "Delta under_score")
		expectNames("not empty", sorted(filter("contact", "not_empty", nil)), "Alpha 100% deal", "Bravo", "Echo")
		expectNames("filters and search combine with AND", query(map[string]any{"search": "acme", "filters": []map[string]any{filter("won", "eq", true)}}), "Echo")
		if total := sorted(filter("stage", "eq", "won")).Total; total != 2 {
			t.Fatalf("total ignores filters: %d", total)
		}
	})
	t.Run("sorts ascending and descending with empty values last", func(t *testing.T) {
		expectNames("name desc", query(map[string]any{"sort": sortBy("name", "desc")}), "Echo", "Delta under_score", "Charlie", "Bravo", "Alpha 100% deal")
		expectNames("amount asc", query(map[string]any{"sort": sortBy("amount", "asc")}), "Alpha 100% deal", "Charlie", "Bravo", "Echo", "Delta under_score")
		expectNames("amount desc", query(map[string]any{"sort": sortBy("amount", "desc")}), "Echo", "Bravo", "Charlie", "Alpha 100% deal", "Delta under_score")
		expectNames("date asc", query(map[string]any{"sort": sortBy("close_on", "asc")}), "Alpha 100% deal", "Bravo", "Charlie", "Echo", "Delta under_score")
	})
	t.Run("cursor paging walks every record once for each sort", func(t *testing.T) {
		for _, sort := range []map[string]any{nil, sortBy("amount", "asc"), sortBy("amount", "desc"), sortBy("close_on", "desc"), sortBy("name", "asc"), sortBy("won", "asc")} {
			whole := query(map[string]any{"sort": sort})
			seen := []string{}
			cursor := ""
			for pageNumber := 0; pageNumber < 10; pageNumber++ {
				body := map[string]any{"sort": sort, "limit": 2}
				if cursor != "" {
					body["cursor"] = cursor
				}
				page := query(body)
				if page.Total != 5 {
					t.Fatalf("sort %v: total %d", sort, page.Total)
				}
				seen = append(seen, page.names()...)
				if page.NextCursor == nil {
					break
				}
				cursor = *page.NextCursor
			}
			if !reflect.DeepEqual(seen, whole.names()) {
				t.Fatalf("sort %v: paged %v, whole %v", sort, seen, whole.names())
			}
		}
	})
	t.Run("rejects invalid queries with a message", func(t *testing.T) {
		invalid := []map[string]any{
			{"filters": []map[string]any{filter("missing", "eq", "x")}},
			{"filters": []map[string]any{filter("amount", "contains", "1")}},
			{"filters": []map[string]any{filter("stage", "gt", "a")}},
			{"filters": []map[string]any{filter("won", "in", []bool{true})}},
			{"filters": []map[string]any{filter("stage", "in", []string{})}},
			{"filters": []map[string]any{filter("stage", "in", "won")}},
			{"filters": []map[string]any{filter("amount", "in", []string{"x"})}},
			{"filters": []map[string]any{filter("amount", "eq", "x")}},
			{"filters": []map[string]any{filter("close_on", "eq", "yesterday")}},
			{"sort": sortBy("missing", "asc")},
			{"sort": sortBy("name", "sideways")},
			{"limit": 501},
			{"limit": -1},
			{"cursor": "not-a-cursor"},
			{"search": string(make([]byte, 201))},
			{"search": "a b c d e f g h i"},
		}
		for _, body := range invalid {
			response := apiCall(t, handler, cookie, "", "POST", "/api/sections/deals/records/query", body)
			expectStatus(t, response, 400)
			var failure map[string]string
			if err := json.Unmarshal(response.Body.Bytes(), &failure); err != nil || failure["error"] == "" {
				t.Fatalf("expected an error message for %v: %s", body, response.Body.String())
			}
		}
		first := query(map[string]any{"sort": sortBy("amount", "asc"), "limit": 1})
		other := apiCall(t, handler, cookie, "", "POST", "/api/sections/deals/records/query", map[string]any{"sort": sortBy("amount", "desc"), "cursor": *first.NextCursor})
		expectStatus(t, other, 400)
		expectStatus(t, apiCall(t, handler, cookie, "", "POST", "/api/sections/deals/records/query", map[string]any{"unknown": true}), 400)
	})
	t.Run("unknown section is not found", func(t *testing.T) {
		expectStatus(t, apiCall(t, handler, cookie, "", "POST", "/api/sections/nope/records/query", map[string]any{}), 404)
	})
	t.Run("built-in sections use the same endpoint", func(t *testing.T) {
		post("/api/sections/clients/records", map[string]any{"data": map[string]any{"name": "Zed", "status": "Activo", "value": 5.0}}, 200)
		response := post("/api/sections/clients/records/query", map[string]any{"filters": []map[string]any{filter("status", "in", []string{"Activo"})}}, 200)
		var page queryPage
		if err := json.Unmarshal(response.Body.Bytes(), &page); err != nil {
			t.Fatal(err)
		}
		expectNames("clients", page, "Zed")
	})
}
