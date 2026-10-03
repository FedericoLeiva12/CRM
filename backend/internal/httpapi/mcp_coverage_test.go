package httpapi

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"siracrm/internal/database"
	"siracrm/internal/domain"
	"siracrm/internal/store"
)

// These checks exercise the real MCP transport, database and browser adapters.
// They use the same disposable database fixture as the existing integration suite.
func TestMCPFeatureCoverage(t *testing.T) {
	ctx := context.Background()
	pool, handler, cookie := startCRM(t, ctx)
	defer pool.Close()
	world := &commentWorld{t: t, ctx: ctx, pool: pool, handler: handler, admin: cookie}
	worker := world.agent("Coverage Worker", domain.Permission{SectionID: "clients", Read: true, Write: true})
	teammate := world.agent("Mention Receiver", domain.Permission{SectionID: "clients", Read: true, Write: true})
	outsider := world.agent("Other Section", domain.Permission{SectionID: "prospects", Read: true, Write: true})
	schemaManager := world.agent("Schema Manager")
	expectStatus(t, apiCall(t, handler, cookie, "", "PUT", "/api/agents/"+schemaManager.id+"/permissions", map[string]any{"manage_schema": true}), 200)
	newRecord := func(section, name string, extra map[string]any) string {
		t.Helper()
		data := map[string]any{"name": name}
		for key, value := range extra {
			data[key] = value
		}
		created := world.json(cookie, "POST", "/api/sections/"+section+"/records", map[string]any{"data": data}, 200)
		return created["id"].(string)
	}
	record := newRecord("clients", "Mara Audit", map[string]any{"company": "Northline", "notes": "Unchanged private fixture"})
	otherRecord := newRecord("prospects", "Unrelated", nil)

	t.Run("search matches words across fields and supports pagination", func(t *testing.T) {
		newRecord("clients", "Mara Second", map[string]any{"company": "Northline"})
		newRecord("clients", "Mara Excluded", map[string]any{"company": "Elsewhere"})
		args := map[string]any{"search": "mara northline", "limit": 1, "sort": map[string]any{"field": "name", "direction": "asc"}}
		page := world.callTool(worker, "clients_list", args)
		if page["total"] != float64(2) || len(page["records"].([]any)) != 1 || page["next_cursor"] == nil {
			t.Fatal("cross-field search/page failed", page)
		}
		args["cursor"] = page["next_cursor"]
		next := world.callTool(worker, "clients_list", args)
		if len(next["records"].([]any)) != 1 || next["next_cursor"] != nil {
			t.Fatal("second page failed", next)
		}
		apiPage := world.json(cookie, "POST", "/api/sections/clients/records/query", map[string]any{"search": "mara northline", "limit": 10}, 200)
		if apiPage["total"] != page["total"] {
			t.Fatal("API/MCP search differs")
		}
		if world.toolFailure(worker, "clients_list", map[string]any{"search": strings.Repeat("x", 201)}) == "" {
			t.Fatal("oversize search accepted")
		}
	})

	t.Run("mention discovery hides ineligible agents and non-public account fields", func(t *testing.T) {
		candidates := world.callTool(worker, "clients_mentionables", map[string]any{})
		raw, _ := json.Marshal(candidates)
		if !strings.Contains(string(raw), teammate.handle) || strings.Contains(string(raw), outsider.handle) || strings.Contains(string(raw), "token") || strings.Contains(string(raw), "email") {
			t.Fatal("unexpected mention candidates", string(raw))
		}
		if world.toolFailure(outsider, "clients_mentionables", map[string]any{}) == "" {
			t.Fatal("mention discovery bypassed section read")
		}
	})

	t.Run("comments preserve ownership, section boundaries and reply tombstones", func(t *testing.T) {
		created := world.callTool(worker, "clients_comment", map[string]any{"id": record, "body": "Original"})["comment"].(map[string]any)
		id := created["id"].(string)
		edited := world.callTool(worker, "clients_comment_update", map[string]any{"id": id, "body": "Updated @" + teammate.handle})["comment"].(map[string]any)
		if edited["body"] != "Updated @"+teammate.handle || edited["edited_at"] == nil {
			t.Fatal("comment was not updated", edited)
		}
		if world.toolFailure(teammate, "clients_comment_update", map[string]any{"id": id, "body": "Impersonation"}) == "" {
			t.Fatal("another agent edited the comment")
		}
		if world.toolFailure(teammate, "clients_comment_delete", map[string]any{"id": id}) == "" {
			t.Fatal("another agent deleted the comment")
		}
		// Grant both sections temporarily, then revoke prospects. Knowing and owning a
		// comment there must not allow editing it through a clients-scoped tool.
		expectStatus(t, apiCall(t, handler, cookie, "", "PUT", "/api/agents/"+worker.id+"/permissions", map[string]any{"permissions": []domain.Permission{{SectionID: "prospects", Read: true, Write: true}}}), 200)
		foreign := world.callTool(worker, "prospects_comment", map[string]any{"id": otherRecord, "body": "Foreign own comment"})["comment"].(map[string]any)["id"].(string)
		expectStatus(t, apiCall(t, handler, cookie, "", "PUT", "/api/agents/"+worker.id+"/permissions", map[string]any{"permissions": []domain.Permission{{SectionID: "prospects"}}}), 200)
		for _, name := range []string{"clients_comment_update", "clients_comment_delete"} {
			if world.toolFailure(worker, name, map[string]any{"id": foreign, "body": "Cross-section"}) == "" {
				t.Fatal("cross-section mutation succeeded", name)
			}
		}
		reply := world.callTool(teammate, "clients_comment", map[string]any{"id": record, "body": "Reply", "parent_id": id})["comment"].(map[string]any)["id"].(string)
		world.callTool(worker, "clients_comment_delete", map[string]any{"id": id})
		page := world.callTool(worker, "clients_comments", map[string]any{"id": record})
		comments := page["comments"].([]any)
		if len(comments) != 1 || comments[0].(map[string]any)["deleted"] != true {
			t.Fatal("parent tombstone missing", page)
		}
		world.callTool(teammate, "clients_comment_delete", map[string]any{"id": reply})
		if count := len(world.callTool(worker, "clients_comments", map[string]any{"id": record})["comments"].([]any)); count != 0 {
			t.Fatal("last reply left an orphan tombstone", count)
		}
	})

	t.Run("mark all mentions is scoped to the caller", func(t *testing.T) {
		for i := 0; i < 2; i++ {
			world.callTool(worker, "clients_comment", map[string]any{"id": record, "body": "Ping @" + teammate.handle + " and @admin"})
		}
		notifications := world.callTool(teammate, "mentions_list", map[string]any{"unread_only": true})
		if notifications["unread_count"].(float64) < 2 {
			t.Fatal("missing mentions", notifications)
		}
		marked := world.callTool(teammate, "mentions_mark_read", map[string]any{"all": true})
		if marked["marked"].(float64) < 2 {
			t.Fatal("all did not mark mentions", marked)
		}
		if world.callTool(teammate, "mentions_list", map[string]any{"unread_only": true})["unread_count"] != float64(0) {
			t.Fatal("unread mentions remain")
		}
		if world.json(cookie, "GET", "/api/mentions?unread_only=true", nil, 200)["unread_count"] == float64(0) {
			t.Fatal("another principal's mentions were marked")
		}
		if world.toolFailure(teammate, "mentions_mark_read", map[string]any{}) == "" {
			t.Fatal("empty selection accepted")
		}
	})

	t.Run("schema managers discover and configure views without record access", func(t *testing.T) {
		schema := world.callTool(schemaManager, "sections_schema", map[string]any{})
		if len(schema["sections"].([]any)) != 2 {
			t.Fatal("schema manager cannot discover definitions", schema)
		}
		if slices.Contains(world.toolNames(schemaManager), "clients_get") {
			t.Fatal("schema management granted record read")
		}
		catalog := world.callTool(worker, "item_view_types", map[string]any{})
		if len(catalog["views"].([]any)) < 2 {
			t.Fatal("view catalog missing", catalog)
		}
		info := domain.SectionView{ID: "info", Enabled: true, Config: map[string]any{}}
		activity := domain.SectionView{ID: "activity", Enabled: true, Config: map[string]any{}}
		args := map[string]any{"section": "clients", "views": []domain.SectionView{info, activity}}
		if world.toolFailure(worker, "section_views_configure", args) == "" {
			t.Fatal("writer configured views without schema grant")
		}
		configured := world.callTool(schemaManager, "section_views_configure", args)
		if len(configured["views"].([]any)) != 2 {
			t.Fatal("view configuration failed", configured)
		}
		args["views"] = []domain.SectionView{activity}
		if world.toolFailure(schemaManager, "section_views_configure", args) == "" {
			t.Fatal("Info was removed")
		}
		args["views"] = []domain.SectionView{info, {ID: "activity", Enabled: false, Config: map[string]any{}}}
		world.callTool(schemaManager, "section_views_configure", args)
		var audits int
		if err := pool.QueryRow(ctx, "SELECT count(*) FROM audit WHERE action='configure_item_views' AND actor=$1", "agent:"+schemaManager.id).Scan(&audits); err != nil || audits != 2 {
			t.Fatal("view writes not audited", audits, err)
		}
		expectStatus(t, apiCall(t, handler, cookie, "", "PUT", "/api/agents/"+schemaManager.id+"/permissions", map[string]any{"manage_schema": false}), 200)
		if slices.Contains(world.toolNames(schemaManager), "section_views_configure") || world.toolFailure(schemaManager, "section_views_configure", args) == "" {
			t.Fatal("schema revocation did not apply")
		}
	})

	t.Run("permanent record deletion requires its own grant", func(t *testing.T) {
		doomed := newRecord("clients", "Deletion fixture", nil)
		if slices.Contains(world.toolNames(worker), "clients_delete") || world.toolFailure(worker, "clients_delete", map[string]any{"id": doomed}) == "" {
			t.Fatal("ordinary writing granted deletion")
		}
		permission := domain.Permission{SectionID: "clients", Read: true, Write: true, Delete: true}
		expectStatus(t, apiCall(t, handler, cookie, "", "PUT", "/api/agents/"+worker.id+"/permissions", map[string]any{"permissions": []domain.Permission{permission}}), 200)
		world.callTool(worker, "clients_delete", map[string]any{"id": doomed})
		expectStatus(t, apiCall(t, handler, cookie, "", "GET", "/api/sections/clients/records/"+doomed, nil), 404)
		permission.Delete = false
		expectStatus(t, apiCall(t, handler, cookie, "", "PUT", "/api/agents/"+worker.id+"/permissions", map[string]any{"permissions": []domain.Permission{permission}}), 200)
		if slices.Contains(world.toolNames(worker), "clients_delete") {
			t.Fatal("delete revocation did not apply")
		}
		before := world.callTool(worker, "clients_get", map[string]any{"id": record})["data"]
		expectStatus(t, apiCall(t, handler, cookie, "", "PUT", "/api/agents/"+worker.id+"/permissions", map[string]any{"manage_schema": true, "permissions": []domain.Permission{{SectionID: "clients", Delete: true}}}), 400)
		if slices.Contains(world.toolNames(worker), "sections_create") {
			t.Fatal("rejected grants enabled schema management")
		}
		after := world.callTool(worker, "clients_get", map[string]any{"id": record})["data"]
		if before.(map[string]any)["name"] != after.(map[string]any)["name"] {
			t.Fatal("invalid permissions changed access")
		}
	})
}

