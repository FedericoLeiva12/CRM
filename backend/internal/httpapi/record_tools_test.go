package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"golang.org/x/crypto/bcrypt"
	"siracrm/internal/auth"
	"siracrm/internal/config"
	"siracrm/internal/database"
	"siracrm/internal/domain"
	"siracrm/internal/store"
)

func TestRecordTools(t *testing.T) {
	ctx := context.Background()
	pool, handler, cookie := startCRM(t, ctx)
	defer pool.Close()
	agentID, token := createAgent(t, handler, cookie)
	grant := func(permissions []domain.Permission) {
		t.Helper()
		expectStatus(t, apiCall(t, handler, cookie, token, "PUT", "/api/agents/"+agentID+"/permissions", map[string]any{"permissions": permissions}), 200)
	}
	grant([]domain.Permission{{SectionID: "prospects", Read: true, Write: true}, {SectionID: "clients", Read: true, Write: true}})
	rpc := func(method string, params any) *httptest.ResponseRecorder {
		t.Helper()
		return apiCall(t, handler, cookie, token, "POST", "/mcp", map[string]any{"jsonrpc": "2.0", "id": 1, "method": method, "params": params})
	}
	expectStatus(t, rpc("initialize", map[string]any{"protocolVersion": "2025-06-18", "capabilities": map[string]any{}, "clientInfo": map[string]string{"name": "test", "version": "1"}}), 200)
	callTool := func(name string, arguments any) map[string]any {
		t.Helper()
		response := rpc("tools/call", map[string]any{"name": name, "arguments": arguments})
		expectStatus(t, response, 200)
		return toolPayload(t, response.Body.Bytes())
	}
	for index := 1; index <= 31; index++ {
		expectStatus(t, apiCall(t, handler, cookie, token, "POST", "/api/sections/prospects/fields", map[string]any{"id": fmt.Sprintf("f%02d", index), "label": fmt.Sprintf("Field %02d", index), "type": "text", "required": false}), 201)
	}
	expectStatus(t, apiCall(t, handler, cookie, token, "POST", "/api/sections/prospects/fields", map[string]any{"id": "next_action_at", "label": "Next action", "type": "date", "required": false}), 201)
	expectStatus(t, apiCall(t, handler, cookie, token, "POST", "/api/sections/clients/fields", map[string]any{"id": "alias", "label": "Alias", "type": "text", "required": false}), 201)

	t.Run("partial update keeps the other fields", func(t *testing.T) {
		data := map[string]any{"name": "Ada Prospect", "email": "ada@example.test", "company": "Northline", "status": "Contactado", "value": 1200.0, "notes": "Original note"}
		for index := 1; index <= 31; index++ {
			data[fmt.Sprintf("f%02d", index)] = fmt.Sprintf("value-%02d", index)
		}
		if len(data) != 37 {
			t.Fatalf("expected 37 fields, got %d", len(data))
		}
		created := callTool("prospects_save", map[string]any{"data": data})
		recordID, _ := created["id"].(string)
		updated := callTool("prospects_update", map[string]any{"id": recordID, "data": map[string]any{"status": "Ganado"}})
		updatedData, _ := updated["data"].(map[string]any)
		if updatedData["status"] != "Ganado" {
			t.Fatalf("status was not updated: %#v", updatedData["status"])
		}
		for key, value := range data {
			if key == "status" {
				continue
			}
			if fmt.Sprint(updatedData[key]) != fmt.Sprint(value) {
				t.Fatalf("field %s changed from %v to %v", key, value, updatedData[key])
			}
		}
		if len(updatedData) != 37 {
			t.Fatalf("updated record has %d fields", len(updatedData))
		}
		cleared := callTool("prospects_update", map[string]any{"id": recordID, "data": map[string]any{"notes": nil}})
		clearedData, _ := cleared["data"].(map[string]any)
		if _, present := clearedData["notes"]; present || clearedData["status"] != "Ganado" || clearedData["f01"] != "value-01" {
			t.Fatalf("null clear changed more than notes: %#v", clearedData["notes"])
		}
		rejected := rpc("tools/call", map[string]any{"name": "prospects_update", "arguments": map[string]any{"id": recordID, "data": map[string]any{"name": nil}}})
		if !strings.Contains(rejected.Body.String(), "Name is required") {
			t.Fatal("required field was cleared", rejected.Body.String())
		}
		var audits int
		if err := pool.QueryRow(ctx, "SELECT count(*) FROM audit WHERE actor=$1 AND action='update' AND section_id='prospects' AND record_id=$2 AND detail='partial'", "agent:"+agentID, recordID).Scan(&audits); err != nil || audits < 1 {
			t.Fatal("partial update was not audited", audits, err)
		}
	})

	t.Run("filters sort and pagination", func(t *testing.T) {
		saved := []string{}
		for _, record := range []map[string]any{
			{"name": "Ada", "status": "Contactado", "next_action_at": "2026-10-04", "value": 10.0},
			{"name": "Bea", "status": "Contactado", "next_action_at": "2026-10-05", "value": 20.0},
			{"name": "Cy", "status": "Contactado", "next_action_at": "2026-10-06", "value": 5.0},
			{"name": "Dee", "status": "Nuevo", "next_action_at": "2026-10-01", "value": 30.0},
			{"name": "Eve", "status": "Contactado", "value": 1.0},
			{"name": "a_b", "status": "Other", "value": 2.0},
			{"name": "axb", "status": "Other", "value": 3.0},
		} {
			created := callTool("prospects_save", map[string]any{"data": record})
			saved = append(saved, created["id"].(string))
		}
		filtered := callTool("prospects_list", map[string]any{"filters": []map[string]any{{"field": "status", "op": "eq", "value": "Contactado"}, {"field": "next_action_at", "op": "lte", "value": "2026-10-05"}}, "sort": map[string]any{"field": "value", "direction": "desc"}})
		names := recordNames(t, filtered)
		if strings.Join(names, ",") != "Bea,Ada" {
			t.Fatalf("filter or sort mismatch: %#v", names)
		}
		if filtered["total"] != float64(2) || filtered["next_cursor"] != nil {
			t.Fatalf("page metadata mismatch: %#v", filtered)
		}
		empty := callTool("prospects_list", map[string]any{"filters": []map[string]any{{"field": "next_action_at", "op": "is_empty"}, {"field": "name", "op": "eq", "value": "Eve"}}})
		if strings.Join(recordNames(t, empty), ",") != "Eve" {
			t.Fatal("is_empty failed", recordNames(t, empty))
		}
		escaped := callTool("prospects_list", map[string]any{"filters": []map[string]any{{"field": "name", "op": "contains", "value": "a_b"}}})
		if strings.Join(recordNames(t, escaped), ",") != "a_b" {
			t.Fatal("contains matched past the escape", recordNames(t, escaped))
		}
		ranged := callTool("prospects_list", map[string]any{"filters": []map[string]any{{"field": "value", "op": "gt", "value": 15.0}, {"field": "status", "op": "eq", "value": "Contactado"}}})
		if strings.Join(recordNames(t, ranged), ",") != "Bea" {
			t.Fatal("range filter failed", recordNames(t, ranged))
		}
		overLimit := rpc("tools/call", map[string]any{"name": "prospects_list", "arguments": map[string]any{"limit": 501}})
		if !strings.Contains(overLimit.Body.String(), "Limit must be between 1 and 500") {
			t.Fatal("limit was not capped", overLimit.Body.String())
		}
		insertPage(t, ctx, pool, 501)
		plain := rpc("tools/call", map[string]any{"name": "clients_list", "arguments": map[string]any{}})
		expectStatus(t, plain, 200)
		if strings.Contains(plain.Body.String(), "next_cursor") || strings.Contains(plain.Body.String(), `"total"`) {
			t.Fatal("list with no arguments changed its response", plain.Body.String())
		}
		plainPayload := toolPayload(t, plain.Body.Bytes())
		if plainPayload["limit"] != float64(500) || len(plainPayload["records"].([]any)) != 500 {
			t.Fatalf("default list was not the latest 500: limit %v count %d", plainPayload["limit"], len(plainPayload["records"].([]any)))
		}
		if recordNames(t, plainPayload)[0] != "Page 0500" {
			t.Fatal("default order changed", recordNames(t, plainPayload)[0])
		}
		newest := callTool("clients_list", map[string]any{"filters": []map[string]any{{"field": "status", "op": "eq", "value": "Paged"}}, "limit": 1})
		if recordNames(t, newest)[0] != "Page 0500" || newest["next_cursor"] == nil {
			t.Fatal("default updated_at page was wrong", recordNames(t, newest), newest["next_cursor"])
		}
		older := callTool("clients_list", map[string]any{"filters": []map[string]any{{"field": "status", "op": "eq", "value": "Paged"}}, "limit": 1, "cursor": newest["next_cursor"]})
		if recordNames(t, older)[0] != "Page 0499" {
			t.Fatal("updated_at cursor skipped or repeated", recordNames(t, older))
		}
		seen := map[string]bool{}
		var cursor any
		for page := 0; page < 3; page++ {
			arguments := map[string]any{"filters": []map[string]any{{"field": "status", "op": "eq", "value": "Paged"}}, "sort": map[string]any{"field": "name", "direction": "asc"}, "limit": 500}
			if cursor != nil {
				arguments["cursor"] = cursor
			}
			payload := callTool("clients_list", arguments)
			if payload["total"] != float64(501) {
				t.Fatalf("total = %v", payload["total"])
			}
			for _, name := range recordNames(t, payload) {
				if seen[name] {
					t.Fatalf("duplicate page row %s on page %d", name, page)
				}
				seen[name] = true
			}
			cursor = payload["next_cursor"]
			if page == 0 && (cursor == nil || len(recordNames(t, payload)) != 500) {
				t.Fatal("first page did not expose the next cursor")
			}
			if page == 1 && (cursor != nil || len(recordNames(t, payload)) != 1) {
				t.Fatalf("second page = %#v cursor %#v", recordNames(t, payload), cursor)
			}
			if cursor == nil {
				break
			}
		}
		if len(seen) != 501 {
			t.Fatalf("paged %d records", len(seen))
		}
		_ = saved
	})

	t.Run("timeline survives edits", func(t *testing.T) {
		created := callTool("prospects_save", map[string]any{"data": map[string]any{"name": "Timeline", "status": "Nuevo"}})
		recordID := created["id"].(string)
		for _, entry := range []map[string]any{
			{"id": recordID, "type": "llamada", "date": "2026-01-02", "summary": "First call", "channel": "phone"},
			{"id": recordID, "type": "email_enviado", "date": "2026-03-04T15:04:05Z", "summary": "Sent the note", "ref": "thread-9"},
			{"id": recordID, "type": "comment", "date": "2026-02-02", "summary": "Left a comment"},
		} {
			callTool("prospects_log_activity", entry)
		}
		before := callTool("prospects_activities", map[string]any{"id": recordID})
		summaries := activitySummaries(t, before)
		if strings.Join(summaries, "|") != "Sent the note|Left a comment|First call" {
			t.Fatalf("activity order: %#v", summaries)
		}
		callTool("prospects_update", map[string]any{"id": recordID, "data": map[string]any{"notes": "Edited after the timeline"}})
		after := callTool("prospects_activities", map[string]any{"id": recordID})
		if strings.Join(activitySummaries(t, after), "|") != strings.Join(summaries, "|") {
			t.Fatal("activities changed after the edit", activitySummaries(t, after))
		}
		callTool("prospects_update", map[string]any{"id": recordID, "data": map[string]any{"status": "Ganado"}})
		withStatus := activitySummaries(t, callTool("prospects_activities", map[string]any{"id": recordID}))
		if len(withStatus) == 0 || !strings.Contains(withStatus[0], "Status changed from Nuevo to Ganado") {
			t.Fatal("status change was not logged", withStatus)
		}
		var audits int
		if err := pool.QueryRow(ctx, "SELECT count(*) FROM audit WHERE actor=$1 AND action='log_activity' AND record_id=$2", "agent:"+agentID, recordID).Scan(&audits); err != nil || audits != 3 {
			t.Fatal("activity audit missing", audits, err)
		}
	})

	t.Run("generic conversion links both ways", func(t *testing.T) {
		created := callTool("prospects_save", map[string]any{"data": map[string]any{"name": "Converted", "company": "Northline", "email": "converted@example.test", "value": 1800.0, "notes": "Bring this over", "status": "Contactado", "f01": "custom"}})
		recordID := created["id"].(string)
		converted := callTool("prospects_convert", map[string]any{"id": recordID, "target": "clients", "status": "Ganado"})
		source := converted["source"].(map[string]any)
		target := converted["target"].(map[string]any)
		sourceData := source["data"].(map[string]any)
		targetData := target["data"].(map[string]any)
		if sourceData["status"] != "Ganado" || sourceData["f01"] != "custom" {
			t.Fatalf("source was not marked Ganado without losing its fields: %#v", sourceData)
		}
		for _, key := range []string{"name", "company", "email", "notes"} {
			if targetData[key] != sourceData[key] {
				t.Fatalf("copied %s = %#v", key, targetData[key])
			}
		}
		if fmt.Sprint(targetData["value"]) != "1800" || targetData["f01"] != nil {
			t.Fatalf("target copy mismatch: %#v", targetData)
		}
		clientID := target["id"].(string)
		again := rpc("tools/call", map[string]any{"name": "prospects_convert", "arguments": map[string]any{"id": recordID, "target": "clients"}})
		if !strings.Contains(again.Body.String(), "already linked") {
			t.Fatal("second conversion succeeded", again.Body.String())
		}
		prospect := callTool("prospects_get", map[string]any{"id": recordID})
		client := callTool("clients_get", map[string]any{"id": clientID})
		if linkedID(t, prospect, "clients", "outgoing") != clientID || linkedID(t, client, "prospects", "incoming") != recordID {
			t.Fatalf("links were not stored both ways: %#v %#v", prospect["links"], client["links"])
		}
		// The spec's client_id and prospect_id are these two link ends.
		if linkedID(t, prospect, "clients", "outgoing") == "" || linkedID(t, client, "prospects", "incoming") == "" {
			t.Fatal("derived ids were empty")
		}
		expectStatus(t, apiCall(t, handler, cookie, token, "POST", "/api/sections", map[string]string{"id": "partners", "name": "Partners"}), 201)
		expectStatus(t, apiCall(t, handler, cookie, token, "POST", "/api/sections/partners/fields", map[string]any{"id": "alias", "label": "Alias", "type": "text", "required": false}), 201)
		grant([]domain.Permission{{SectionID: "partners", Read: true, Write: true}})
		mapped := callTool("prospects_convert", map[string]any{"id": recordID, "target": "partners", "mapping": map[string]string{"f01": "alias"}, "overrides": map[string]any{"name": "Partner record"}})
		partner := mapped["target"].(map[string]any)["data"].(map[string]any)
		if partner["name"] != "Partner record" || partner["alias"] != "custom" {
			t.Fatalf("mapping or overrides were not applied: %#v", partner)
		}
		secondPartner := rpc("tools/call", map[string]any{"name": "prospects_convert", "arguments": map[string]any{"id": recordID, "target": "partners"}})
		if !strings.Contains(secondPartner.Body.String(), "already linked") {
			t.Fatal("repeat conversion into another section succeeded", secondPartner.Body.String())
		}
		var audits int
		if err := pool.QueryRow(ctx, "SELECT count(*) FROM audit WHERE actor=$1 AND action='convert' AND record_id=$2", "agent:"+agentID, recordID).Scan(&audits); err != nil || audits != 2 {
			t.Fatal("conversion audit missing", audits, err)
		}
		detail := apiCall(t, handler, cookie, token, "GET", "/api/sections/prospects/records/"+recordID, nil)
		expectStatus(t, detail, 200)
		if !strings.Contains(detail.Body.String(), clientID) || !strings.Contains(detail.Body.String(), "Status changed from Contactado to Ganado") {
			t.Fatal("record view payload is missing links or history", detail.Body.String())
		}
	})

	t.Run("existing grants cover the new tools", func(t *testing.T) {
		readerID, readerToken := createAgent(t, handler, cookie)
		expectStatus(t, apiCall(t, handler, cookie, readerToken, "PUT", "/api/agents/"+readerID+"/permissions", map[string]any{"permissions": []domain.Permission{{SectionID: "prospects", Read: true}}}), 200)
		listed := apiCall(t, handler, cookie, readerToken, "POST", "/mcp", map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/list", "params": map[string]any{}})
		body := listed.Body.String()
		for _, name := range []string{"prospects_get", "prospects_list", "prospects_activities"} {
			if !strings.Contains(body, name) {
				t.Fatalf("read grant did not expose %s", name)
			}
		}
		for _, name := range []string{"prospects_update", "prospects_log_activity", "prospects_convert", "prospects_save"} {
			if strings.Contains(body, name) {
				t.Fatalf("read grant exposed %s", name)
			}
		}
		readerCall := func(name string, arguments any) map[string]any {
			t.Helper()
			response := apiCall(t, handler, cookie, readerToken, "POST", "/mcp", map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": map[string]any{"name": name, "arguments": arguments}})
			expectStatus(t, response, 200)
			return toolPayload(t, response.Body.Bytes())
		}
		hidden := readerCall("prospects_list", map[string]any{"filters": []map[string]any{{"field": "name", "op": "eq", "value": "Converted"}}})
		got := readerCall("prospects_get", map[string]any{"id": recordID(t, hidden)})
		for _, item := range got["links"].([]any) {
			link := item.(map[string]any)
			if link["section_id"] == "clients" && link["name"] != nil && link["name"] != "" {
				t.Fatal("linked client name was visible without client read access", link)
			}
		}
		writerID, writerToken := createAgent(t, handler, cookie)
		expectStatus(t, apiCall(t, handler, cookie, writerToken, "PUT", "/api/agents/"+writerID+"/permissions", map[string]any{"permissions": []domain.Permission{{SectionID: "prospects", Write: true}}}), 200)
		listed = apiCall(t, handler, cookie, writerToken, "POST", "/mcp", map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/list", "params": map[string]any{}})
		body = listed.Body.String()
		for _, name := range []string{"prospects_update", "prospects_log_activity", "prospects_save", "prospects_delete"} {
			if !strings.Contains(body, name) {
				t.Fatalf("write grant did not expose %s", name)
			}
		}
		for _, name := range []string{"prospects_get", "prospects_list", "prospects_activities", "prospects_convert"} {
			if strings.Contains(body, name) {
				t.Fatalf("write grant exposed %s", name)
			}
		}
		limitedID, limitedToken := createAgent(t, handler, cookie)
		expectStatus(t, apiCall(t, handler, cookie, limitedToken, "PUT", "/api/agents/"+limitedID+"/permissions", map[string]any{"permissions": []domain.Permission{{SectionID: "prospects", Read: true, Write: true}, {SectionID: "clients", Read: true}}}), 200)
		denied := apiCall(t, handler, cookie, limitedToken, "POST", "/mcp", map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": map[string]any{"name": "prospects_convert", "arguments": map[string]any{"id": "missing", "target": "clients"}}})
		if !strings.Contains(denied.Body.String(), "permission denied") {
			t.Fatal("conversion did not require write on the target", denied.Body.String())
		}
	})

	t.Run("fresh list and call share one connection", func(t *testing.T) {
		connectedID, connectedToken := createAgent(t, handler, cookie)
		server := httptest.NewServer(handler)
		defer server.Close()
		var dials atomic.Int32
		dialer := &net.Dialer{}
		transport := &http.Transport{DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			dials.Add(1)
			return dialer.DialContext(ctx, network, address)
		}, MaxConnsPerHost: 1}
		mcpHTTP := &http.Client{Transport: bearerTransport{token: connectedToken, base: transport}}
		client := mcp.NewClient(&mcp.Implementation{Name: "connected-test", Version: "1"}, nil)
		session, err := client.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: server.URL + "/mcp", HTTPClient: mcpHTTP, DisableStandaloneSSE: true}, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer session.Close()
		before, err := session.ListTools(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		if toolNamed(before, "prospects_get") {
			t.Fatal("tool was visible before the grant")
		}
		expectStatus(t, apiCall(t, handler, cookie, connectedToken, "PUT", "/api/agents/"+connectedID+"/permissions", map[string]any{"permissions": []domain.Permission{{SectionID: "prospects", Read: true}}}), 200)
		created := callTool("prospects_save", map[string]any{"data": map[string]any{"name": "Visible after connect"}})
		after, err := session.ListTools(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		if !toolNamed(after, "prospects_get") {
			t.Fatal("fresh tools/list did not include prospects_get")
		}
		result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "prospects_get", Arguments: map[string]any{"id": created["id"]}})
		if err != nil {
			t.Fatal(err)
		}
		if result.IsError {
			t.Fatalf("tools/call after tools/list failed: %#v", result.Content)
		}
		encoded, _ := json.Marshal(result.StructuredContent)
		if !strings.Contains(string(encoded), "Visible after connect") {
			t.Fatalf("call result: %s", encoded)
		}
		if dials.Load() != 1 {
			t.Fatalf("list and call used %d connections", dials.Load())
		}
	})

}

func startCRM(t *testing.T, ctx context.Context) (*pgxpool.Pool, http.Handler, string) {
	t.Helper()
	url := databaseURL(t)
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, "DROP SCHEMA public CASCADE; CREATE SCHEMA public"); err != nil {
		t.Fatal(err)
	}
	if err = database.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	passwordHash, _ := bcrypt.GenerateFromPassword([]byte("a-long-test-password"), bcrypt.MinCost)
	if _, err = pool.Exec(ctx, "INSERT INTO users(id,email,password_hash,role) VALUES('admin','admin@example.test',$1,'admin')", string(passwordHash)); err != nil {
		t.Fatal(err)
	}
	settings := config.Config{AppOrigin: "https://crm.example.test", SecureCookies: true}
	repository := store.New(pool)
	handler := New(repository, auth.New(repository), settings).Handler()
	response := apiCall(t, handler, "", "", "POST", "/api/login", map[string]string{"email": "admin@example.test", "password": "a-long-test-password"})
	expectStatus(t, response, 200)
	sessionCookie := response.Result().Cookies()[0]
	return pool, handler, sessionCookie.Name + "=" + sessionCookie.Value
}

func databaseURL(t *testing.T) string {
	t.Helper()
	value := os.Getenv("TEST_DATABASE_URL")
	if value == "" {
		t.Skip("Set TEST_DATABASE_URL to an isolated PostgreSQL database")
	}
	return value
}

func createAgent(t *testing.T, handler http.Handler, cookie string) (string, string) {
	t.Helper()
	response := apiCall(t, handler, cookie, "", "POST", "/api/agents", map[string]string{"name": "Record agent"})
	expectStatus(t, response, 201)
	var agent map[string]string
	if err := json.Unmarshal(response.Body.Bytes(), &agent); err != nil {
		t.Fatal(err)
	}
	return agent["id"], agent["token"]
}

func apiCall(t *testing.T, handler http.Handler, cookie, token, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var reader io.Reader
	if body != nil {
		raw, _ := json.Marshal(body)
		reader = bytes.NewReader(raw)
	}
	request := httptest.NewRequest(method, path, reader)
	request.Header.Set("Origin", "https://crm.example.test")
	request.Header.Set("Content-Type", "application/json")
	if cookie != "" {
		request.Header.Set("Cookie", cookie)
	}
	if path == "/mcp" {
		request.Header.Set("Authorization", "Bearer "+token)
		request.Header.Set("Accept", "application/json, text/event-stream")
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func expectStatus(t *testing.T, response *httptest.ResponseRecorder, status int) {
	t.Helper()
	if response.Code != status {
		t.Fatalf("expected %d, got %d: %s", status, response.Code, response.Body.String())
	}
}

func toolPayload(t *testing.T, raw []byte) map[string]any {
	t.Helper()
	var payload struct {
		Result struct {
			IsError           bool             `json:"isError"`
			StructuredContent map[string]any   `json:"structuredContent"`
			Content           []map[string]any `json:"content"`
		} `json:"result"`
		Error any `json:"error"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatal(err, string(raw))
	}
	if payload.Error != nil || payload.Result.IsError {
		t.Fatalf("tool failed: %s", raw)
	}
	if payload.Result.StructuredContent != nil {
		return payload.Result.StructuredContent
	}
	if len(payload.Result.Content) > 0 {
		text, _ := payload.Result.Content[0]["text"].(string)
		var data map[string]any
		if err := json.Unmarshal([]byte(text), &data); err != nil {
			t.Fatal(err, text)
		}
		return data
	}
	t.Fatalf("empty tool result: %s", raw)
	return nil
}

func recordNames(t *testing.T, payload map[string]any) []string {
	t.Helper()
	records, _ := payload["records"].([]any)
	names := []string{}
	for _, item := range records {
		record, _ := item.(map[string]any)
		data, _ := record["data"].(map[string]any)
		names = append(names, fmt.Sprint(data["name"]))
	}
	return names
}

func activitySummaries(t *testing.T, payload map[string]any) []string {
	t.Helper()
	entries, _ := payload["activities"].([]any)
	summaries := []string{}
	for _, item := range entries {
		activity, _ := item.(map[string]any)
		summaries = append(summaries, fmt.Sprint(activity["summary"]))
	}
	return summaries
}

func linkedID(t *testing.T, payload map[string]any, sectionID, direction string) string {
	t.Helper()
	links, _ := payload["links"].([]any)
	for _, item := range links {
		link, _ := item.(map[string]any)
		if link["section_id"] == sectionID && link["direction"] == direction {
			return fmt.Sprint(link["record_id"])
		}
	}
	return ""
}

func insertPage(t *testing.T, ctx context.Context, pool *pgxpool.Pool, count int) {
	t.Helper()
	transaction, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = transaction.Rollback(ctx) }()
	for index := 0; index < count; index++ {
		identifier := fmt.Sprintf("page-%04d", index)
		name := fmt.Sprintf("Page %04d", index)
		if _, err = transaction.Exec(ctx, "INSERT INTO records(id,section_id,data,updated_at) VALUES($1,'clients',$2::jsonb, now() - make_interval(secs => $3))", identifier, fmt.Sprintf(`{"name":%q,"status":"Paged"}`, name), count-index); err != nil {
			t.Fatal(err)
		}
		if _, err = transaction.Exec(ctx, "INSERT INTO record_values(record_id,section_id,field_id,text_value) VALUES($1,'clients','name',$2),($1,'clients','status','Paged')", identifier, name); err != nil {
			t.Fatal(err)
		}
	}
	if err = transaction.Commit(ctx); err != nil {
		t.Fatal(err)
	}
}

type bearerTransport struct {
	token string
	base  http.RoundTripper
}

func (transport bearerTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	clone := request.Clone(request.Context())
	clone.Header.Set("Authorization", "Bearer "+transport.token)
	return transport.base.RoundTrip(clone)
}

func recordID(t *testing.T, payload map[string]any) string {
	t.Helper()
	records, _ := payload["records"].([]any)
	if len(records) != 1 {
		t.Fatalf("expected one record, got %#v", recordNames(t, payload))
	}
	record := records[0].(map[string]any)
	return fmt.Sprint(record["id"])
}

func toolNamed(result *mcp.ListToolsResult, name string) bool {
	if result == nil {
		return false
	}
	for _, tool := range result.Tools {
		if tool.Name == name {
			return true
		}
	}
	return false
}
