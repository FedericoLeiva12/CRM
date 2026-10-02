package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
	"siracrm/internal/domain"
)

type commentWorld struct {
	t       *testing.T
	ctx     context.Context
	pool    *pgxpool.Pool
	handler http.Handler
	admin   string
	member  string
	other   string
}

type testAgent struct {
	id, token, handle string
}

func (world *commentWorld) login(email string) string {
	world.t.Helper()
	response := apiCall(world.t, world.handler, "", "", "POST", "/api/login", map[string]string{"email": email, "password": "a-long-test-password"})
	expectStatus(world.t, response, 200)
	cookie := response.Result().Cookies()[0]
	return cookie.Name + "=" + cookie.Value
}

func (world *commentWorld) agent(name string, permissions ...domain.Permission) testAgent {
	world.t.Helper()
	response := apiCall(world.t, world.handler, world.admin, "", "POST", "/api/agents", map[string]string{"name": name})
	expectStatus(world.t, response, 201)
	var created map[string]string
	if err := json.Unmarshal(response.Body.Bytes(), &created); err != nil {
		world.t.Fatal(err)
	}
	agent := testAgent{id: created["id"], token: created["token"]}
	if len(permissions) > 0 {
		expectStatus(world.t, apiCall(world.t, world.handler, world.admin, "", "PUT", "/api/agents/"+agent.id+"/permissions", map[string]any{"permissions": permissions}), 200)
	}
	if err := world.pool.QueryRow(world.ctx, "SELECT handle FROM agents WHERE id=$1", agent.id).Scan(&agent.handle); err != nil {
		world.t.Fatal(err)
	}
	return agent
}

func (world *commentWorld) tool(agent testAgent, name string, arguments any) *httptest.ResponseRecorder {
	world.t.Helper()
	response := apiCall(world.t, world.handler, "", agent.token, "POST", "/mcp", map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": map[string]any{"name": name, "arguments": arguments}})
	expectStatus(world.t, response, 200)
	return response
}

func (world *commentWorld) callTool(agent testAgent, name string, arguments any) map[string]any {
	world.t.Helper()
	return toolPayload(world.t, world.tool(agent, name, arguments).Body.Bytes())
}

// toolFailure returns the error text when the call failed, or "" when it succeeded.
func (world *commentWorld) toolFailure(agent testAgent, name string, arguments any) string {
	world.t.Helper()
	var payload struct {
		Result struct {
			IsError bool             `json:"isError"`
			Content []map[string]any `json:"content"`
		} `json:"result"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	raw := world.tool(agent, name, arguments).Body.Bytes()
	if err := json.Unmarshal(raw, &payload); err != nil {
		world.t.Fatal(err, string(raw))
	}
	if payload.Error != nil {
		return payload.Error.Message
	}
	if payload.Result.IsError {
		if len(payload.Result.Content) > 0 {
			text, _ := payload.Result.Content[0]["text"].(string)
			return text
		}
		return "error"
	}
	return ""
}

func (world *commentWorld) toolNames(agent testAgent) []string {
	world.t.Helper()
	response := apiCall(world.t, world.handler, "", agent.token, "POST", "/mcp", map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/list", "params": map[string]any{}})
	expectStatus(world.t, response, 200)
	var payload struct {
		Result struct {
			Tools []struct {
				Name string `json:"name"`
			} `json:"tools"`
		} `json:"result"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		world.t.Fatal(err)
	}
	names := []string{}
	for _, tool := range payload.Result.Tools {
		names = append(names, tool.Name)
	}
	return names
}

func (world *commentWorld) json(cookie, method, path string, body any, status int) map[string]any {
	world.t.Helper()
	response := apiCall(world.t, world.handler, cookie, "", method, path, body)
	expectStatus(world.t, response, status)
	var payload map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		world.t.Fatal(err, response.Body.String())
	}
	return payload
}

func (world *commentWorld) outbox(eventType string) []map[string]any {
	world.t.Helper()
	rows, err := world.pool.Query(world.ctx, "SELECT payload FROM webhook_outbox WHERE event_type=$1 ORDER BY created_at, id", eventType)
	if err != nil {
		world.t.Fatal(err)
	}
	defer rows.Close()
	events := []map[string]any{}
	for rows.Next() {
		var raw string
		if err = rows.Scan(&raw); err != nil {
			world.t.Fatal(err)
		}
		var event map[string]any
		if err = json.Unmarshal([]byte(raw), &event); err != nil {
			world.t.Fatal(err)
		}
		events = append(events, event)
	}
	return events
}

func (world *commentWorld) resetOutbox() {
	world.t.Helper()
	if _, err := world.pool.Exec(world.ctx, "DELETE FROM webhook_outbox"); err != nil {
		world.t.Fatal(err)
	}
}

func handles(items any) []string {
	list, _ := items.([]any)
	result := []string{}
	for _, item := range list {
		entry, _ := item.(map[string]any)
		result = append(result, fmt.Sprint(entry["handle"]))
	}
	return result
}

func ids(items any) []string {
	list, _ := items.([]any)
	result := []string{}
	for _, item := range list {
		entry, _ := item.(map[string]any)
		result = append(result, fmt.Sprint(entry["id"]))
	}
	return result
}

func strs(items any) []string {
	list, _ := items.([]any)
	result := []string{}
	for _, item := range list {
		result = append(result, fmt.Sprint(item))
	}
	return result
}