// A rejected write-only update must not alter either the old grants or schema access.
func TestMCPWriteRequiresRead(t *testing.T) {
	ctx := context.Background()
	pool, handler, cookie := startCRM(t, ctx)
	defer pool.Close()
	world := &commentWorld{t: t, ctx: ctx, pool: pool, handler: handler, admin: cookie}
	created := world.json(cookie, "POST", "/api/sections/clients/records", map[string]any{"data": map[string]any{"name": "Prerequisite fixture", "notes": "Original value"}}, 200)
	agent := world.agent("Prerequisite fixture", domain.Permission{SectionID: "clients", Read: true})
	id := created["id"].(string)
	expectStatus(t, apiCall(t, handler, cookie, "", "PUT", "/api/agents/"+agent.id+"/permissions", map[string]any{"manage_schema": true, "permissions": []domain.Permission{{SectionID: "clients", Write: true}}}), 400)
	if slices.Contains(world.toolNames(agent), "sections_create") || slices.Contains(world.toolNames(agent), "clients_update") {
		t.Fatal("rejected grants changed access")
	}
	if world.toolFailure(agent, "clients_update", map[string]any{"id": id, "data": map[string]any{}}) == "" {
		t.Fatal("read-only agent wrote a record")
	}
	if world.callTool(agent, "clients_get", map[string]any{"id": id})["data"].(map[string]any)["notes"] != "Original value" {
		t.Fatal("read-only grant changed")
	}
	expectStatus(t, apiCall(t, handler, cookie, "", "PUT", "/api/agents/"+agent.id+"/permissions", map[string]any{"permissions": []domain.Permission{{SectionID: "clients", Read: true, Write: true}}}), 200)
	updated := world.callTool(agent, "clients_update", map[string]any{"id": id, "data": map[string]any{"status": "Active"}})["data"].(map[string]any)
	if updated["notes"] != "Original value" || updated["status"] != "Active" {
		t.Fatal("writer's read response lost fields", updated)
	}
	// SQL writes must satisfy the same invariant even when the API is bypassed.
	if _, err := pool.Exec(ctx, "UPDATE permissions SET can_read=false WHERE agent_id=$1 AND section_id='clients'", agent.id); err == nil {
		t.Fatal("database accepted write without read")
	}
	expectStatus(t, apiCall(t, handler, cookie, "", "PUT", "/api/agents/"+agent.id+"/permissions", map[string]any{"permissions": []domain.Permission{{SectionID: "clients"}}}), 200)
	if world.toolFailure(agent, "clients_update", map[string]any{"id": id, "data": map[string]any{}}) == "" || world.toolFailure(agent, "clients_get", map[string]any{"id": id}) == "" {
		t.Fatal("revoked grant still allowed access")
	}
}

