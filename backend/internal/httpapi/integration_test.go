package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
	"siracrm/internal/auth"
	"siracrm/internal/config"
	"siracrm/internal/database"
	"siracrm/internal/domain"
	"siracrm/internal/store"
)

func TestIntegration(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("Set TEST_DATABASE_URL to an isolated PostgreSQL database")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if _, err = pool.Exec(ctx, "DROP SCHEMA public CASCADE; CREATE SCHEMA public"); err != nil {
		t.Fatal(err)
	}
	if err = database.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	if err = database.Migrate(ctx, pool); err != nil {
		t.Fatal("Migration replay failed", err)
	}
	var migrationCount int
	if err = pool.QueryRow(ctx, "SELECT count(*) FROM schema_migrations").Scan(&migrationCount); err != nil || migrationCount != 1 {
		t.Fatal("Migration was not tracked exactly once")
	}
	passwordHash, _ := bcrypt.GenerateFromPassword([]byte("a-long-test-password"), bcrypt.MinCost)
	pool.Exec(ctx, "INSERT INTO users(id,email,password_hash) VALUES('admin','admin@example.test',$1)", string(passwordHash))
	settings := config.Config{AppOrigin: "https://crm.example.test", SecureCookies: true}
	repository := store.New(pool)
	app := New(repository, auth.New(repository), settings)
	handler := app.Handler()
	var cookie, token string
	call := func(method, path string, body any, auth bool) *httptest.ResponseRecorder {
		t.Helper()
		raw, _ := json.Marshal(body)
		request := httptest.NewRequest(method, path, bytes.NewReader(raw))
		request.Header.Set("Origin", settings.AppOrigin)
		if auth && cookie != "" {
			request.Header.Set("Cookie", cookie)
		}
		if path == "/mcp" {
			request.Header.Set("Authorization", "Bearer "+token)
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("Accept", "application/json, text/event-stream")
		}
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		return response
	}
	expect := func(response *httptest.ResponseRecorder, status int) {
		t.Helper()
		if response.Code != status {
			t.Fatalf("Expected %d, got %d: %s", status, response.Code, response.Body.String())
		}
	}
	// Browser authentication rejects anonymous requests and cross-origin mutations.
	expect(call("GET", "/api/sections", nil, false), 401)
	bad := httptest.NewRequest("POST", "/api/login", strings.NewReader(`{"email":"admin@example.test","password":"a-long-test-password"}`))
	bad.Header.Set("Origin", "https://attacker.test")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, bad)
	expect(response, 403)
	expect(call("POST", "/api/login", map[string]string{"email": "admin@example.test", "password": "wrong"}, false), 401)
	response = call("POST", "/api/login", map[string]string{"email": "admin@example.test", "password": "a-long-test-password"}, false)
	expect(response, 200)
	sessionCookie := response.Result().Cookies()[0]
	if !sessionCookie.Secure || !sessionCookie.HttpOnly || sessionCookie.SameSite != http.SameSiteStrictMode {
		t.Fatal("Insecure cookie")
	}
	cookie = sessionCookie.Name + "=" + sessionCookie.Value
	expect(call("GET", "/api/sections", nil, true), 200)
	// The registry validates all record writes, including custom fields.
	expect(call("POST", "/api/sections/clients/records", map[string]any{"data": map[string]any{"name": "Mara Santos", "email": "invalid"}}, true), 400)
	response = call("POST", "/api/sections/clients/records", map[string]any{"data": map[string]any{"name": "Mara Santos", "email": "mara@example.test"}}, true)
	expect(response, 200)
	var record map[string]string
	json.Unmarshal(response.Body.Bytes(), &record)
	expect(call("POST", "/api/sections/clients/fields", map[string]any{"id": "industry", "label": "Industry", "type": "text", "required": false}, true), 201)
	expect(call("POST", "/api/sections/clients/fields", map[string]any{"id": "required", "label": "Required", "type": "text", "required": true}, true), 400)
	expect(call("PUT", "/api/sections/clients/records/"+record["id"], map[string]any{"data": map[string]any{"name": "Mara Santos", "industry": "Architecture"}}, true), 200)
	// Agent discovery and invocation enforce independent grants.
	response = call("POST", "/api/agents", map[string]string{"name": "Test agent"}, true)
	expect(response, 201)
	var agent map[string]string
	json.Unmarshal(response.Body.Bytes(), &agent)
	token = agent["token"]
	rpc := func(method string, params any) *httptest.ResponseRecorder {
		return call("POST", "/mcp", map[string]any{"jsonrpc": "2.0", "id": 1, "method": method, "params": params}, false)
	}
	response = rpc("initialize", map[string]any{"protocolVersion": "2025-06-18", "capabilities": map[string]any{}, "clientInfo": map[string]string{"name": "test", "version": "1"}})
	expect(response, 200)
	response = rpc("tools/list", map[string]any{})
	expect(response, 200)
	if strings.Contains(response.Body.String(), "clients_list") {
		t.Fatal("Default deny failed")
	}
	expect(call("PUT", "/api/agents/"+agent["id"]+"/permissions", map[string]any{"permissions": []domain.Permission{{SectionID: "clients", Read: true, Write: false}}}, true), 200)
	response = rpc("tools/list", map[string]any{})
	expect(response, 200)
	if !strings.Contains(response.Body.String(), "clients_list") || strings.Contains(response.Body.String(), "clients_save") {
		t.Fatal("Read permission isolation failed", response.Body.String())
	}
	response = rpc("tools/call", map[string]any{"name": "clients_list", "arguments": map[string]any{}})
	expect(response, 200)
	if !strings.Contains(response.Body.String(), "Mara Santos") {
		t.Fatal("MCP read failed", response.Body.String())
	}
	response = rpc("tools/call", map[string]any{"name": "clients_save", "arguments": map[string]any{"data": map[string]any{"name": "Unauthorized"}}})
	expect(response, 200)
	if !strings.Contains(response.Body.String(), "error") {
		t.Fatal("MCP write was allowed")
	}
	// A future section appears automatically with access denied by default.
	expect(call("POST", "/api/sections", map[string]string{"id": "employees", "name": "Employees"}, true), 201)
	response = call("GET", "/api/agents", nil, true)
	expect(response, 200)
	if !strings.Contains(response.Body.String(), `"section_id":"employees","read":false,"write":false`) {
		t.Fatal("Automatic section permissions missing", response.Body.String())
	}
	expect(call("PUT", "/api/agents/"+agent["id"]+"/permissions", map[string]any{"permissions": []domain.Permission{{SectionID: "employees", Write: true}}}, true), 200)
	response = rpc("tools/call", map[string]any{"name": "employees_save", "arguments": map[string]any{"data": map[string]any{"name": "Tess Ward"}}})
	expect(response, 200)
	if strings.Contains(response.Body.String(), `"isError":true`) || strings.Contains(response.Body.String(), `"error":`) {
		t.Fatal("MCP write failed", response.Body.String())
	}
	response = rpc("tools/list", map[string]any{})
	expect(response, 200)
	if !strings.Contains(response.Body.String(), "employees_save") || strings.Contains(response.Body.String(), "employees_list") {
		t.Fatal("Write-only permission failed", response.Body.String())
	}
	expect(call("PUT", "/api/agents/"+agent["id"]+"/permissions", map[string]any{"permissions": []domain.Permission{{SectionID: "clients"}}}, true), 200)
	response = rpc("tools/list", map[string]any{})
	expect(response, 200)
	if strings.Contains(response.Body.String(), "clients_list") {
		t.Fatal("Live permission revocation failed")
	}
	expect(call("DELETE", "/api/agents/"+agent["id"], nil, true), 200)
	expect(rpc("tools/list", map[string]any{}), 401)
	expect(call("DELETE", "/api/sections/clients/records/"+record["id"], nil, true), 200)
	var count int
	pool.QueryRow(ctx, "SELECT count(*) FROM audit").Scan(&count)
	if count < 4 {
		t.Fatal("Mutation auditing missing")
	}
	// Password replacement revokes prior sessions before the new password can sign in.
	expect(call("POST", "/api/password", map[string]string{"current": "wrong", "password": "new-long-test-password"}, true), 400)
	expect(call("POST", "/api/password", map[string]string{"current": "a-long-test-password", "password": "short"}, true), 400)
	expect(call("POST", "/api/password", map[string]string{"current": "a-long-test-password", "password": "new-long-test-password"}, true), 200)
	expect(call("GET", "/api/sections", nil, true), 401)
	response = call("POST", "/api/login", map[string]string{"email": "admin@example.test", "password": "new-long-test-password"}, false)
	expect(response, 200)
	sessionCookie = response.Result().Cookies()[0]
	cookie = sessionCookie.Name + "=" + sessionCookie.Value
	expect(call("POST", "/api/logout", nil, true), 200)
	expect(call("GET", "/api/sections", nil, true), 401)
}
