package domain

import (
	"net"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"
)

var headerNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9-]{0,63}$`)

var reservedHeaders = map[string]struct{}{
	"host": {}, "content-length": {}, "content-type": {}, "transfer-encoding": {}, "connection": {},
	"keep-alive": {}, "upgrade": {}, "trailer": {}, "te": {}, "proxy-authorization": {}, "proxy-authenticate": {},
	"cookie": {}, "set-cookie": {}, "forwarded": {}, "x-forwarded-for": {}, "x-forwarded-host": {}, "x-forwarded-proto": {},
	"x-crm-signature": {}, "x-crm-timestamp": {}, "x-crm-idempotency-key": {}, "x-crm-event": {},
}

const EventSchemaVersion = 1

const (
	EventRecordCreated        = "record.created"
	EventRecordUpdated        = "record.updated"
	EventRecordDeleted        = "record.deleted"
	EventSectionCreated       = "section.created"
	EventFieldCreated         = "field.created"
	EventTimelineEntryCreated = "timeline.entry_created"
	EventCommentMentioned     = "comment.mentioned"
	EventWebhookTest          = "webhook.test"
)

// EventActor is a user, an agent, or the system process that produced an event.
type EventActor struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
	Name string `json:"name"`
}

// EventSection is present when the event belongs to a section.
type EventSection struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// EventEnvelope is schema version 1 of the outbound webhook body.
// Data is always a JSON object. record_id and section are null when they do not apply.
// record.updated data includes changed_field_ids and the new values of those fields.
// A field that was cleared is listed in changed_field_ids and omitted from fields.
type EventEnvelope struct {
	SchemaVersion int            `json:"schema_version"`
	ID            string         `json:"id"`
	Type          string         `json:"type"`
	OccurredAt    time.Time      `json:"occurred_at"`
	Actor         EventActor     `json:"actor"`
	Section       *EventSection  `json:"section"`
	RecordID      *string        `json:"record_id"`
	Data          map[string]any `json:"data"`
}

func SubscribableEvents() []string {
	return []string{
		EventRecordCreated,
		EventRecordUpdated,
		EventRecordDeleted,
		EventSectionCreated,
		EventFieldCreated,
		EventTimelineEntryCreated,
		EventCommentMentioned,
	}
}

func Subscribable(eventType string) bool {
	for _, known := range SubscribableEvents() {
		if eventType == known {
			return true
		}
	}
	return false
}

func NormalizeEventTypes(types []string) ([]string, error) {
	if len(types) == 0 {
		return nil, Invalid("Choose at least one event")
	}
	seen := make(map[string]struct{}, len(types))
	clean := make([]string, 0, len(types))
	for _, eventType := range types {
		eventType = strings.TrimSpace(eventType)
		if !Subscribable(eventType) {
			return nil, Invalid("Choose a supported event type")
		}
		if _, ok := seen[eventType]; ok {
			continue
		}
		seen[eventType] = struct{}{}
		clean = append(clean, eventType)
	}
	sort.Strings(clean)
	return clean, nil
}

func ValidateWebhookDescription(description string) error {
	if len([]rune(description)) > 200 {
		return Invalid("Description must be 200 characters or fewer")
	}
	return nil
}

func ValidateSigningSecret(secret string) error {
	if secret == "" {
		return nil
	}
	if strings.ContainsAny(secret, "\r\n\x00") || len(secret) < 8 || len(secret) > 256 {
		return Invalid("Signing secret must be 8–256 characters without line breaks")
	}
	return nil
}

func ValidateCustomHeader(name, value string) error {
	if name == "" {
		return Invalid("Enter a name for the custom header")
	}
	if value == "" {
		return Invalid("Enter a value for the custom header")
	}
	if !headerNamePattern.MatchString(name) {
		return Invalid("Header name must use letters, digits, and hyphens")
	}
	if _, reserved := reservedHeaders[strings.ToLower(name)]; reserved {
		return Invalid("Choose a different header name")
	}
	if len(value) > 512 || strings.ContainsAny(value, "\r\n\x00") {
		return Invalid("Header value must be 512 characters or fewer without line breaks")
	}
	for _, char := range value {
		if unicode.IsControl(char) {
			return Invalid("Header value must be 512 characters or fewer without line breaks")
		}
	}
	return nil
}

// ValidateWebhookURL checks the URL shape and rejects private, loopback, and link-local literals.
// Hostnames are resolved separately at save and again at delivery.
func ValidateWebhookURL(raw string, allowLoopback bool) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return Invalid("Enter a webhook URL")
	}
	if len(raw) > 2000 {
		return Invalid("Webhook URL is too long")
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" || parsed.Hostname() == "" || parsed.Fragment != "" {
		return Invalid("Enter a valid webhook URL")
	}
	if parsed.User != nil {
		return Invalid("Webhook URL cannot include a username or password")
	}
	host := strings.ToLower(strings.TrimSuffix(parsed.Hostname(), "."))
	loopbackName := host == "localhost" || host == "127.0.0.1" || host == "::1"
	switch parsed.Scheme {
	case "https":
	case "http":
		if !allowLoopback || !loopbackName {
			return Invalid("Use an https URL")
		}
	default:
		return Invalid("Use an https URL")
	}
	if blockedWebhookHost(host, allowLoopback) {
		return Invalid("Webhook URLs must be public https addresses")
	}
	if ip := net.ParseIP(host); ip != nil && BlockedIP(ip, allowLoopback) {
		return Invalid("Webhook URLs must be public https addresses")
	}
	return nil
}

func blockedWebhookHost(host string, allowLoopback bool) bool {
	if host == "localhost" || host == "localhost.localdomain" {
		return !allowLoopback
	}
	if host == "metadata.google.internal" || strings.HasSuffix(host, ".metadata.google.internal") {
		return true
	}
	if strings.HasSuffix(host, ".local") || strings.HasSuffix(host, ".localhost") || strings.HasSuffix(host, ".internal") {
		return true
	}
	return false
}

// BlockedIP reports destinations that must not receive webhooks.
// Loopback is allowed only for local development. Private, link-local, multicast,
// unspecified, carrier-grade NAT, and reserved ranges are always blocked.
func BlockedIP(ip net.IP, allowLoopback bool) bool {
	if ip == nil {
		return true
	}
	if mapped := ip.To4(); mapped != nil {
		ip = mapped
	}
	if allowLoopback && ip.IsLoopback() {
		return false
	}
	if ip.IsUnspecified() || ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast() || ip.IsInterfaceLocalMulticast() {
		return true
	}
	v4 := ip.To4()
	if v4 == nil {
		return len(ip) == 16 && ip[0] == 0xfe && ip[1]&0xc0 == 0xc0
	}
	if v4[0] == 0 || v4[0] >= 240 {
		return true
	}
	if v4[0] == 100 && v4[1]&0xc0 == 64 {
		return true
	}
	if v4[0] == 198 && v4[1]&0xfe == 18 {
		return true
	}
	return false
}

// ChangedFields returns sorted field ids whose values differ and the new values that remain.
func ChangedFields(before, after map[string]any) ([]string, map[string]any) {
	seen := make(map[string]struct{}, len(before)+len(after))
	for key := range before {
		seen[key] = struct{}{}
	}
	for key := range after {
		seen[key] = struct{}{}
	}
	keys := make([]string, 0, len(seen))
	for key := range seen {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	ids := []string{}
	values := map[string]any{}
	for _, key := range keys {
		if sameJSONValue(before[key], after[key]) {
			continue
		}
		ids = append(ids, key)
		if _, present := after[key]; present {
			values[key] = after[key]
		}
	}
	return ids, values
}

func sameJSONValue(left, right any) bool {
	leftJSON, leftErr := jsonMarshal(left)
	rightJSON, rightErr := jsonMarshal(right)
	return leftErr == nil && rightErr == nil && leftJSON == rightJSON
}