func TestWriteRequiresReadMigration(t *testing.T) {
	ctx := context.Background()
	pool, handler, cookie := startCRM(t, ctx)
	defer pool.Close()
	world := &commentWorld{t: t, ctx: ctx, pool: pool, handler: handler, admin: cookie}
	agent := world.agent("Legacy grants")
	// Recreate the pre-010 permission state in the disposable test database only.
	if _, err := pool.Exec(ctx, "ALTER TABLE permissions DROP CONSTRAINT permissions_write_requires_read; DELETE FROM schema_migrations WHERE version='migrations/010_write_requires_read.sql'"); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "INSERT INTO permissions(agent_id,section_id,can_read,can_write) VALUES($1,'clients',false,true),($1,'prospects',true,false)", agent.id); err != nil {
		t.Fatal(err)
	}
	repository := store.New(pool)
	if repository.CanAccess(ctx, agent.id, "clients", domain.WriteAccess) {
		t.Fatal("legacy inconsistent row was authorized before migration")
	}
	for i := 0; i < 2; i++ {
		if err := database.Migrate(ctx, pool); err != nil {
			t.Fatal(err)
		}
	}
	var read, write, canDelete bool
	if err := pool.QueryRow(ctx, "SELECT can_read,can_write,can_delete FROM permissions WHERE agent_id=$1 AND section_id='clients'", agent.id).Scan(&read, &write, &canDelete); err != nil || !read || !write || canDelete {
		t.Fatal("legacy writer migration failed", read, write, canDelete, err)
	}
	if err := pool.QueryRow(ctx, "SELECT can_read,can_write FROM permissions WHERE agent_id=$1 AND section_id='prospects'", agent.id).Scan(&read, &write); err != nil || !read || write {
		t.Fatal("migration changed a read-only grant", read, write, err)
	}
	var count int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM schema_migrations WHERE version='migrations/010_write_requires_read.sql'").Scan(&count); err != nil || count != 1 {
		t.Fatal("migration was not idempotent", count, err)
	}
}
