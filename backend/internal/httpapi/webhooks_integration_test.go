package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
	"siracrm/internal/auth"
	"siracrm/internal/config"
	"siracrm/internal/database"
	"siracrm/internal/store"
	"siracrm/internal/webhooks"
)

func TestWebhooks(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("Set TEST_DATABASE_URL to an isolated PostgreSQL database")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, databaseURL)
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
	if _, err = pool.Exec(ctx, "INSERT INTO users(id,email,password_hash,name,role) VALUES('admin','admin@example.test',$1,'Ada Admin','admin'),('member','member@example.test',$1,'Nia Cole','member')", string(passwordHash)); err != nil {
		t.Fatal(err)
	}
	repository := store.New(pool)
	devSettings := config.Config{AppOrigin: "http://127.0.0.1:3000", AllowLoopbackWebhooks: true}
	prodSettings := config.Config{AppOrigin: "https://crm.example.test", SecureCookies: true}
	devHandler := New(repository, auth.New(repository), devSettings).Handler()
	prodHandler := New(repository, auth.New(repository), prodSettings).Handler()

	type captured struct {
		body   []byte
		header http.Header
		path   string
	}
	var mu sync.Mutex
	var seen []captured
	var failUntil int
	receiver := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, _ := io.ReadAll(request.Body)
		mu.Lock()
		seen = append(seen, captured{body: body, header: request.Header.Clone(), path: request.URL.Path})
		fail := request.URL.Path == "/retry" && failUntil > 0
		if fail {
			failUntil--
		}
		mu.Unlock()
		if request.URL.Path == "/always-fail" {
			http.Error(writer, "nope", http.StatusInternalServerError)
			return
		}
		if fail {
			http.Error(writer, "later", http.StatusInternalServerError)
			return
		}
		writer.WriteHeader(http.StatusOK)
		_, _ = writer.Write(bytes.Repeat([]byte("x"), 800))
	}))
	defer receiver.Close()
	var ssrfHits int
	ssrfServer := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		mu.Lock()
		ssrfHits++
		mu.Unlock()
	}))
	defer ssrfServer.Close()

	call := func(handler http.Handler, settings config.Config, method, path, cookie string, body any) *httptest.ResponseRecorder {
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
	login := func(handler http.Handler, settings config.Config, email string) string {
		t.Helper()
		response := call(handler, settings, "POST", "/api/login", "", map[string]string{"email": email, "password": "a-long-test-password"})
		expect(response, 200)
		return response.Result().Cookies()[0].Name + "=" + response.Result().Cookies()[0].Value
	}
	adminCookie := login(devHandler, devSettings, "admin@example.test")
	memberCookie := login(devHandler, devSettings, "member@example.test")
	prodAdmin := login(prodHandler, prodSettings, "admin@example.test")

	t.Run("ssrf_save", func(t *testing.T) {
		for _, rawURL := range []string{
			"http://example.com/hook",
			"https://127.0.0.1/hook",
			"https://localhost/hook",
			"https://10.0.0.8/hook",
			"https://192.168.0.4/hook",
			"https://169.254.169.254/latest",
			"https://[::1]/hook",
			"https://[fe80::1]/hook",
			receiver.URL + "/hook",
		} {
			denied := call(prodHandler, prodSettings, "POST", "/api/webhooks", prodAdmin, endpointBody(rawURL, "whsec_test_value_9f3a", ""))
			expect(denied, 400)
		}
	})

	t.Run("member_forbidden", func(t *testing.T) {
		for _, route := range []struct{ method, path string }{
			{"GET", "/api/webhooks"},
			{"POST", "/api/webhooks"},
			{"PUT", "/api/webhooks/missing"},
			{"DELETE", "/api/webhooks/missing"},
			{"POST", "/api/webhooks/missing/test"},
			{"POST", "/api/webhooks/missing/enable"},
			{"POST", "/api/webhooks/missing/disable"},
			{"GET", "/api/webhooks/missing/deliveries"},
		} {
			denied := call(devHandler, devSettings, route.method, route.path, memberCookie, endpointBody(receiver.URL+"/hook", "whsec_test_value_9f3a", ""))
			expect(denied, 403)
			if !strings.Contains(denied.Body.String(), "Administrator access required") {
				t.Fatal(denied.Body.String())
			}
		}
		expect(call(devHandler, devSettings, "GET", "/api/webhooks", "", nil), 401)
		created := call(devHandler, devSettings, "POST", "/api/sections/clients/records", memberCookie, map[string]any{"data": map[string]any{"name": "Member note"}})
		expect(created, 200)
		var queued int
		if err = pool.QueryRow(ctx, "SELECT count(*) FROM webhook_outbox WHERE event_type='record.created'").Scan(&queued); err != nil || queued != 1 {
			t.Fatal("member edit did not reach the outbox", queued, err)
		}
	})

	secret := "whsec_test_value_9f3a"
	headerValue := "Bearer sender-key-9f3a"
	created := call(devHandler, devSettings, "POST", "/api/webhooks", adminCookie, endpointBody(receiver.URL+"/hook", secret, headerValue))
	expect(created, 201)
	if strings.Contains(created.Body.String(), secret) || strings.Contains(created.Body.String(), headerValue) {
		t.Fatal("create response included a secret")
	}
	var endpoint map[string]any
	if err = json.Unmarshal(created.Body.Bytes(), &endpoint); err != nil {
		t.Fatal(err)
	}
	endpointID := jsonID(t, created)
	if endpoint["signing_secret_set"] != true || endpoint["custom_header_set"] != true || endpoint["custom_header_name"] != "Authorization" {
		t.Fatal("auth metadata missing", created.Body.String())
	}
	listed := call(devHandler, devSettings, "GET", "/api/webhooks", adminCookie, nil)
	expect(listed, 200)
	if strings.Contains(listed.Body.String(), secret) || strings.Contains(listed.Body.String(), headerValue) {
		t.Fatal("list response included a secret")
	}

	filtered := call(devHandler, devSettings, "POST", "/api/webhooks", adminCookie, map[string]any{
		"url": receiver.URL + "/prospects", "description": "Prospects only", "event_types": []string{"record.created"},
		"section_id": "prospects", "enabled": true, "signing_secret": secret,
	})
	expect(filtered, 201)
	filteredID := jsonID(t, filtered)
	disabled := call(devHandler, devSettings, "POST", "/api/webhooks", adminCookie, map[string]any{
		"url": receiver.URL + "/disabled", "description": "", "event_types": []string{"record.created"},
		"section_id": "", "enabled": false, "signing_secret": secret,
	})
	expect(disabled, 201)
	disabledID := jsonID(t, disabled)

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err = store.EmitTimelineEntry(ctx, tx, "user:admin", "clients", "record-1", store.TimelineEntry{ID: "entry-1", Kind: "comment", Body: "Hello @ada"}); err != nil {
		t.Fatal(err)
	}
	var inside int
	if err = tx.QueryRow(ctx, "SELECT count(*) FROM webhook_outbox WHERE event_type='timeline.entry_created'").Scan(&inside); err != nil || inside != 1 {
		t.Fatal("event was not visible inside the transaction", inside, err)
	}
	if err = tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	var outside int
	if err = pool.QueryRow(ctx, "SELECT count(*) FROM webhook_outbox WHERE event_type='timeline.entry_created'").Scan(&outside); err != nil || outside != 0 {
		t.Fatal("rolled back event was committed", outside, err)
	}

	saved := call(devHandler, devSettings, "POST", "/api/sections/clients/records", adminCookie, map[string]any{"data": map[string]any{"name": "Mara", "email": "mara@example.test"}})
	expect(saved, 200)
	var record map[string]string
	json.Unmarshal(saved.Body.Bytes(), &record)
	var pendingBeforeDelivery int
	if err = pool.QueryRow(ctx, "SELECT count(*) FROM webhook_deliveries WHERE endpoint_id=$1 AND status='pending'", endpointID).Scan(&pendingBeforeDelivery); err != nil || pendingBeforeDelivery < 1 {
		t.Fatal("outbox delivery was not durable before the worker ran", pendingBeforeDelivery, err)
	}
	expect(call(devHandler, devSettings, "PUT", "/api/sections/clients/records/"+record["id"], adminCookie, map[string]any{"data": map[string]any{"name": "Mara Santos", "email": "mara@example.test"}}), 200)
	expect(call(devHandler, devSettings, "POST", "/api/sections", adminCookie, map[string]string{"id": "vendors", "name": "Vendors"}), 201)
	expect(call(devHandler, devSettings, "POST", "/api/sections/vendors/fields", adminCookie, map[string]any{"id": "region", "label": "Region", "type": "text", "required": false}), 201)
	expect(call(devHandler, devSettings, "POST", "/api/sections/prospects/records", adminCookie, map[string]any{"data": map[string]any{"name": "Prospect"}}), 200)
	expect(call(devHandler, devSettings, "DELETE", "/api/sections/clients/records/"+record["id"], adminCookie, nil), 200)
	committed, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err = store.EmitTimelineEntry(ctx, committed, "user:admin", "clients", record["id"], store.TimelineEntry{ID: "entry-2", Kind: "comment", Body: "Hello @ada"}); err != nil {
		t.Fatal(err)
	}
	if err = committed.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	testEvent := call(devHandler, devSettings, "POST", "/api/webhooks/"+endpointID+"/test", adminCookie, nil)
	expect(testEvent, 202)
	var prospectDeliveries, disabledDeliveries int
	pool.QueryRow(ctx, "SELECT count(*) FROM webhook_deliveries WHERE endpoint_id=$1", filteredID).Scan(&prospectDeliveries)
	pool.QueryRow(ctx, "SELECT count(*) FROM webhook_deliveries WHERE endpoint_id=$1", disabledID).Scan(&disabledDeliveries)
	if prospectDeliveries != 1 || disabledDeliveries != 0 {
		t.Fatalf("subscription filter failed: prospects=%d disabled=%d", prospectDeliveries, disabledDeliveries)
	}

	successWorker := webhooks.NewWorker(repository, webhooks.Options{AllowLoopback: true, Timeout: 2 * time.Second, BaseBackoff: 200 * time.Millisecond, MaxBackoff: time.Second})
	for attempt := 0; attempt < 5; attempt++ {
		if err = successWorker.DeliverOnce(ctx); err != nil {
			t.Fatal(err)
		}
		var waiting int
		pool.QueryRow(ctx, "SELECT count(*) FROM webhook_deliveries WHERE endpoint_id=$1 AND status IN ('pending','inflight')", endpointID).Scan(&waiting)
		if waiting == 0 {
			break
		}
	}
	mu.Lock()
	received := append([]captured(nil), seen...)
	mu.Unlock()
	if len(received) < 5 {
		t.Fatalf("expected several deliveries, got %d", len(received))
	}
	var updated []byte
	for _, item := range received {
		if item.path != "/hook" {
			continue
		}
		if item.header.Get("Authorization") != headerValue {
			t.Fatal("custom header missing", item.header.Get("Authorization"))
		}
		if item.header.Get(webhooks.HeaderIdempotencyKey) == "" || item.header.Get(webhooks.HeaderEvent) == "" {
			t.Fatal("delivery headers missing")
		}
		stamp, _ := strconv.ParseInt(item.header.Get(webhooks.HeaderTimestamp), 10, 64)
		signature := item.header.Get(webhooks.HeaderSignature)
		if signature != webhooks.SignaturePrefix+webhooks.Sign(secret, stamp, item.body) {
			t.Fatal("signature mismatch")
		}
		if item.header.Get(webhooks.HeaderEvent) == "record.updated" {
			updated = item.body
		}
	}
	if updated == nil || !strings.Contains(string(updated), `"changed_field_ids":["name"]`) || !strings.Contains(string(updated), `"schema_version":1`) || !strings.Contains(string(updated), `"name":"Ada Admin"`) {
		t.Fatal("record.updated payload", string(updated))
	}
	var sawTimeline bool
	for _, item := range received {
		if item.header.Get(webhooks.HeaderEvent) == "timeline.entry_created" && strings.Contains(string(item.body), "Hello @ada") {
			sawTimeline = true
		}
	}
	if !sawTimeline {
		t.Fatal("timeline hook was not delivered")
	}
	logResponse := call(devHandler, devSettings, "GET", "/api/webhooks/"+endpointID+"/deliveries", adminCookie, nil)
	expect(logResponse, 200)
	if !strings.Contains(logResponse.Body.String(), `"status":"succeeded"`) || !strings.Contains(logResponse.Body.String(), `"attempt_count":1`) || !strings.Contains(logResponse.Body.String(), `"status_code":200`) {
		t.Fatal("delivery log", logResponse.Body.String())
	}
	var deliveries []map[string]any
	json.Unmarshal(logResponse.Body.Bytes(), &deliveries)
	for _, delivery := range deliveries {
		responseText, _ := delivery["response"].(string)
		if len([]rune(responseText)) > 500 {
			t.Fatal("response was not truncated", len(responseText))
		}
	}

	kept := call(devHandler, devSettings, "PUT", "/api/webhooks/"+endpointID, adminCookie, map[string]any{
		"url": receiver.URL + "/hook", "description": "Renamed", "event_types": []string{"record.created"},
		"section_id": "", "enabled": true, "custom_header_name": "Authorization",
	})
	expect(kept, 200)
	if strings.Contains(kept.Body.String(), secret) {
		t.Fatal("update response included the saved secret")
	}
	cleared := call(devHandler, devSettings, "PUT", "/api/webhooks/"+endpointID, adminCookie, map[string]any{
		"url": receiver.URL + "/hook", "description": "Renamed", "event_types": []string{"record.created"},
		"section_id": "", "enabled": true, "clear_signing_secret": true, "clear_custom_header": true,
	})
	expect(cleared, 400)

	mu.Lock()
	failUntil = 1
	seen = nil
	mu.Unlock()
	retry := call(devHandler, devSettings, "POST", "/api/webhooks", adminCookie, map[string]any{
		"url": receiver.URL + "/retry", "description": "Retry", "event_types": []string{"record.created"},
		"section_id": "clients", "enabled": true, "signing_secret": secret,
	})
	expect(retry, 201)
	retryID := jsonID(t, retry)
	expect(call(devHandler, devSettings, "POST", "/api/sections/clients/records", adminCookie, map[string]any{"data": map[string]any{"name": "Retry"}}), 200)
	retryWorker := webhooks.NewWorker(repository, webhooks.Options{AllowLoopback: true, Timeout: 2 * time.Second, MaxAttempts: 3, BaseBackoff: 200 * time.Millisecond, MaxBackoff: time.Second, FailureLimit: 10})
	if err = retryWorker.DeliverOnce(ctx); err != nil {
		t.Fatal(err)
	}
	var next time.Time
	var attempts int
	var status string
	var code *int
	if err = pool.QueryRow(ctx, "SELECT status,attempt_count,next_attempt_at,last_status_code FROM webhook_deliveries WHERE endpoint_id=$1", retryID).Scan(&status, &attempts, &next, &code); err != nil {
		t.Fatal(err)
	}
	if status != "pending" || attempts != 1 || code == nil || *code != 500 {
		t.Fatalf("retry state = %s attempts=%d code=%v", status, attempts, code)
	}
	wait := time.Until(next)
	if wait < 50*time.Millisecond || wait > 2*time.Second {
		t.Fatalf("backoff = %s", wait)
	}
	if _, err = pool.Exec(ctx, "UPDATE webhook_deliveries SET next_attempt_at=now() WHERE endpoint_id=$1", retryID); err != nil {
		t.Fatal(err)
	}
	if err = retryWorker.DeliverOnce(ctx); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, "SELECT status,attempt_count FROM webhook_deliveries WHERE endpoint_id=$1", retryID).Scan(&status, &attempts); err != nil || status != "succeeded" || attempts != 2 {
		t.Fatal("retry did not succeed", status, attempts, err)
	}
	mu.Lock()
	retrySeen := append([]captured(nil), seen...)
	mu.Unlock()
	var retryKeys []string
	for _, item := range retrySeen {
		if item.path == "/retry" {
			retryKeys = append(retryKeys, item.header.Get(webhooks.HeaderIdempotencyKey))
		}
	}
	if len(retryKeys) != 2 || retryKeys[0] == "" || retryKeys[0] != retryKeys[1] {
		t.Fatal("retries did not reuse the idempotency key", retryKeys)
	}

	failing := call(devHandler, devSettings, "POST", "/api/webhooks", adminCookie, map[string]any{
		"url": receiver.URL + "/always-fail", "description": "Fails", "event_types": []string{"record.created"},
		"section_id": "clients", "enabled": true, "custom_header_name": "X-Sender", "custom_header_value": headerValue,
	})
	expect(failing, 201)
	failingID := jsonID(t, failing)
	breaker := webhooks.NewWorker(repository, webhooks.Options{AllowLoopback: true, Timeout: 2 * time.Second, MaxAttempts: 1, BaseBackoff: 20 * time.Millisecond, FailureLimit: 2})
	for range 2 {
		expect(call(devHandler, devSettings, "POST", "/api/sections/clients/records", adminCookie, map[string]any{"data": map[string]any{"name": "Fail"}}), 200)
		if err = breaker.DeliverOnce(ctx); err != nil {
			t.Fatal(err)
		}
	}
	var enabled, autoDisabled bool
	var failures int
	if err = pool.QueryRow(ctx, "SELECT enabled,auto_disabled,consecutive_failures FROM webhook_endpoints WHERE id=$1", failingID).Scan(&enabled, &autoDisabled, &failures); err != nil {
		t.Fatal(err)
	}
	if enabled || !autoDisabled || failures < 2 {
		t.Fatalf("endpoint was not paused: enabled=%v auto=%v failures=%d", enabled, autoDisabled, failures)
	}
	var audits int
	if err = pool.QueryRow(ctx, "SELECT count(*) FROM audit WHERE action='webhook_auto_disable' AND record_id=$1 AND actor='system:webhooks'", failingID).Scan(&audits); err != nil || audits != 1 {
		t.Fatal("auto-disable was not audited", audits, err)
	}
	expect(call(devHandler, devSettings, "POST", "/api/sections/clients/records", adminCookie, map[string]any{"data": map[string]any{"name": "After pause"}}), 200)
	var afterPause int
	pool.QueryRow(ctx, "SELECT count(*) FROM webhook_deliveries WHERE endpoint_id=$1 AND event_type='record.created'", failingID).Scan(&afterPause)
	if afterPause != 2 {
		t.Fatal("paused endpoint still received events", afterPause)
	}
	expect(call(devHandler, devSettings, "POST", "/api/webhooks/"+failingID+"/enable", adminCookie, nil), 200)
	expect(call(devHandler, devSettings, "DELETE", "/api/webhooks/"+disabledID, adminCookie, nil), 200)

	if _, err = pool.Exec(ctx, `INSERT INTO webhook_endpoints(id,url,description,event_types,enabled,signing_secret)
VALUES('ssrf',$1,'',ARRAY['record.created'],true,'ssrf-secret-value')`, "https://"+strings.TrimPrefix(ssrfServer.URL, "http://")+"/hook"); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO webhook_outbox(id,event_type,payload) VALUES('evt_ssrf','record.created','{"schema_version":1}')`); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO webhook_deliveries(outbox_id,endpoint_id,event_type) VALUES('evt_ssrf','ssrf','record.created')`); err != nil {
		t.Fatal(err)
	}
	blocked := webhooks.NewWorker(repository, webhooks.Options{AllowLoopback: false, Timeout: time.Second, MaxAttempts: 1, FailureLimit: 5})
	if err = blocked.DeliverOnce(ctx); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	hits := ssrfHits
	mu.Unlock()
	if hits != 0 {
		t.Fatal("delivery connected to a loopback address")
	}
	var ssrfStatus, ssrfResponse string
	if err = pool.QueryRow(ctx, "SELECT status,last_response FROM webhook_deliveries WHERE endpoint_id='ssrf'").Scan(&ssrfStatus, &ssrfResponse); err != nil {
		t.Fatal(err)
	}
	if ssrfStatus != "failed" || !strings.Contains(ssrfResponse, "public https") {
		t.Fatal("delivery-time SSRF block missing", ssrfStatus, ssrfResponse)
	}

	var secretAudits int
	if err = pool.QueryRow(ctx, "SELECT count(*) FROM audit WHERE detail LIKE $1 OR detail LIKE $2", "%"+secret+"%", "%"+headerValue+"%").Scan(&secretAudits); err != nil || secretAudits != 0 {
		t.Fatal("audit stored a webhook secret", secretAudits, err)
	}
	var endpointAudits int
	if err = pool.QueryRow(ctx, "SELECT count(*) FROM audit WHERE action IN ('webhook_create','webhook_update') AND actor='user:admin'").Scan(&endpointAudits); err != nil || endpointAudits < 2 {
		t.Fatal("endpoint changes were not audited", endpointAudits, err)
	}

	t.Run("excluded_actors", func(t *testing.T) {
		filtered := call(devHandler, devSettings, "POST", "/api/webhooks", adminCookie, map[string]any{
			"url": receiver.URL + "/excluded", "description": "Actor filter", "event_types": []string{"record.created"},
			"section_id": "clients", "enabled": true, "signing_secret": secret,
			"excluded_actors": []map[string]string{{"kind": "user", "id": "admin"}},
		})
		expect(filtered, 201)
		filteredID := jsonID(t, filtered)
		var listed []map[string]any
		if err = json.Unmarshal(call(devHandler, devSettings, "GET", "/api/webhooks", adminCookie, nil).Body.Bytes(), &listed); err != nil {
			t.Fatal(err)
		}
		var found bool
		for _, item := range listed {
			if item["id"] == filteredID {
				actors, _ := item["excluded_actors"].([]any)
				if len(actors) != 1 {
					t.Fatalf("excluded_actors in list = %v", item["excluded_actors"])
				}
				found = true
			}
		}
		if !found {
			t.Fatal("filtered endpoint missing from list")
		}
		expect(call(devHandler, devSettings, "POST", "/api/sections/clients/records", memberCookie, map[string]any{"data": map[string]any{"name": "Member row"}}), 200)
		var memberDeliveries int
		if err = pool.QueryRow(ctx, "SELECT count(*) FROM webhook_deliveries WHERE endpoint_id=$1 AND event_type='record.created'", filteredID).Scan(&memberDeliveries); err != nil || memberDeliveries != 1 {
			t.Fatalf("member delivery count = %d (%v)", memberDeliveries, err)
		}
		expect(call(devHandler, devSettings, "POST", "/api/sections/clients/records", adminCookie, map[string]any{"data": map[string]any{"name": "Admin row"}}), 200)
		var adminDeliveries int
		var skipped int64
		if err = pool.QueryRow(ctx, "SELECT count(*) FROM webhook_deliveries WHERE endpoint_id=$1 AND event_type='record.created'", filteredID).Scan(&adminDeliveries); err != nil {
			t.Fatal(err)
		}
		if err = pool.QueryRow(ctx, "SELECT skipped_events FROM webhook_endpoints WHERE id=$1", filteredID).Scan(&skipped); err != nil {
			t.Fatal(err)
		}
		if adminDeliveries != 1 || skipped < 1 {
			t.Fatalf("admin should be skipped: deliveries=%d skipped=%d", adminDeliveries, skipped)
		}
		expect(call(devHandler, devSettings, "POST", "/api/webhooks/"+filteredID+"/test", adminCookie, nil), 202)
		var testDeliveries int
		if err = pool.QueryRow(ctx, "SELECT count(*) FROM webhook_deliveries WHERE endpoint_id=$1 AND event_type='webhook.test'", filteredID).Scan(&testDeliveries); err != nil || testDeliveries != 1 {
			t.Fatalf("test delivery count = %d (%v)", testDeliveries, err)
		}
		expect(call(devHandler, devSettings, "POST", "/api/webhooks", adminCookie, map[string]any{
			"url": receiver.URL + "/bad-ex", "event_types": []string{"record.created"}, "enabled": true, "signing_secret": secret,
			"excluded_actors": []map[string]string{{"kind": "agent", "id": "missing-agent"}},
		}), 400)

		mentionHook := call(devHandler, devSettings, "POST", "/api/webhooks", adminCookie, map[string]any{
			"url": receiver.URL + "/mentions", "event_types": []string{"comment.mentioned"}, "enabled": true, "signing_secret": secret,
			"excluded_actors": []map[string]string{{"kind": "user", "id": "member"}},
		})
		expect(mentionHook, 201)
		mentionID := jsonID(t, mentionHook)
		recordResp := call(devHandler, devSettings, "POST", "/api/sections/clients/records", adminCookie, map[string]any{"data": map[string]any{"name": "Mention target"}})
		expect(recordResp, 200)
		var mentionRecord map[string]string
		json.Unmarshal(recordResp.Body.Bytes(), &mentionRecord)
		comment := call(devHandler, devSettings, "POST", "/api/sections/clients/records/"+mentionRecord["id"]+"/comments", adminCookie, map[string]any{"body": "ping", "mentions": []string{"user:member"}})
		expect(comment, 201)
		var mentionDeliveries int
		if err = pool.QueryRow(ctx, "SELECT count(*) FROM webhook_deliveries WHERE endpoint_id=$1 AND event_type='comment.mentioned'", mentionID).Scan(&mentionDeliveries); err != nil || mentionDeliveries != 1 {
			t.Fatalf("mention to excluded principal should deliver: %d (%v)", mentionDeliveries, err)
		}
	})

	if _, err = pool.Exec(ctx, "UPDATE webhook_deliveries SET created_at=now()-interval '40 days', status='succeeded' WHERE endpoint_id=$1", endpointID); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, "UPDATE webhook_outbox SET created_at=now()-interval '40 days' WHERE id IN (SELECT outbox_id FROM webhook_deliveries WHERE endpoint_id=$1)", endpointID); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO webhook_outbox(id,event_type,payload,created_at) VALUES('evt_old_pending','record.created','{}',now()-interval '40 days')`); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO webhook_deliveries(outbox_id,endpoint_id,event_type,status,created_at) VALUES('evt_old_pending',$1,'record.created','pending',now()-interval '40 days')`, endpointID); err != nil {
		t.Fatal(err)
	}
	if err = repository.PruneWebhookHistory(ctx, time.Now().Add(-30*24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	var pruned, keptPending int
	pool.QueryRow(ctx, "SELECT count(*) FROM webhook_deliveries WHERE endpoint_id=$1 AND status='succeeded'", endpointID).Scan(&pruned)
	pool.QueryRow(ctx, "SELECT count(*) FROM webhook_deliveries WHERE outbox_id='evt_old_pending'").Scan(&keptPending)
	if pruned != 0 || keptPending != 1 {
		t.Fatalf("retention pruned=%d pending=%d", pruned, keptPending)
	}
}

func jsonID(t *testing.T, response *httptest.ResponseRecorder) string {
	t.Helper()
	var payload map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	identifier, _ := payload["id"].(string)
	if identifier == "" {
		t.Fatalf("missing id in %s", response.Body.String())
	}
	return identifier
}

func endpointBody(rawURL, secret, headerValue string) map[string]any {
	body := map[string]any{
		"url": rawURL, "description": "Primary", "enabled": true, "section_id": "",
		"event_types":    []string{"record.created", "record.updated", "record.deleted", "section.created", "field.created", "timeline.entry_created"},
		"signing_secret": secret,
	}
	if headerValue != "" {
		body["custom_header_name"] = "Authorization"
		body["custom_header_value"] = headerValue
	}
	return body
}