func TestComments(t *testing.T) {
	ctx := context.Background()
	pool, handler, adminCookie := startCRM(t, ctx)
	defer pool.Close()
	hash, _ := bcrypt.GenerateFromPassword([]byte("a-long-test-password"), bcrypt.MinCost)
	if _, err := pool.Exec(ctx, `INSERT INTO users(id,email,password_hash,name,role) VALUES
		('nia','nia@example.test',$1,'Nia Cole','member'),
		('nia2','nia.cole@example.test',$1,'Nia Cole','member')`, string(hash)); err != nil {
		t.Fatal(err)
	}
	world := &commentWorld{t: t, ctx: ctx, pool: pool, handler: handler, admin: adminCookie}
	world.member = world.login("nia@example.test")
	world.other = world.login("nia.cole@example.test")

	scout := world.agent("Scout Bot", domain.Permission{SectionID: "prospects", Read: true, Write: true})
	scoutTwin := world.agent("Scout Bot", domain.Permission{SectionID: "prospects", Read: true, Write: true})
	reader := world.agent("Read Only", domain.Permission{SectionID: "prospects", Read: true})
	writer := world.agent("Write Only", domain.Permission{SectionID: "prospects", Write: true})
	quiet := world.agent("Quiet Agent", domain.Permission{SectionID: "clients", Read: true, Write: true})
	nobody := world.agent("No Grants")

	newRecord := func(section, name string) string {
		t.Helper()
		return world.json(adminCookie, "POST", "/api/sections/"+section+"/records", map[string]any{"data": map[string]any{"name": name}}, 200)["id"].(string)
	}
	record := newRecord("prospects", "Ada Prospect")
	otherRecord := newRecord("prospects", "Bea Prospect")
	clientRecord := newRecord("clients", "Mara Client")
	commentsPath := func(section, recordID string) string {
		return "/api/sections/" + section + "/records/" + recordID + "/comments"
	}
	postComment := func(cookie, body string, extra map[string]any) map[string]any {
		t.Helper()
		payload := map[string]any{"body": body}
		for key, value := range extra {
			payload[key] = value
		}
		return world.json(cookie, "POST", commentsPath("prospects", record), payload, 201)
	}

	t.Run("handles are derived, unique across users and agents, and exposed", func(t *testing.T) {
		var adminHandle, niaHandle, nia2Handle string
		pool.QueryRow(ctx, "SELECT handle FROM users WHERE id='admin'").Scan(&adminHandle)
		pool.QueryRow(ctx, "SELECT handle FROM users WHERE id='nia'").Scan(&niaHandle)
		pool.QueryRow(ctx, "SELECT handle FROM users WHERE id='nia2'").Scan(&nia2Handle)
		if adminHandle != "admin" || niaHandle != "nia-cole" || nia2Handle != "nia-cole-2" {
			t.Fatalf("user handles: %q %q %q", adminHandle, niaHandle, nia2Handle)
		}
		if scout.handle != "scout-bot" || scoutTwin.handle != "scout-bot-2" || reader.handle != "read-only" {
			t.Fatalf("agent handles: %q %q %q", scout.handle, scoutTwin.handle, reader.handle)
		}
		// A user who later takes an agent's name must not share its handle.
		if _, err := pool.Exec(ctx, "INSERT INTO users(id,email,password_hash,name,role) VALUES('clash','clash@example.test','x','Scout Bot','member')"); err != nil {
			t.Fatal(err)
		}
		var clash string
		pool.QueryRow(ctx, "SELECT handle FROM users WHERE id='clash'").Scan(&clash)
		if clash != "scout-bot-3" {
			t.Fatalf("handle collided across namespaces: %q", clash)
		}
		for input, want := range map[string]string{"José Núñez!!": "jose-nunez", "   ": "member", "A.B+C@x": "a-b-c-x", strings.Repeat("a", 60): strings.Repeat("a", 28)} {
			var got string
			if err := pool.QueryRow(ctx, "SELECT sira_unique_handle($1,'member','probe')", input).Scan(&got); err != nil || got != want {
				t.Fatalf("derive(%q) = %q (%v), want %q", input, got, err, want)
			}
		}
		if _, err := pool.Exec(ctx, "INSERT INTO agents(id,name,token_hash,handle) VALUES('dup','Dup','dup-hash','admin')"); err == nil {
			t.Fatal("a database-level duplicate handle was accepted")
		}
		response := apiCall(t, handler, adminCookie, "", "GET", "/api/users", nil)
		if !strings.Contains(response.Body.String(), `"handle":"nia-cole"`) {
			t.Fatal("users list does not expose handles", response.Body.String())
		}
		response = apiCall(t, handler, adminCookie, "", "GET", "/api/agents", nil)
		if !strings.Contains(response.Body.String(), `"handle":"scout-bot"`) {
			t.Fatal("agents list does not expose handles", response.Body.String())
		}
		me := world.json(world.member, "GET", "/api/me", nil, 200)
		if me["handle"] != "nia-cole" {
			t.Fatalf("/api/me handle = %v", me["handle"])
		}
		if _, err := pool.Exec(ctx, "DELETE FROM users WHERE id='clash'"); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("tools follow read and write grants", func(t *testing.T) {
		has := func(agent testAgent, names ...string) []bool {
			list := world.toolNames(agent)
			result := []bool{}
			for _, name := range names {
				result = append(result, slices.Contains(list, name))
			}
			return result
		}
		names := []string{"prospects_comment", "prospects_comments", "mentions_list", "mentions_mark_read", "clients_comment"}
		for agent, want := range map[string][]bool{
			"scout":  {true, true, true, true, false},
			"reader": {false, true, true, true, false},
			"writer": {true, false, true, true, false},
			"nobody": {false, false, true, true, false},
			"quiet":  {false, false, true, true, true},
		} {
			agents := map[string]testAgent{"scout": scout, "reader": reader, "writer": writer, "nobody": nobody, "quiet": quiet}
			if got := has(agents[agent], names...); !slices.Equal(got, want) {
				t.Fatalf("%s tools %v, want %v (%v)", agent, got, want, world.toolNames(agents[agent]))
			}
		}
		// Discovery is not authorization: calls are rechecked.
		if world.toolFailure(reader, "prospects_comment", map[string]any{"id": record, "body": "no"}) == "" {
			t.Fatal("a read-only agent commented")
		}
		if world.toolFailure(writer, "prospects_comments", map[string]any{"id": record}) == "" {
			t.Fatal("a write-only agent listed comments")
		}
		if world.toolFailure(nobody, "prospects_comment", map[string]any{"id": record, "body": "no"}) == "" {
			t.Fatal("an agent without grants commented")
		}
		if world.toolFailure(scout, "clients_comment", map[string]any{"id": clientRecord, "body": "no"}) == "" {
			t.Fatal("an agent commented in a section it has no grant for")
		}
		var count int
		pool.QueryRow(ctx, "SELECT count(*) FROM activities WHERE type='comment'").Scan(&count)
		if count != 0 {
			t.Fatalf("denied calls created %d comments", count)
		}
	})

	t.Run("mentions resolve from handles and ids", func(t *testing.T) {
		world.resetOutbox()
		body := "Hey @admin and @Scout-Bot, ping @ghost and @quiet-agent plus @nia-cole (me). Mail nia@example.test, code `@read-only`."
		result := postComment(world.member, body, nil)
		comment := result["comment"].(map[string]any)
		if got := handles(comment["mentions"]); !slices.Equal(got, []string{"admin", "scout-bot"}) {
			t.Fatalf("resolved mentions = %v", got)
		}
		if got := strs(result["unresolved_mentions"]); !slices.Equal(got, []string{"@ghost", "@quiet-agent"}) {
			t.Fatalf("unresolved = %v", got)
		}
		author := comment["author"].(map[string]any)
		if author["kind"] != "user" || author["id"] != "nia" || author["name"] != "Nia Cole" || author["handle"] != "nia-cole" {
			t.Fatalf("author = %v", author)
		}
		if comment["body"] != body || comment["edited_at"] != nil || comment["parent_id"] != nil {
			t.Fatalf("comment = %v", comment)
		}
		var mentionRows int
		pool.QueryRow(ctx, "SELECT count(*) FROM comment_mentions WHERE entry_id=$1", comment["id"]).Scan(&mentionRows)
		if mentionRows != 2 {
			t.Fatalf("stored %d mention rows", mentionRows)
		}
		// Explicit ids work on their own, with or without a kind prefix, and are not duplicated by a handle.
		var scoutTwinID string
		pool.QueryRow(ctx, "SELECT id FROM agents WHERE handle='scout-bot-2'").Scan(&scoutTwinID)
		result = postComment(world.member, "ids only, plus @admin", map[string]any{"mentions": []string{scoutTwinID, "user:admin", "agent:" + reader.id, "agent:nope", "missing-id"}})
		comment = result["comment"].(map[string]any)
		if got := handles(comment["mentions"]); !slices.Equal(got, []string{"admin", "read-only", "scout-bot-2"}) {
			t.Fatalf("explicit mentions = %v", got)
		}
		if got := strs(result["unresolved_mentions"]); !slices.Equal(got, []string{"agent:nope", "missing-id"}) {
			t.Fatalf("unresolved explicit = %v", got)
		}
		expectStatus(t, apiCall(t, handler, world.member, "", "POST", commentsPath("prospects", record), map[string]any{"body": "x", "mentions": []string{"group:x"}}), 400)
		// Resolved mentions are capped.
		var manyHandles []string
		for index := range 21 {
			id := fmt.Sprintf("cap%d", index)
			if _, err := pool.Exec(ctx, "INSERT INTO users(id,email,password_hash,name) VALUES($1,$2,'x',$3)", id, id+"@example.test", "Cap Person "+id); err != nil {
				t.Fatal(err)
			}
			manyHandles = append(manyHandles, "@cap-person-"+id)
		}
		expectStatus(t, apiCall(t, handler, world.member, "", "POST", commentsPath("prospects", record), map[string]any{"body": strings.Join(manyHandles, " ")}), 400)
		pool.Exec(ctx, "DELETE FROM users WHERE id LIKE 'cap%'")
	})

	t.Run("agents mention by handle and are validated like everyone else", func(t *testing.T) {
		result := world.callTool(scout, "prospects_comment", map[string]any{"id": record, "body": "Following up with @nia-cole and @admin, and @scout-bot (myself)"})
		comment := result["comment"].(map[string]any)
		author := comment["author"].(map[string]any)
		if author["kind"] != "agent" || author["id"] != scout.id || author["handle"] != "scout-bot" {
			t.Fatalf("author = %v", author)
		}
		if got := handles(comment["mentions"]); !slices.Equal(got, []string{"admin", "nia-cole"}) {
			t.Fatalf("mentions = %v", got)
		}
		if got := strs(result["unresolved_mentions"]); len(got) != 0 {
			t.Fatalf("self mention was reported: %v", got)
		}
		if message := world.toolFailure(scout, "prospects_comment", map[string]any{"id": record, "body": "   "}); !strings.Contains(message, "Write a comment") {
			t.Fatalf("empty body error = %q", message)
		}
		if message := world.toolFailure(scout, "prospects_comment", map[string]any{"id": "missing", "body": "hi"}); !strings.Contains(message, "does not exist") {
			t.Fatalf("missing record error = %q", message)
		}
	})

	t.Run("webhook events", func(t *testing.T) {
		world.resetOutbox()
		endpoint := world.json(adminCookie, "POST", "/api/webhooks", map[string]any{
			"url": "https://1.1.1.1/hook", "description": "agents", "enabled": true, "section_id": "",
			"event_types": []string{"comment.mentioned"}, "signing_secret": "a-long-secret",
		}, 201)
		endpointID := endpoint["id"].(string)
		sectionScoped := world.json(adminCookie, "POST", "/api/webhooks", map[string]any{
			"url": "https://1.1.1.1/clients", "enabled": true, "section_id": "clients",
			"event_types": []string{"comment.mentioned"}, "signing_secret": "a-long-secret",
		}, 201)
		expectStatus(t, apiCall(t, handler, adminCookie, "", "POST", "/api/webhooks", map[string]any{
			"url": "https://1.1.1.1/bad", "enabled": true, "section_id": "", "event_types": []string{"comment.nope"}, "signing_secret": "a-long-secret",
		}), 400)

		long := strings.Repeat("é", 700)
		parent := postComment(world.member, "Parent for @scout-bot", nil)["comment"].(map[string]any)["id"].(string)
		world.resetOutbox()
		result := postComment(world.member, "@admin and @scout-bot "+long, map[string]any{"parent_id": parent})
		entryID := result["comment"].(map[string]any)["id"].(string)

		created := world.outbox("timeline.entry_created")
		if len(created) != 1 {
			t.Fatalf("expected one timeline.entry_created, got %d", len(created))
		}
		createdData := created[0]["data"].(map[string]any)
		if createdData["kind"] != "comment" || createdData["entry_id"] != entryID {
			t.Fatalf("timeline event = %v", createdData)
		}

		mentioned := world.outbox("comment.mentioned")
		if len(mentioned) != 2 {
			t.Fatalf("expected one comment.mentioned per principal, got %d", len(mentioned))
		}
		byHandle := map[string]map[string]any{}
		for _, event := range mentioned {
			if event["type"] != "comment.mentioned" || event["schema_version"] != float64(1) || event["record_id"] != record {
				t.Fatalf("envelope = %v", event)
			}
			actor := event["actor"].(map[string]any)
			if actor["kind"] != "user" || actor["id"] != "nia" {
				t.Fatalf("actor = %v", actor)
			}
			section := event["section"].(map[string]any)
			if section["id"] != "prospects" || section["name"] != "Prospects" {
				t.Fatalf("section = %v", section)
			}
			data := event["data"].(map[string]any)
			byHandle[data["mentioned"].(map[string]any)["handle"].(string)] = data
		}
		for handle, kind := range map[string]string{"admin": "user", "scout-bot": "agent"} {
			data, ok := byHandle[handle]
			if !ok {
				t.Fatalf("no event for %s: %v", handle, byHandle)
			}
			mentionedPrincipal := data["mentioned"].(map[string]any)
			if mentionedPrincipal["kind"] != kind || mentionedPrincipal["id"] == "" {
				t.Fatalf("mentioned = %v", mentionedPrincipal)
			}
			author := data["author"].(map[string]any)
			if author["kind"] != "user" || author["id"] != "nia" || author["name"] != "Nia Cole" || author["handle"] != "nia-cole" {
				t.Fatalf("author = %v", author)
			}
			if data["entry_id"] != entryID || data["record_id"] != record || data["parent_id"] != parent {
				t.Fatalf("payload = %v", data)
			}
			section := data["section"].(map[string]any)
			if section["id"] != "prospects" || section["name"] != "Prospects" {
				t.Fatalf("data.section = %v", section)
			}
			body := data["body"].(string)
			if len([]rune(body)) != 500 || !strings.HasPrefix(body, "@admin and @scout-bot é") {
				t.Fatalf("body truncated to %d runes", len([]rune(body)))
			}
		}
		// A top-level comment carries a null parent_id.
		world.resetOutbox()
		postComment(world.member, "top level @admin", nil)
		top := world.outbox("comment.mentioned")
		if len(top) != 1 {
			t.Fatalf("expected one event, got %d", len(top))
		}
		if value, present := top[0]["data"].(map[string]any)["parent_id"]; !present || value != nil {
			t.Fatalf("parent_id should be present and null: %v", top[0]["data"])
		}

		// The subscribed endpoint gets a delivery for the mention; the clients-only endpoint does not.
		var deliveries, scopedDeliveries int
		pool.QueryRow(ctx, "SELECT count(*) FROM webhook_deliveries WHERE endpoint_id=$1 AND event_type='comment.mentioned'", endpointID).Scan(&deliveries)
		pool.QueryRow(ctx, "SELECT count(*) FROM webhook_deliveries WHERE endpoint_id=$1", sectionScoped["id"]).Scan(&scopedDeliveries)
		if deliveries != 1 || scopedDeliveries != 0 {
			t.Fatalf("deliveries=%d scoped=%d", deliveries, scopedDeliveries)
		}

		// No mention, no event. A rolled-back comment leaves nothing in the outbox.
		world.resetOutbox()
		postComment(world.member, "nobody here", nil)
		expectStatus(t, apiCall(t, handler, world.member, "", "POST", commentsPath("prospects", "missing"), map[string]any{"body": "hi @admin"}), 404)
		if len(world.outbox("comment.mentioned")) != 0 || len(world.outbox("timeline.entry_created")) != 1 {
			t.Fatal("events were written for comments without mentions or for failed writes")
		}
	})

	t.Run("notifications for users and agents", func(t *testing.T) {
		pool.Exec(ctx, "DELETE FROM activities")
		postComment(world.member, "First for @admin", nil)
		postComment(world.other, "Second for @admin and @nia-cole", nil)
		postComment(world.member, "For @scout-bot only", nil)

		listed := world.json(adminCookie, "GET", "/api/mentions", nil, 200)
		mentions := listed["mentions"].([]any)
		if len(mentions) != 2 || listed["unread_count"] != float64(2) {
			t.Fatalf("admin mentions = %v", listed)
		}
		newest := mentions[0].(map[string]any)
		if !strings.HasPrefix(newest["body"].(string), "Second for") || newest["section_id"] != "prospects" || newest["section_name"] != "Prospects" ||
			newest["record_id"] != record || newest["record_name"] != "Ada Prospect" || newest["read_at"] != nil {
			t.Fatalf("newest = %v", newest)
		}
		if newest["author"].(map[string]any)["handle"] != "nia-cole-2" {
			t.Fatalf("author = %v", newest["author"])
		}
		memberView := world.json(world.member, "GET", "/api/mentions", nil, 200)
		if memberView["unread_count"] != float64(1) || len(memberView["mentions"].([]any)) != 1 {
			t.Fatalf("member mentions = %v", memberView)
		}
		mentionIDs := ids(listed["mentions"])
		// Someone else's notification id is ignored.
		foreign := world.json(world.member, "POST", "/api/mentions/read", map[string]any{"ids": mentionIDs}, 200)
		if foreign["marked"] != float64(0) {
			t.Fatalf("marked someone else's mentions: %v", foreign)
		}
		oldest := mentionIDs[1]
		if got := world.json(adminCookie, "POST", "/api/mentions/read", map[string]any{"ids": []string{oldest}}, 200); got["marked"] != float64(1) {
			t.Fatalf("mark one = %v", got)
		}
		unread := world.json(adminCookie, "GET", "/api/mentions?unread_only=true", nil, 200)
		if unread["unread_count"] != float64(1) || len(unread["mentions"].([]any)) != 1 {
			t.Fatalf("unread = %v", unread)
		}
		all := world.json(adminCookie, "GET", "/api/mentions", nil, 200)
		if len(all["mentions"].([]any)) != 2 || all["mentions"].([]any)[1].(map[string]any)["read_at"] == nil {
			t.Fatalf("read mentions disappeared: %v", all)
		}
		if got := world.json(adminCookie, "POST", "/api/mentions/read", map[string]any{"ids": []string{oldest}}, 200); got["marked"] != float64(0) {
			t.Fatalf("marking twice changed rows: %v", got)
		}
		if got := world.json(adminCookie, "POST", "/api/mentions/read", map[string]any{"all": true}, 200); got["marked"] != float64(1) {
			t.Fatalf("mark all = %v", got)
		}
		expectStatus(t, apiCall(t, handler, adminCookie, "", "POST", "/api/mentions/read", map[string]any{"ids": []string{}}), 400)
		expectStatus(t, apiCall(t, handler, "", "", "GET", "/api/mentions", nil), 401)

		agentList := world.callTool(scout, "mentions_list", map[string]any{})
		agentMentions := agentList["mentions"].([]any)
		if len(agentMentions) != 1 || agentList["unread_count"] != float64(1) {
			t.Fatalf("agent mentions = %v", agentList)
		}
		agentMention := agentMentions[0].(map[string]any)
		if agentMention["body"] != "For @scout-bot only" || agentMention["record_id"] != record || agentMention["section_id"] != "prospects" || agentMention["id"] == "" || agentMention["entry_id"] == "" {
			t.Fatalf("agent mention = %v", agentMention)
		}
		// Another agent cannot mark it.
		other := world.callTool(scoutTwin, "mentions_mark_read", map[string]any{"ids": []string{agentMention["id"].(string)}})
		if other["marked"] != float64(0) {
			t.Fatalf("another agent marked it: %v", other)
		}
		if world.callTool(scout, "mentions_list", map[string]any{"unread_only": true})["unread_count"] != float64(1) {
			t.Fatal("unread count changed")
		}
		// Losing read access hides the mention; getting it back restores it.
		expectStatus(t, apiCall(t, handler, adminCookie, "", "PUT", "/api/agents/"+scout.id+"/permissions", map[string]any{"permissions": []domain.Permission{{SectionID: "prospects", Read: false, Write: true}}}), 200)
		hidden := world.callTool(scout, "mentions_list", map[string]any{})
		if len(hidden["mentions"].([]any)) != 0 || hidden["unread_count"] != float64(0) {
			t.Fatalf("mention leaked after read access was revoked: %v", hidden)
		}
		expectStatus(t, apiCall(t, handler, adminCookie, "", "PUT", "/api/agents/"+scout.id+"/permissions", map[string]any{"permissions": []domain.Permission{{SectionID: "prospects", Read: true, Write: true}}}), 200)
		marked := world.callTool(scout, "mentions_mark_read", map[string]any{"ids": []string{agentMention["id"].(string)}})
		if marked["marked"] != float64(1) {
			t.Fatalf("marked = %v", marked)
		}
		if got := world.callTool(scout, "mentions_list", map[string]any{"unread_only": true}); got["unread_count"] != float64(0) || len(got["mentions"].([]any)) != 0 {
			t.Fatalf("unread after mark = %v", got)
		}
		if world.toolFailure(scout, "mentions_mark_read", map[string]any{"ids": []string{}}) == "" {
			t.Fatal("empty id list was accepted")
		}
		// An agent that lost read access cannot be mentioned any more.
		result := postComment(world.member, "@read-only and @write-only", nil)
		if got := handles(result["comment"].(map[string]any)["mentions"]); !slices.Equal(got, []string{"read-only"}) {
			t.Fatalf("mentions = %v", got)
		}
		if got := strs(result["unresolved_mentions"]); !slices.Equal(got, []string{"@write-only"}) {
			t.Fatalf("a write-only agent cannot read the record: %v", got)
		}
	})

	t.Run("replies, ordering, and pagination", func(t *testing.T) {
		pool.Exec(ctx, "DELETE FROM activities")
		a := postComment(world.member, "A", nil)["comment"].(map[string]any)["id"].(string)
		b := postComment(world.member, "B", nil)["comment"].(map[string]any)["id"].(string)
		c := postComment(world.member, "C", nil)["comment"].(map[string]any)["id"].(string)
		r1 := postComment(world.other, "A reply 1 @admin", map[string]any{"parent_id": a})["comment"].(map[string]any)
		r2 := world.callTool(scout, "prospects_comment", map[string]any{"id": record, "body": "A reply 2", "parent_id": a})["comment"].(map[string]any)
		if r1["parent_id"] != a || r2["parent_id"] != a {
			t.Fatal("reply parent not stored")
		}
		expectStatus(t, apiCall(t, handler, world.member, "", "POST", commentsPath("prospects", record), map[string]any{"body": "too deep", "parent_id": r1["id"]}), 400)
		expectStatus(t, apiCall(t, handler, world.member, "", "POST", commentsPath("prospects", otherRecord), map[string]any{"body": "wrong record", "parent_id": a}), 404)
		expectStatus(t, apiCall(t, handler, world.member, "", "POST", commentsPath("prospects", record), map[string]any{"body": "ghost parent", "parent_id": "nope"}), 404)
		// A non-comment entry cannot be a parent.
		activity := world.callTool(scout, "prospects_log_activity", map[string]any{"id": record, "type": "llamada", "date": "2026-01-02", "summary": "A call"})
		expectStatus(t, apiCall(t, handler, world.member, "", "POST", commentsPath("prospects", record), map[string]any{"body": "x", "parent_id": activity["id"]}), 404)
		if world.toolFailure(scout, "prospects_comment", map[string]any{"id": record, "body": "x", "parent_id": r2["id"]}) == "" {
			t.Fatal("reply to a reply was accepted")
		}

		first := world.callTool(scout, "prospects_comments", map[string]any{"id": record, "limit": 2})
		comments := first["comments"].([]any)
		if len(comments) != 2 || first["next_cursor"] == nil {
			t.Fatalf("first page = %v", first)
		}
		if comments[0].(map[string]any)["id"] != c || comments[1].(map[string]any)["id"] != b {
			t.Fatalf("not newest first: %v", ids(first["comments"]))
		}
		second := world.callTool(scout, "prospects_comments", map[string]any{"id": record, "limit": 2, "cursor": first["next_cursor"]})
		last := second["comments"].([]any)
		if len(last) != 1 || second["next_cursor"] != nil || last[0].(map[string]any)["id"] != a {
			t.Fatalf("second page = %v", second)
		}
		replies := last[0].(map[string]any)["replies"].([]any)
		if len(replies) != 2 || replies[0].(map[string]any)["id"] != r1["id"] || replies[1].(map[string]any)["id"] != r2["id"] {
			t.Fatalf("replies not oldest first: %v", replies)
		}
		if got := handles(replies[0].(map[string]any)["mentions"]); !slices.Equal(got, []string{"admin"}) {
			t.Fatalf("reply mentions = %v", got)
		}
		all := world.callTool(scout, "prospects_comments", map[string]any{"id": record})
		if len(all["comments"].([]any)) != 3 || all["next_cursor"] != nil {
			t.Fatalf("all = %v", all)
		}
		for _, bad := range []map[string]any{{"id": record, "limit": 0}, {"id": record, "limit": 201}, {"id": record, "cursor": "garbage"}, {"id": "missing"}} {
			if world.toolFailure(scout, "prospects_comments", bad) == "" {
				t.Fatalf("accepted %v", bad)
			}
		}
		fromAPI := world.json(world.member, "GET", commentsPath("prospects", record)+"?limit=1", nil, 200)
		if len(fromAPI["comments"].([]any)) != 1 || fromAPI["next_cursor"] == nil {
			t.Fatalf("api page = %v", fromAPI)
		}

		// The record view interleaves comments with every other entry, newest first, with authors and mentions.
		detail := world.json(world.member, "GET", "/api/sections/prospects/records/"+record, nil, 200)
		kinds := map[string]int{}
		for _, item := range detail["activities"].([]any) {
			entry := item.(map[string]any)
			kinds[entry["type"].(string)]++
			if entry["type"] == "comment" {
				author := entry["author"].(map[string]any)
				if author["name"] == "" || author["handle"] == "" {
					t.Fatalf("comment author is anonymous: %v", entry)
				}
				if entry["id"] == r1["id"] && (entry["parent_id"] != a || len(entry["mentions"].([]any)) != 1) {
					t.Fatalf("reply entry = %v", entry)
				}
			}
		}
		if kinds["comment"] != 5 || kinds["llamada"] != 1 {
			t.Fatalf("timeline = %v", kinds)
		}
		stamps := []string{}
		for _, item := range detail["activities"].([]any) {
			stamps = append(stamps, item.(map[string]any)["date"].(string))
		}
		if !slices.IsSortedFunc(stamps, func(left, right string) int { return strings.Compare(right, left) }) {
			t.Fatalf("not newest first: %v", stamps)
		}
	})

	t.Run("editing", func(t *testing.T) {
		pool.Exec(ctx, "DELETE FROM activities")
		created := postComment(world.member, "Draft for @admin and @scout-bot", nil)["comment"].(map[string]any)
		id := created["id"].(string)
		adminMention := world.json(adminCookie, "GET", "/api/mentions", nil, 200)["mentions"].([]any)[0].(map[string]any)["id"].(string)
		world.json(adminCookie, "POST", "/api/mentions/read", map[string]any{"ids": []string{adminMention}}, 200)

		// Only the author edits, even an administrator.
		expectStatus(t, apiCall(t, handler, adminCookie, "", "PUT", "/api/comments/"+id, map[string]any{"body": "hijack"}), 403)
		expectStatus(t, apiCall(t, handler, world.other, "", "PUT", "/api/comments/"+id, map[string]any{"body": "hijack"}), 403)
		expectStatus(t, apiCall(t, handler, world.member, "", "PUT", "/api/comments/"+id, map[string]any{"body": "  "}), 400)
		expectStatus(t, apiCall(t, handler, world.member, "", "PUT", "/api/comments/missing", map[string]any{"body": "x"}), 404)
		expectStatus(t, apiCall(t, handler, "", "", "PUT", "/api/comments/"+id, map[string]any{"body": "x"}), 401)
		var stored string
		pool.QueryRow(ctx, "SELECT summary FROM activities WHERE id=$1", id).Scan(&stored)
		if stored != "Draft for @admin and @scout-bot" {
			t.Fatal("a rejected edit changed the comment", stored)
		}

		world.resetOutbox()
		edited := world.json(world.member, "PUT", "/api/comments/"+id, map[string]any{"body": "Final for @admin and @nia-cole2-nobody and @read-only"}, 200)
		comment := edited["comment"].(map[string]any)
		if comment["edited_at"] == nil || comment["body"] != "Final for @admin and @nia-cole2-nobody and @read-only" {
			t.Fatalf("edited = %v", comment)
		}
		if got := handles(comment["mentions"]); !slices.Equal(got, []string{"admin", "read-only"}) {
			t.Fatalf("mentions after edit = %v (scout-bot was removed, read-only was added)", got)
		}
		if got := strs(edited["unresolved_mentions"]); !slices.Equal(got, []string{"@nia-cole2-nobody"}) {
			t.Fatalf("unresolved = %v", got)
		}
		events := world.outbox("comment.mentioned")
		if len(events) != 1 || events[0]["data"].(map[string]any)["mentioned"].(map[string]any)["handle"] != "read-only" {
			t.Fatalf("an edit must notify only newly mentioned principals: %v", events)
		}
		if len(world.outbox("timeline.entry_created")) != 0 {
			t.Fatal("an edit is not a new timeline entry")
		}
		// The existing mention keeps its read state; the removed one is gone.
		var readAt any
		pool.QueryRow(ctx, "SELECT read_at FROM comment_mentions WHERE id=$1", adminMention).Scan(&readAt)
		if readAt == nil {
			t.Fatal("an edit reset a read notification")
		}
		if got := world.callTool(scout, "mentions_list", map[string]any{}); len(got["mentions"].([]any)) != 0 {
			t.Fatalf("removed mention is still listed: %v", got)
		}
		// Repeating the same text changes nothing.
		world.resetOutbox()
		before := comment["edited_at"]
		again := world.json(world.member, "PUT", "/api/comments/"+id, map[string]any{"body": "Final for @admin and @nia-cole2-nobody and @read-only"}, 200)["comment"].(map[string]any)
		if again["edited_at"] != before || len(world.outbox("comment.mentioned")) != 0 {
			t.Fatal("an unchanged edit was recorded")
		}
		var audits int
		pool.QueryRow(ctx, "SELECT count(*) FROM audit WHERE action='comment_edit' AND detail=$1", id).Scan(&audits)
		if audits != 1 {
			t.Fatalf("edit audits = %d", audits)
		}
		// A person cannot edit an agent's comment.
		agentComment := world.callTool(scout, "prospects_comment", map[string]any{"id": record, "body": "agent says hi"})["comment"].(map[string]any)["id"].(string)
		expectStatus(t, apiCall(t, handler, world.member, "", "PUT", "/api/comments/"+agentComment, map[string]any{"body": "x"}), 403)
		// Other timeline entries are not comments.
		activity := world.callTool(scout, "prospects_log_activity", map[string]any{"id": record, "type": "llamada", "date": "2026-01-02", "summary": "A call"})
		expectStatus(t, apiCall(t, handler, world.member, "", "PUT", "/api/comments/"+activity["id"].(string), map[string]any{"body": "x"}), 404)
		expectStatus(t, apiCall(t, handler, world.member, "", "DELETE", "/api/comments/"+activity["id"].(string), nil), 404)
		expectStatus(t, apiCall(t, handler, world.member, "", "PUT", "/api/comments/"+id, map[string]any{"body": "x", "parent_id": "other"}), 400)
	})

	t.Run("deleting", func(t *testing.T) {
		pool.Exec(ctx, "DELETE FROM activities")
		mine := postComment(world.member, "mine @admin", nil)["comment"].(map[string]any)["id"].(string)
		agentOwn := world.callTool(scout, "prospects_comment", map[string]any{"id": record, "body": "agent comment @nia-cole"})["comment"].(map[string]any)["id"].(string)

		expectStatus(t, apiCall(t, handler, world.other, "", "DELETE", "/api/comments/"+mine, nil), 403)
		expectStatus(t, apiCall(t, handler, world.member, "", "DELETE", "/api/comments/"+agentOwn, nil), 403)
		expectStatus(t, apiCall(t, handler, "", "", "DELETE", "/api/comments/"+mine, nil), 401)
		expectStatus(t, apiCall(t, handler, world.member, "", "DELETE", "/api/comments/missing", nil), 404)
		var count int
		pool.QueryRow(ctx, "SELECT count(*) FROM activities WHERE type='comment'").Scan(&count)
		if count != 2 {
			t.Fatalf("forbidden deletes removed %d comments", 2-count)
		}
		// Authors delete their own; administrators delete any.
		expectStatus(t, apiCall(t, handler, world.member, "", "DELETE", "/api/comments/"+mine, nil), 200)
		expectStatus(t, apiCall(t, handler, adminCookie, "", "DELETE", "/api/comments/"+agentOwn, nil), 200)
		expectStatus(t, apiCall(t, handler, adminCookie, "", "DELETE", "/api/comments/"+agentOwn, nil), 404)
		pool.QueryRow(ctx, "SELECT count(*) FROM activities WHERE type='comment'").Scan(&count)
		var mentionRows int
		pool.QueryRow(ctx, "SELECT count(*) FROM comment_mentions").Scan(&mentionRows)
		if count != 0 || mentionRows != 0 {
			t.Fatalf("comments=%d mentions=%d after deletes", count, mentionRows)
		}
		if got := world.json(adminCookie, "GET", "/api/mentions", nil, 200); len(got["mentions"].([]any)) != 0 || got["unread_count"] != float64(0) {
			t.Fatalf("deleted comments still notify: %v", got)
		}
		var audits int
		pool.QueryRow(ctx, "SELECT count(*) FROM audit WHERE action='comment_delete'").Scan(&audits)
		if audits != 2 {
			t.Fatalf("delete audits = %d", audits)
		}

		// A parent with replies stays as a tombstone; the last reply takes it away.
		parent := postComment(world.member, "thread @admin", nil)["comment"].(map[string]any)["id"].(string)
		replyOne := postComment(world.other, "r1", map[string]any{"parent_id": parent})["comment"].(map[string]any)["id"].(string)
		replyTwo := postComment(world.other, "r2", map[string]any{"parent_id": parent})["comment"].(map[string]any)["id"].(string)
		expectStatus(t, apiCall(t, handler, world.member, "", "DELETE", "/api/comments/"+parent, nil), 200)
		page := world.json(world.member, "GET", commentsPath("prospects", record), nil, 200)["comments"].([]any)
		tomb := page[0].(map[string]any)
		if len(page) != 1 || tomb["deleted"] != true || tomb["body"] != "" || len(tomb["replies"].([]any)) != 2 || len(tomb["mentions"].([]any)) != 0 {
			t.Fatalf("tombstone = %v", page)
		}
		expectStatus(t, apiCall(t, handler, world.member, "", "POST", commentsPath("prospects", record), map[string]any{"body": "late", "parent_id": parent}), 400)
		expectStatus(t, apiCall(t, handler, world.member, "", "PUT", "/api/comments/"+parent, map[string]any{"body": "revive"}), 404)
		expectStatus(t, apiCall(t, handler, world.other, "", "DELETE", "/api/comments/"+replyOne, nil), 200)
		pool.QueryRow(ctx, "SELECT count(*) FROM activities WHERE id=$1", parent).Scan(&count)
		if count != 1 {
			t.Fatal("the tombstone went away while a reply remained")
		}
		expectStatus(t, apiCall(t, handler, adminCookie, "", "DELETE", "/api/comments/"+replyTwo, nil), 200)
		pool.QueryRow(ctx, "SELECT count(*) FROM activities WHERE type='comment'").Scan(&count)
		if count != 0 {
			t.Fatalf("%d comments left after the thread emptied", count)
		}
	})

	t.Run("activity tool comments are real comments", func(t *testing.T) {
		pool.Exec(ctx, "DELETE FROM activities")
		world.resetOutbox()
		logged := world.callTool(scout, "prospects_log_activity", map[string]any{"id": record, "type": "comment", "date": "2026-02-02", "summary": "Logged for @admin"})
		if logged["type"] != "comment" || logged["date"] != "2026-02-02T00:00:00Z" {
			t.Fatalf("logged = %v", logged)
		}
		if got := handles(logged["mentions"]); !slices.Equal(got, []string{"admin"}) {
			t.Fatalf("mentions = %v", got)
		}
		if len(world.outbox("comment.mentioned")) != 1 || len(world.outbox("timeline.entry_created")) != 1 {
			t.Fatal("legacy comment path skipped events")
		}
		page := world.callTool(scout, "prospects_comments", map[string]any{"id": record})
		if len(page["comments"].([]any)) != 1 {
			t.Fatalf("comments = %v", page)
		}
	})

	t.Run("candidates for the composer", func(t *testing.T) {
		got := world.json(world.member, "GET", "/api/sections/prospects/mentionables", nil, 200)
		if users := handles(got["users"]); !slices.Contains(users, "admin") || !slices.Contains(users, "nia-cole") || !slices.Contains(users, "nia-cole-2") {
			t.Fatalf("users = %v", users)
		}
		agents := handles(got["agents"])
		if !slices.Contains(agents, "scout-bot") || !slices.Contains(agents, "scout-bot-2") || !slices.Contains(agents, "read-only") {
			t.Fatalf("agents = %v", agents)
		}
		for _, excluded := range []string{"write-only", "quiet-agent", "no-grants"} {
			if slices.Contains(agents, excluded) {
				t.Fatalf("%s cannot read prospects but was offered: %v", excluded, agents)
			}
		}
		clients := world.json(world.member, "GET", "/api/sections/clients/mentionables", nil, 200)
		if got := handles(clients["agents"]); !slices.Equal(got, []string{"quiet-agent"}) {
			t.Fatalf("clients agents = %v", got)
		}
		expectStatus(t, apiCall(t, handler, world.member, "", "GET", "/api/sections/missing/mentionables", nil), 404)
		expectStatus(t, apiCall(t, handler, "", "", "GET", "/api/sections/prospects/mentionables", nil), 401)
		if body := apiCall(t, handler, world.member, "", "GET", "/api/sections/prospects/mentionables", nil).Body.String(); strings.Contains(body, "token") || strings.Contains(body, "@example.test") {
			t.Fatalf("candidate list leaks credentials or emails: %s", body)
		}
	})

	t.Run("works in every section and cleans up with its owners", func(t *testing.T) {
		pool.Exec(ctx, "DELETE FROM activities")
		world.json(adminCookie, "POST", "/api/sections", map[string]any{"id": "vendors", "name": "Vendors"}, 201)
		vendor := newRecord("vendors", "Acme Supplies")
		expectStatus(t, apiCall(t, handler, adminCookie, "", "PUT", "/api/agents/"+nobody.id+"/permissions", map[string]any{"permissions": []domain.Permission{{SectionID: "vendors", Read: true, Write: true}}}), 200)
		if !slices.Contains(world.toolNames(nobody), "vendors_comment") || !slices.Contains(world.toolNames(nobody), "vendors_comments") {
			t.Fatal("tools were not generated for a new section", world.toolNames(nobody))
		}
		result := world.callTool(nobody, "vendors_comment", map[string]any{"id": vendor, "body": "New section, @admin"})
		if got := handles(result["comment"].(map[string]any)["mentions"]); !slices.Equal(got, []string{"admin"}) {
			t.Fatalf("mentions = %v", got)
		}
		expectStatus(t, apiCall(t, handler, adminCookie, "", "POST", "/api/sections", map[string]any{"id": "mentions", "name": "Mentions"}), 400)

		// Removing a user or revoking an agent removes their notifications.
		postComment(world.member, "bye @nia-cole-2 and @scout-bot", nil)
		var rows int
		pool.QueryRow(ctx, "SELECT count(*) FROM comment_mentions WHERE principal_id IN ('nia2',$1)", scout.id).Scan(&rows)
		if rows != 2 {
			t.Fatalf("mention rows = %d", rows)
		}
		expectStatus(t, apiCall(t, handler, adminCookie, "", "DELETE", "/api/users/nia2", nil), 200)
		expectStatus(t, apiCall(t, handler, adminCookie, "", "DELETE", "/api/agents/"+scout.id, nil), 200)
		pool.QueryRow(ctx, "SELECT count(*) FROM comment_mentions WHERE principal_id IN ('nia2',$1)", scout.id).Scan(&rows)
		if rows != 0 {
			t.Fatalf("%d orphaned mention rows", rows)
		}
		// Deleting a record deletes its comments and notifications.
		expectStatus(t, apiCall(t, handler, adminCookie, "", "DELETE", "/api/sections/vendors/records/"+vendor, nil), 200)
		pool.QueryRow(ctx, "SELECT count(*) FROM comment_mentions m JOIN activities a ON a.id=m.entry_id WHERE a.section_id='vendors'").Scan(&rows)
		var left int
		pool.QueryRow(ctx, "SELECT count(*) FROM activities WHERE section_id='vendors'").Scan(&left)
		if rows != 0 || left != 0 {
			t.Fatalf("record delete left %d mentions and %d entries", rows, left)
		}
	})
}
