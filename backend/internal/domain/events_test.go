package domain

import (
	"net"
	"slices"
	"testing"
)

func TestChangedFields(t *testing.T) {
	ids, values := ChangedFields(map[string]any{"name": "A", "email": "a@b.c"}, map[string]any{"name": "B", "email": "a@b.c"})
	if !slices.Equal(ids, []string{"name"}) || values["name"] != "B" || len(values) != 1 {
		t.Fatalf("update diff = %v %v", ids, values)
	}
	ids, values = ChangedFields(map[string]any{"name": "A", "notes": "x"}, map[string]any{"name": "A"})
	if !slices.Equal(ids, []string{"notes"}) || len(values) != 0 {
		t.Fatalf("removed field = %v %v", ids, values)
	}
	ids, _ = ChangedFields(map[string]any{"name": "A"}, map[string]any{"name": "A"})
	if len(ids) != 0 {
		t.Fatal("unchanged record reported a diff")
	}
}

func TestNormalizeEventTypes(t *testing.T) {
	types, err := NormalizeEventTypes([]string{" field.created ", "record.created", "record.created"})
	if err != nil || !slices.Equal(types, []string{EventFieldCreated, EventRecordCreated}) {
		t.Fatalf("normalized = %v %v", types, err)
	}
	if _, err = NormalizeEventTypes(nil); err == nil {
		t.Fatal("accepted an empty subscription")
	}
	if _, err = NormalizeEventTypes([]string{EventWebhookTest}); err == nil {
		t.Fatal("test events are not subscribable")
	}
}

func TestWebhookURLPolicy(t *testing.T) {
	allowed := []string{"https://example.com/hook", "https://1.1.1.1/hook"}
	for _, raw := range allowed {
		if err := ValidateWebhookURL(raw, false); err != nil {
			t.Errorf("rejected %s: %v", raw, err)
		}
	}
	blocked := []string{
		"http://example.com/hook",
		"https://user:pass@example.com/hook",
		"https://127.0.0.1/hook",
		"https://localhost/hook",
		"https://10.0.0.8/hook",
		"https://192.168.1.1/hook",
		"https://172.16.0.1/hook",
		"https://169.254.169.254/latest",
		"https://[::1]/hook",
		"https://[fe80::1]/hook",
		"https://[fc00::1]/hook",
		"https://100.64.0.1/hook",
		"https://metadata.google.internal/hook",
		"ftp://example.com/hook",
	}
	for _, raw := range blocked {
		if err := ValidateWebhookURL(raw, false); err == nil {
			t.Errorf("accepted blocked URL %s", raw)
		}
	}
	if err := ValidateWebhookURL("http://127.0.0.1:8080/hook", true); err != nil {
		t.Fatal(err)
	}
	if err := ValidateWebhookURL("http://localhost/hook", true); err != nil {
		t.Fatal(err)
	}
	if err := ValidateWebhookURL("http://example.com/hook", true); err == nil {
		t.Fatal("development still requires https off loopback")
	}
}

func TestBlockedIP(t *testing.T) {
	if !BlockedIP(net.ParseIP("8.8.8.8"), false) && BlockedIP(net.ParseIP("8.8.8.8"), false) {
		t.Fatal("logic error")
	}
	if BlockedIP(net.ParseIP("8.8.8.8"), false) {
		t.Fatal("public address blocked")
	}
	if !BlockedIP(net.ParseIP("::ffff:10.1.2.3"), false) {
		t.Fatal("mapped private address allowed")
	}
	if BlockedIP(net.ParseIP("127.0.0.1"), true) {
		t.Fatal("loopback refused in development")
	}
}

func TestCustomHeaderValidation(t *testing.T) {
	if err := ValidateCustomHeader("Authorization", "Bearer secret"); err != nil {
		t.Fatal(err)
	}
	if err := ValidateCustomHeader("X-CRM-Signature", "sha256=abc"); err == nil {
		t.Fatal("reserved header accepted")
	}
	if err := ValidateCustomHeader("Bad Name", "value"); err == nil {
		t.Fatal("invalid header name accepted")
	}
	if err := ValidateCustomHeader("X-Token", "line\nbreak"); err == nil {
		t.Fatal("header injection accepted")
	}
}
