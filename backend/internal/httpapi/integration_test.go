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
	"siracrm/internal/auth/token"
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
	if err = pool.QueryRow(ctx, "SELECT count(*) FROM schema_migrations").Scan(&migrationCount); err != nil || migrationCount != 4 {
		t.Fatal("Migration was not tracked exactly once", migrationCount, err)
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
	if strings.Contains(response.Body.String(), "clients_list") || strings.Contains(response.Body.String(), "sections_create") || strings.Contains(response.Body.String(), "fields_add") {
		t.Fatal("Default deny failed", response.Body.String())
	}
	response = rpc("tools/call", map[string]any{"name": "sections_create", "arguments": map[string]any{"id": "vendors", "name": "Vendors"}})
	expect(response, 200)
	if !strings.Contains(response.Body.String(), "error") {
		t.Fatal("Schema management was allowed by default", response.Body.String())
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
	// Schema management is a separate grant. It uses the same validation as the admin UI.
	expect(call("PUT", "/api/agents/"+agent["id"]+"/permissions", map[string]any{"manage_schema": true}, true), 200)
	response = rpc("tools/list", map[string]any{})
	expect(response, 200)
	if !strings.Contains(response.Body.String(), "sections_create") || !strings.Contains(response.Body.String(), "fields_add") {
		t.Fatal("Schema tools missing after grant", response.Body.String())
	}
	uiInvalid := call("POST", "/api/sections", map[string]string{"id": "Bad", "name": "Vendors"}, true)
	mcpInvalid := rpc("tools/call", map[string]any{"name": "sections_create", "arguments": map[string]any{"id": "Bad", "name": "Vendors"}})
	expect(uiInvalid, 400)
	expect(mcpInvalid, 200)
	if !strings.Contains(uiInvalid.Body.String(), "Use a valid identifier and section name") || !strings.Contains(mcpInvalid.Body.String(), "Use a valid identifier and section name") {
		t.Fatal("Section validation diverged", uiInvalid.Body.String(), mcpInvalid.Body.String())
	}
	response = rpc("tools/call", map[string]any{"name": "sections_create", "arguments": map[string]any{"id": "vendors", "name": "Vendors"}})
	expect(response, 200)
	if strings.Contains(response.Body.String(), `"isError":true`) || strings.Contains(response.Body.String(), `"error"`) {
		t.Fatal("Schema create failed", response.Body.String())
	}
	response = call("GET", "/api/agents", nil, true)
	expect(response, 200)
	if !strings.Contains(response.Body.String(), `"manage_schema":true`) || !strings.Contains(response.Body.String(), `"section_id":"vendors","read":false,"write":false`) {
		t.Fatal("Created section was not denied", response.Body.String())
	}
	response = rpc("tools/list", map[string]any{})
	expect(response, 200)
	if strings.Contains(response.Body.String(), "vendors_list") || strings.Contains(response.Body.String(), "vendors_save") {
		t.Fatal("Creating a section granted record tools", response.Body.String())
	}
	const requiredFieldMessage = "New fields must be optional while records exist"
	uiRequired := call("POST", "/api/sections/clients/fields", map[string]any{"id": "priority", "label": "Priority", "type": "text", "required": true}, true)
	mcpRequired := rpc("tools/call", map[string]any{"name": "fields_add", "arguments": map[string]any{"section": "clients", "id": "priority", "label": "Priority", "type": "text", "required": true}})
	expect(uiRequired, 400)
	expect(mcpRequired, 200)
	if !strings.Contains(uiRequired.Body.String(), requiredFieldMessage) || !strings.Contains(mcpRequired.Body.String(), requiredFieldMessage) {
		t.Fatal("Required-field validation diverged", uiRequired.Body.String(), mcpRequired.Body.String())
	}
	uiType := call("POST", "/api/sections/vendors/fields", map[string]any{"id": "kind", "label": "Kind", "type": "file", "required": false}, true)
	mcpType := rpc("tools/call", map[string]any{"name": "fields_add", "arguments": map[string]any{"section": "vendors", "id": "kind", "label": "Kind", "type": "file", "required": false}})
	expect(uiType, 400)
	expect(mcpType, 200)
	if !strings.Contains(uiType.Body.String(), "Unsupported field type") || !strings.Contains(mcpType.Body.String(), "Unsupported field type") {
		t.Fatal("Field type validation diverged", uiType.Body.String(), mcpType.Body.String())
	}
	response = rpc("tools/call", map[string]any{"name": "fields_add", "arguments": map[string]any{"section": "vendors", "id": "region", "label": "Region", "type": "text", "required": true}})
	expect(response, 200)
	if strings.Contains(response.Body.String(), `"isError":true`) {
		t.Fatal("Optional-section required field was rejected", response.Body.String())
	}
	expect(call("PUT", "/api/agents/"+agent["id"]+"/permissions", map[string]any{"manage_schema": false, "permissions": []domain.Permission{{SectionID: "vendors", Read: true}}}, true), 200)
	response = rpc("tools/list", map[string]any{})
	expect(response, 200)
	if strings.Contains(response.Body.String(), "sections_create") || strings.Contains(response.Body.String(), "fields_add") || !strings.Contains(response.Body.String(), "vendors_list") {
		t.Fatal("Schema revocation or tool refresh failed", response.Body.String())
	}
	response = rpc("tools/call", map[string]any{"name": "fields_add", "arguments": map[string]any{"section": "vendors", "id": "tier", "label": "Tier", "type": "text", "required": false}})
	expect(response, 200)
	if !strings.Contains(response.Body.String(), "error") {
		t.Fatal("Revoked schema management still worked", response.Body.String())
	}
	var schemaAudits int
	if err = pool.QueryRow(ctx, "SELECT count(*) FROM audit WHERE actor=$1 AND ((action='create_section' AND section_id='vendors') OR (action='add_field' AND section_id='vendors' AND record_id='region'))", "agent:"+agent["id"]).Scan(&schemaAudits); err != nil || schemaAudits != 2 {
		t.Fatal("Schema audit missing", schemaAudits, err)
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

func TestTeamInvitations(t *testing.T) {
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
	passwordHash, _ := bcrypt.GenerateFromPassword([]byte("a-long-test-password"), bcrypt.MinCost)
	if _, err = pool.Exec(ctx, "INSERT INTO users(id,email,password_hash) VALUES('admin','admin@example.test',$1)", string(passwordHash)); err != nil {
		t.Fatal(err)
	}
	var role string
	if err = pool.QueryRow(ctx, "SELECT role FROM users WHERE id='admin'").Scan(&role); err != nil || role != domain.RoleAdmin {
		t.Fatal("Existing administrator was not kept as admin", role, err)
	}
	settings := config.Config{AppOrigin: "https://crm.example.test", SecureCookies: true}
	repository := store.New(pool)
	handler := New(repository, auth.New(repository), settings).Handler()
	adminCookie := ""
	callAs := func(method, path, cookie string, body any) *httptest.ResponseRecorder {
		t.Helper()
		raw, _ := json.Marshal(body)
		request := httptest.NewRequest(method, path, bytes.NewReader(raw))
		request.Header.Set("Origin", settings.AppOrigin)
		request.Header.Set("Content-Type", "application/json")
		if cookie != "" {
			request.Header.Set("Cookie", cookie)
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
	mustContain := func(response *httptest.ResponseRecorder, text string) {
		t.Helper()
		if !strings.Contains(response.Body.String(), text) {
			t.Fatalf("Expected %q in %s", text, response.Body.String())
		}
	}
	login := func(email, password string) string {
		t.Helper()
		response := callAs("POST", "/api/login", "", map[string]string{"email": email, "password": password})
		expect(response, 200)
		return response.Result().Cookies()[0].Name + "=" + response.Result().Cookies()[0].Value
	}
	adminCookie = login("admin@example.test", "a-long-test-password")
	expect(callAs("POST", "/api/invites", "", map[string]string{"email": "person@example.test", "role": "member"}), 401)
	response := callAs("POST", "/api/invites", adminCookie, map[string]string{"email": "person@example.test", "role": "member"})
	expect(response, 201)
	var created map[string]string
	json.Unmarshal(response.Body.Bytes(), &created)
	inviteToken := created["token"]
	if inviteToken == "" || created["link"] != settings.AppOrigin+"/invite/"+inviteToken {
		t.Fatal("Invite link was not returned once", response.Body.String())
	}
	var storedHash string
	if err = pool.QueryRow(ctx, "SELECT token_hash FROM invites WHERE email='person@example.test'").Scan(&storedHash); err != nil || storedHash == inviteToken || storedHash != token.Hash(inviteToken) {
		t.Fatal("Invite token was not stored as a hash", storedHash, err)
	}
	listed := callAs("GET", "/api/invites", adminCookie, nil)
	expect(listed, 200)
	if strings.Contains(listed.Body.String(), inviteToken) {
		t.Fatal("Pending invite list exposed the token")
	}
	duplicate := callAs("POST", "/api/invites", adminCookie, map[string]string{"email": "person@example.test", "role": "admin"})
	expect(duplicate, 400)
	mustContain(duplicate, "An invitation for this email is already pending")
	preview := callAs("GET", "/api/invite/"+inviteToken, "", nil)
	expect(preview, 200)
	mustContain(preview, "person@example.test")
	accepted := callAs("POST", "/api/invite/"+inviteToken, "", map[string]string{"name": "Nia Cole", "password": "short"})
	expect(accepted, 400)
	mustContain(accepted, "Password must be 14–72 bytes")
	accepted = callAs("POST", "/api/invite/"+inviteToken, "", map[string]string{"name": "Nia Cole", "password": "member-long-password"})
	expect(accepted, 201)
	memberCookie := accepted.Result().Cookies()[0].Name + "=" + accepted.Result().Cookies()[0].Value
	if !accepted.Result().Cookies()[0].HttpOnly || accepted.Result().Cookies()[0].SameSite != http.SameSiteStrictMode {
		t.Fatal("Invite acceptance did not sign the person in")
	}
	expect(callAs("GET", "/api/sections", memberCookie, nil), 200)
	memberProfile := callAs("GET", "/api/me", memberCookie, nil)
	expect(memberProfile, 200)
	mustContain(memberProfile, `"role":"member"`)
	again := callAs("POST", "/api/invite/"+inviteToken, "", map[string]string{"name": "Nia Cole", "password": "member-long-password"})
	expect(again, 404)
	mustContain(again, "This invitation is no longer valid")
	response = callAs("POST", "/api/invites", adminCookie, map[string]string{"email": "person@example.test", "role": "member"})
	expect(response, 400)
	mustContain(response, "An account with this email already exists")
	response = callAs("POST", "/api/invites", adminCookie, map[string]string{"email": "later@example.test", "role": "member"})
	expect(response, 201)
	json.Unmarshal(response.Body.Bytes(), &created)
	revokedToken := created["token"]
	expect(callAs("DELETE", "/api/invites/"+created["id"], adminCookie, nil), 200)
	expect(callAs("POST", "/api/invite/"+revokedToken, "", map[string]string{"name": "Later Person", "password": "member-long-password"}), 404)
	response = callAs("POST", "/api/invites", adminCookie, map[string]string{"email": "stale@example.test", "role": "admin"})
	expect(response, 201)
	json.Unmarshal(response.Body.Bytes(), &created)
	if _, err = pool.Exec(ctx, "UPDATE invites SET expires_at=now()-interval '1 minute' WHERE id=$1", created["id"]); err != nil {
		t.Fatal(err)
	}
	expiredToken := created["token"]
	expired := callAs("POST", "/api/invite/"+expiredToken, "", map[string]string{"name": "Stale Person", "password": "member-long-password"})
	expect(expired, 400)
	mustContain(expired, "This invitation has expired")
	replacement := callAs("POST", "/api/invites", adminCookie, map[string]string{"email": "stale@example.test", "role": "member"})
	expect(replacement, 201)
	var replaced map[string]string
	json.Unmarshal(replacement.Body.Bytes(), &replaced)
	if replaced["token"] == "" || replaced["token"] == expiredToken {
		t.Fatal("Expired invitation was not replaced")
	}
	expect(callAs("POST", "/api/invite/"+expiredToken, "", map[string]string{"name": "Stale Person", "password": "member-long-password"}), 404)
	for _, forbidden := range []struct {
		method, path string
		body         any
	}{
		{"POST", "/api/sections", map[string]string{"id": "vendors", "name": "Vendors"}},
		{"POST", "/api/sections/clients/fields", map[string]any{"id": "region", "label": "Region", "type": "text"}},
		{"GET", "/api/agents", nil},
		{"POST", "/api/agents", map[string]string{"name": "Member agent"}},
		{"GET", "/api/users", nil},
		{"POST", "/api/invites", map[string]string{"email": "other@example.test", "role": "member"}},
		{"PUT", "/api/users/admin/role", map[string]string{"role": "member"}},
		{"DELETE", "/api/users/admin", nil},
	} {
		denied := callAs(forbidden.method, forbidden.path, memberCookie, forbidden.body)
		expect(denied, 403)
		mustContain(denied, "Administrator access required")
	}
	expect(callAs("POST", "/api/sections/clients/records", memberCookie, map[string]any{"data": map[string]any{"name": "Member Record"}}), 200)
	demote := callAs("PUT", "/api/users/admin/role", adminCookie, map[string]string{"role": "member"})
	expect(demote, 400)
	mustContain(demote, "The workspace must keep one administrator")
	selfRemove := callAs("DELETE", "/api/users/admin", adminCookie, nil)
	expect(selfRemove, 400)
	mustContain(selfRemove, "You cannot remove your own account")
	response = callAs("POST", "/api/invites", adminCookie, map[string]string{"email": "second@example.test", "role": "admin"})
	expect(response, 201)
	json.Unmarshal(response.Body.Bytes(), &created)
	accepted = callAs("POST", "/api/invite/"+created["token"], "", map[string]string{"name": "Second Admin", "password": "second-long-password"})
	expect(accepted, 201)
	secondCookie := accepted.Result().Cookies()[0].Name + "=" + accepted.Result().Cookies()[0].Value
	var secondID string
	if err = pool.QueryRow(ctx, "SELECT id FROM users WHERE email='second@example.test'").Scan(&secondID); err != nil {
		t.Fatal(err)
	}
	expect(callAs("DELETE", "/api/users/"+secondID, adminCookie, nil), 200)
	expect(callAs("GET", "/api/sections", secondCookie, nil), 401)
	expect(callAs("GET", "/api/me", adminCookie, nil), 200)
	var audits int
	if err = pool.QueryRow(ctx, "SELECT count(*) FROM audit WHERE action IN ('invite_create','invite_revoke','invite_accept','user_role','user_remove')").Scan(&audits); err != nil || audits < 5 {
		t.Fatal("User and invite auditing missing", audits, err)
	}
}
