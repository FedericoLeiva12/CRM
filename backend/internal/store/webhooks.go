package store

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"siracrm/internal/auth/token"
	"siracrm/internal/domain"
)

type WebhookEndpoint struct {
	ID                  string    `json:"id"`
	URL                 string    `json:"url"`
	Description         string    `json:"description"`
	EventTypes          []string  `json:"event_types"`
	SectionID           *string   `json:"section_id"`
	Enabled             bool      `json:"enabled"`
	AutoDisabled        bool      `json:"auto_disabled"`
	ConsecutiveFailures int       `json:"consecutive_failures"`
	SigningSecretSet    bool      `json:"signing_secret_set"`
	CustomHeaderName    string    `json:"custom_header_name"`
	CustomHeaderSet     bool      `json:"custom_header_set"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
}

type WebhookInput struct {
	URL                string
	Description        string
	EventTypes         []string
	SectionID          string
	Enabled            bool
	SigningSecret      string
	ClearSigningSecret bool
	CustomHeaderName   string
	CustomHeaderValue  string
	ClearCustomHeader  bool
}

type WebhookDelivery struct {
	ID            int64     `json:"id"`
	EventID       string    `json:"event_id"`
	EventType     string    `json:"event_type"`
	Status        string    `json:"status"`
	AttemptCount  int       `json:"attempt_count"`
	StatusCode    *int      `json:"status_code"`
	LatencyMS     *int      `json:"latency_ms"`
	Response      string    `json:"response"`
	NextAttemptAt time.Time `json:"next_attempt_at"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type ClaimedDelivery struct {
	ID            int64
	EventID       string
	EventType     string
	EndpointID    string
	Attempt       int
	URL           string
	Body          []byte
	SigningSecret string
	HeaderName    string
	HeaderValue   string
}

type DeliveryOutcome struct {
	DeliveryID   int64
	EndpointID   string
	EventType    string
	Attempt      int
	MaxAttempts  int
	FailureLimit int
	Success      bool
	StatusCode   int
	Latency      time.Duration
	Response     string
	NextAttempt  time.Time
}

type preparedWebhook struct {
	URL           string
	Description   string
	EventTypes    []string
	SectionID     string
	SigningSecret string
	HeaderName    string
	HeaderValue   string
}

type storedSecrets struct {
	secret string
	name   string
	value  string
}

func (repository *Repository) ListWebhookEndpoints(ctx context.Context) ([]WebhookEndpoint, error) {
	rows, err := repository.pool.Query(ctx, webhookSelect+" ORDER BY created_at,id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	endpoints := []WebhookEndpoint{}
	for rows.Next() {
		endpoint, err := scanWebhook(rows)
		if err != nil {
			return nil, err
		}
		endpoints = append(endpoints, endpoint)
	}
	return endpoints, rows.Err()
}

func (repository *Repository) WebhookEndpoint(ctx context.Context, id string) (WebhookEndpoint, error) {
	return loadWebhook(ctx, repository.pool, id)
}

func (repository *Repository) CreateWebhookEndpoint(ctx context.Context, actor string, input WebhookInput, allowLoopback bool) (WebhookEndpoint, error) {
	fields, err := prepareWebhook(input, allowLoopback, "", "", "")
	if err != nil {
		return WebhookEndpoint{}, err
	}
	transaction, err := repository.pool.Begin(ctx)
	if err != nil {
		return WebhookEndpoint{}, err
	}
	defer func() { _ = transaction.Rollback(ctx) }()
	if err = ensureSection(ctx, transaction, fields.SectionID); err != nil {
		return WebhookEndpoint{}, err
	}
	identifier := token.New()
	_, err = transaction.Exec(ctx, `INSERT INTO webhook_endpoints(id,url,description,event_types,section_id,enabled,signing_secret,custom_header_name,custom_header_value)
VALUES($1,$2,$3,$4,NULLIF($5,''),$6,NULLIF($7,''),NULLIF($8,''),NULLIF($9,''))`, identifier, fields.URL, fields.Description, fields.EventTypes, fields.SectionID, input.Enabled, fields.SigningSecret, fields.HeaderName, fields.HeaderValue)
	if err != nil {
		return WebhookEndpoint{}, err
	}
	if err = insertWebhookAudit(ctx, transaction, "user:"+actor, "webhook_create", identifier, fields.URL); err != nil {
		return WebhookEndpoint{}, err
	}
	endpoint, err := loadWebhook(ctx, transaction, identifier)
	if err != nil {
		return WebhookEndpoint{}, err
	}
	return endpoint, transaction.Commit(ctx)
}

func (repository *Repository) UpdateWebhookEndpoint(ctx context.Context, actor, id string, input WebhookInput, allowLoopback bool) (WebhookEndpoint, error) {
	transaction, err := repository.pool.Begin(ctx)
	if err != nil {
		return WebhookEndpoint{}, err
	}
	defer func() { _ = transaction.Rollback(ctx) }()
	current, err := loadSecrets(ctx, transaction, id)
	if err != nil {
		return WebhookEndpoint{}, err
	}
	fields, err := prepareWebhook(input, allowLoopback, current.secret, current.name, current.value)
	if err != nil {
		return WebhookEndpoint{}, err
	}
	if err = ensureSection(ctx, transaction, fields.SectionID); err != nil {
		return WebhookEndpoint{}, err
	}
	var wasEnabled bool
	if err = transaction.QueryRow(ctx, "SELECT enabled FROM webhook_endpoints WHERE id=$1", id).Scan(&wasEnabled); err != nil {
		return WebhookEndpoint{}, err
	}
	_, err = transaction.Exec(ctx, `UPDATE webhook_endpoints
SET url=$2, description=$3, event_types=$4, section_id=NULLIF($5,''), enabled=$6,
    signing_secret=NULLIF($7,''), custom_header_name=NULLIF($8,''), custom_header_value=NULLIF($9,''), updated_at=now()
WHERE id=$1`, id, fields.URL, fields.Description, fields.EventTypes, fields.SectionID, input.Enabled, fields.SigningSecret, fields.HeaderName, fields.HeaderValue)
	if err != nil {
		return WebhookEndpoint{}, err
	}
	if wasEnabled && !input.Enabled {
		if err = cancelPending(ctx, transaction, id, 0); err != nil {
			return WebhookEndpoint{}, err
		}
	}
	if !wasEnabled && input.Enabled {
		if _, err = transaction.Exec(ctx, "UPDATE webhook_endpoints SET auto_disabled=false, consecutive_failures=0, updated_at=now() WHERE id=$1", id); err != nil {
			return WebhookEndpoint{}, err
		}
	}
	if err = insertWebhookAudit(ctx, transaction, "user:"+actor, "webhook_update", id, fields.URL); err != nil {
		return WebhookEndpoint{}, err
	}
	endpoint, err := loadWebhook(ctx, transaction, id)
	if err != nil {
		return WebhookEndpoint{}, err
	}
	return endpoint, transaction.Commit(ctx)
}

func (repository *Repository) SetWebhookEnabled(ctx context.Context, actor, id string, enabled bool) (WebhookEndpoint, error) {
	transaction, err := repository.pool.Begin(ctx)
	if err != nil {
		return WebhookEndpoint{}, err
	}
	defer func() { _ = transaction.Rollback(ctx) }()
	var url string
	err = transaction.QueryRow(ctx, "SELECT url FROM webhook_endpoints WHERE id=$1 FOR UPDATE", id).Scan(&url)
	if err != nil {
		return WebhookEndpoint{}, classifyMissingRow(err)
	}
	action := "webhook_disable"
	if enabled {
		action = "webhook_enable"
		_, err = transaction.Exec(ctx, "UPDATE webhook_endpoints SET enabled=true, auto_disabled=false, consecutive_failures=0, updated_at=now() WHERE id=$1", id)
	} else {
		_, err = transaction.Exec(ctx, "UPDATE webhook_endpoints SET enabled=false, updated_at=now() WHERE id=$1", id)
		if err == nil {
			err = cancelPending(ctx, transaction, id, 0)
		}
	}
	if err != nil {
		return WebhookEndpoint{}, err
	}
	if err = insertWebhookAudit(ctx, transaction, "user:"+actor, action, id, url); err != nil {
		return WebhookEndpoint{}, err
	}
	endpoint, err := loadWebhook(ctx, transaction, id)
	if err != nil {
		return WebhookEndpoint{}, err
	}
	return endpoint, transaction.Commit(ctx)
}

func (repository *Repository) DeleteWebhookEndpoint(ctx context.Context, actor, id string) error {
	transaction, err := repository.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = transaction.Rollback(ctx) }()
	var url string
	err = transaction.QueryRow(ctx, "SELECT url FROM webhook_endpoints WHERE id=$1 FOR UPDATE", id).Scan(&url)
	if err != nil {
		return classifyMissingRow(err)
	}
	if _, err = transaction.Exec(ctx, "DELETE FROM webhook_endpoints WHERE id=$1", id); err != nil {
		return err
	}
	if err = insertWebhookAudit(ctx, transaction, "user:"+actor, "webhook_delete", id, url); err != nil {
		return err
	}
	return transaction.Commit(ctx)
}

func (repository *Repository) EnqueueTestEvent(ctx context.Context, actor, endpointID string) (string, error) {
	transaction, err := repository.pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer func() { _ = transaction.Rollback(ctx) }()
	identifier, err := emit(ctx, transaction, outboundEvent{
		Type:       domain.EventWebhookTest,
		Actor:      actor,
		EndpointID: endpointID,
		Data:       map[string]any{"message": "Test delivery"},
	})
	if err != nil {
		return "", err
	}
	return identifier, transaction.Commit(ctx)
}

func (repository *Repository) ListWebhookDeliveries(ctx context.Context, endpointID string, limit int) ([]WebhookDelivery, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	var exists bool
	if err := repository.pool.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM webhook_endpoints WHERE id=$1)", endpointID).Scan(&exists); err != nil {
		return nil, err
	}
	if !exists {
		return nil, ErrNotFound
	}
	rows, err := repository.pool.Query(ctx, `SELECT id,outbox_id,event_type,status,attempt_count,last_status_code,last_latency_ms,last_response,next_attempt_at,created_at,updated_at
FROM webhook_deliveries WHERE endpoint_id=$1 ORDER BY created_at DESC,id DESC LIMIT $2`, endpointID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	deliveries := []WebhookDelivery{}
	for rows.Next() {
		var delivery WebhookDelivery
		if err = rows.Scan(&delivery.ID, &delivery.EventID, &delivery.EventType, &delivery.Status, &delivery.AttemptCount, &delivery.StatusCode, &delivery.LatencyMS, &delivery.Response, &delivery.NextAttemptAt, &delivery.CreatedAt, &delivery.UpdatedAt); err != nil {
			return nil, err
		}
		deliveries = append(deliveries, delivery)
	}
	return deliveries, rows.Err()
}

func (repository *Repository) ClaimDueDeliveries(ctx context.Context, limit int) ([]ClaimedDelivery, error) {
	if limit <= 0 {
		limit = 20
	}
	transaction, err := repository.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = transaction.Rollback(ctx) }()
	rows, err := transaction.Query(ctx, `SELECT d.id,d.outbox_id,d.event_type,d.attempt_count,e.id,e.url,o.payload,coalesce(e.signing_secret,''),coalesce(e.custom_header_name,''),coalesce(e.custom_header_value,'')
FROM webhook_deliveries d
JOIN webhook_endpoints e ON e.id=d.endpoint_id
JOIN webhook_outbox o ON o.id=d.outbox_id
WHERE d.status='pending' AND d.next_attempt_at<=now() AND (e.enabled OR d.event_type=$1)
ORDER BY d.next_attempt_at,d.id
LIMIT $2
FOR UPDATE OF d SKIP LOCKED`, domain.EventWebhookTest, limit)
	if err != nil {
		return nil, err
	}
	claimed := []ClaimedDelivery{}
	ids := []int64{}
	for rows.Next() {
		var delivery ClaimedDelivery
		var payload string
		var attempts int
		if err = rows.Scan(&delivery.ID, &delivery.EventID, &delivery.EventType, &attempts, &delivery.EndpointID, &delivery.URL, &payload, &delivery.SigningSecret, &delivery.HeaderName, &delivery.HeaderValue); err != nil {
			rows.Close()
			return nil, err
		}
		delivery.Attempt = attempts + 1
		delivery.Body = []byte(payload)
		claimed = append(claimed, delivery)
		ids = append(ids, delivery.ID)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	if len(ids) == 0 {
		return nil, transaction.Commit(ctx)
	}
	if _, err = transaction.Exec(ctx, "UPDATE webhook_deliveries SET status='inflight', claimed_at=now(), updated_at=now(), attempt_count=attempt_count+1 WHERE id = ANY($1)", ids); err != nil {
		return nil, err
	}
	return claimed, transaction.Commit(ctx)
}

func (repository *Repository) FinishDelivery(ctx context.Context, outcome DeliveryOutcome) (bool, error) {
	transaction, err := repository.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer func() { _ = transaction.Rollback(ctx) }()
	status := "pending"
	if outcome.Success {
		status = "succeeded"
	} else if outcome.Attempt >= outcome.MaxAttempts {
		status = "failed"
	}
	var code any
	if outcome.StatusCode > 0 {
		code = outcome.StatusCode
	}
	latency := int(outcome.Latency / time.Millisecond)
	if latency < 0 {
		latency = 0
	}
	nextAttempt := outcome.NextAttempt
	if nextAttempt.IsZero() {
		nextAttempt = time.Now()
	}
	tag, err := transaction.Exec(ctx, `UPDATE webhook_deliveries
SET status=$2, next_attempt_at=$3, last_status_code=$4, last_latency_ms=$5, last_response=$6, updated_at=now()
WHERE id=$1 AND status='inflight'`, outcome.DeliveryID, status, nextAttempt, code, latency, clipText(outcome.Response, 500))
	if err != nil {
		return false, err
	}
	if tag.RowsAffected() == 0 {
		return false, transaction.Commit(ctx)
	}
	disabled := false
	if outcome.Success {
		_, err = transaction.Exec(ctx, "UPDATE webhook_endpoints SET consecutive_failures=0, updated_at=now() WHERE id=$1", outcome.EndpointID)
	} else if outcome.EventType != domain.EventWebhookTest {
		disabled, err = noteFailure(ctx, transaction, outcome.EndpointID, outcome.DeliveryID, outcome.FailureLimit)
	}
	if err != nil {
		return false, err
	}
	return disabled, transaction.Commit(ctx)
}

func (repository *Repository) RequeueStaleDeliveries(ctx context.Context, claimedBefore time.Time, maxAttempts, failureLimit int) error {
	transaction, err := repository.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = transaction.Rollback(ctx) }()
	rows, err := transaction.Query(ctx, "SELECT id,endpoint_id,event_type,attempt_count FROM webhook_deliveries WHERE status='inflight' AND claimed_at < $1 FOR UPDATE", claimedBefore)
	if err != nil {
		return err
	}
	type stale struct {
		endpoint, eventType string
		deliveryID          int64
		attempt             int
	}
	pending := []stale{}
	for rows.Next() {
		var row stale
		if err = rows.Scan(&row.deliveryID, &row.endpoint, &row.eventType, &row.attempt); err != nil {
			rows.Close()
			return err
		}
		pending = append(pending, row)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	for _, row := range pending {
		status := "pending"
		if row.attempt >= maxAttempts {
			status = "failed"
		}
		if _, err = transaction.Exec(ctx, `UPDATE webhook_deliveries
SET status=$2, next_attempt_at=now(),
    last_response=CASE WHEN last_response='' AND $2='failed' THEN 'Delivery interrupted before a response' ELSE last_response END,
    updated_at=now()
WHERE id=$1 AND status='inflight'`, row.deliveryID, status); err != nil {
			return err
		}
		if status == "failed" && row.eventType != domain.EventWebhookTest {
			if _, err = noteFailure(ctx, transaction, row.endpoint, 0, failureLimit); err != nil {
				return err
			}
		}
	}
	return transaction.Commit(ctx)
}

func (repository *Repository) PruneWebhookHistory(ctx context.Context, before time.Time) error {
	transaction, err := repository.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = transaction.Rollback(ctx) }()
	if _, err = transaction.Exec(ctx, "DELETE FROM webhook_deliveries WHERE status IN ('succeeded','failed') AND created_at < $1", before); err != nil {
		return err
	}
	if _, err = transaction.Exec(ctx, `DELETE FROM webhook_outbox o WHERE o.created_at < $1 AND NOT EXISTS (SELECT 1 FROM webhook_deliveries d WHERE d.outbox_id=o.id)`, before); err != nil {
		return err
	}
	return transaction.Commit(ctx)
}

func prepareWebhook(input WebhookInput, allowLoopback bool, currentSecret, currentName, currentValue string) (preparedWebhook, error) {
	rawURL := strings.TrimSpace(input.URL)
	if err := domain.ValidateWebhookURL(rawURL, allowLoopback); err != nil {
		return preparedWebhook{}, err
	}
	description := strings.TrimSpace(input.Description)
	if err := domain.ValidateWebhookDescription(description); err != nil {
		return preparedWebhook{}, err
	}
	events, err := domain.NormalizeEventTypes(input.EventTypes)
	if err != nil {
		return preparedWebhook{}, err
	}
	sectionID := strings.TrimSpace(input.SectionID)
	if sectionID != "" && !identifierPatternMatches(sectionID) {
		return preparedWebhook{}, domain.Invalid("Choose a section that exists")
	}
	secret := currentSecret
	if input.SigningSecret != "" {
		secret = input.SigningSecret
	} else if input.ClearSigningSecret {
		secret = ""
	}
	name, value := currentName, currentValue
	if input.CustomHeaderValue != "" {
		name = strings.TrimSpace(input.CustomHeaderName)
		value = input.CustomHeaderValue
	} else if input.ClearCustomHeader {
		name, value = "", ""
	} else if strings.TrimSpace(input.CustomHeaderName) != "" {
		name = strings.TrimSpace(input.CustomHeaderName)
	}
	if err = domain.ValidateSigningSecret(secret); err != nil {
		return preparedWebhook{}, err
	}
	if name != "" || value != "" {
		if err = domain.ValidateCustomHeader(name, value); err != nil {
			return preparedWebhook{}, err
		}
	}
	if secret == "" && value == "" {
		return preparedWebhook{}, domain.Invalid("Add a signing secret or a custom header")
	}
	return preparedWebhook{URL: rawURL, Description: description, EventTypes: events, SectionID: sectionID, SigningSecret: secret, HeaderName: name, HeaderValue: value}, nil
}

func identifierPatternMatches(value string) bool {
	return domain.ValidIdentifier(value)
}

func ensureSection(ctx context.Context, transaction pgx.Tx, sectionID string) error {
	if sectionID == "" {
		return nil
	}
	var exists bool
	if err := transaction.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM sections WHERE id=$1)", sectionID).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return domain.Invalid("Choose a section that exists")
	}
	return nil
}

func noteFailure(ctx context.Context, transaction pgx.Tx, endpointID string, currentDelivery int64, failureLimit int) (bool, error) {
	var failures int
	err := transaction.QueryRow(ctx, "UPDATE webhook_endpoints SET consecutive_failures=consecutive_failures+1, updated_at=now() WHERE id=$1 RETURNING consecutive_failures", endpointID).Scan(&failures)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if failureLimit <= 0 || failures < failureLimit {
		return false, nil
	}
	tag, err := transaction.Exec(ctx, "UPDATE webhook_endpoints SET enabled=false, auto_disabled=true, updated_at=now() WHERE id=$1 AND NOT auto_disabled", endpointID)
	if err != nil {
		return false, err
	}
	if tag.RowsAffected() == 0 {
		return false, nil
	}
	if err = cancelPending(ctx, transaction, endpointID, currentDelivery); err != nil {
		return false, err
	}
	if _, err = transaction.Exec(ctx, "UPDATE webhook_deliveries SET status='failed', updated_at=now() WHERE id=$1 AND status='pending'", currentDelivery); err != nil {
		return false, err
	}
	var endpointURL string
	if err = transaction.QueryRow(ctx, "SELECT url FROM webhook_endpoints WHERE id=$1", endpointID).Scan(&endpointURL); err != nil {
		return false, err
	}
	if err = insertWebhookAudit(ctx, transaction, "system:webhooks", "webhook_auto_disable", endpointID, endpointURL); err != nil {
		return false, err
	}
	return true, nil
}

func cancelPending(ctx context.Context, transaction pgx.Tx, endpointID string, except int64) error {
	_, err := transaction.Exec(ctx, "UPDATE webhook_deliveries SET status='failed', last_response='Endpoint disabled', updated_at=now() WHERE endpoint_id=$1 AND status='pending' AND id<>$2", endpointID, except)
	return err
}

func insertWebhookAudit(ctx context.Context, transaction pgx.Tx, actor, action, endpointID, detail string) error {
	_, err := transaction.Exec(ctx, "INSERT INTO audit(actor,action,record_id,detail) VALUES($1,$2,$3,$4)", actor, action, endpointID, clipText(detail, 300))
	return err
}

func loadSecrets(ctx context.Context, transaction pgx.Tx, id string) (storedSecrets, error) {
	var current storedSecrets
	var secret, name, value *string
	err := transaction.QueryRow(ctx, "SELECT signing_secret,custom_header_name,custom_header_value FROM webhook_endpoints WHERE id=$1 FOR UPDATE", id).Scan(&secret, &name, &value)
	if err != nil {
		return storedSecrets{}, classifyMissingRow(err)
	}
	current.secret = deref(secret)
	current.name = deref(name)
	current.value = deref(value)
	return current, nil
}

func deref(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

const webhookSelect = `SELECT id,url,description,event_types,section_id,enabled,auto_disabled,consecutive_failures,
(coalesce(signing_secret,'') <> ''), coalesce(custom_header_name,''), (coalesce(custom_header_value,'') <> ''), created_at, updated_at
FROM webhook_endpoints`

type webhookScanner interface {
	Scan(dest ...any) error
}

func loadWebhook(ctx context.Context, queryer interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, id string) (WebhookEndpoint, error) {
	endpoint, err := scanWebhook(queryer.QueryRow(ctx, webhookSelect+" WHERE id=$1", id))
	return endpoint, classifyMissingRow(err)
}

func scanWebhook(row webhookScanner) (WebhookEndpoint, error) {
	var endpoint WebhookEndpoint
	err := row.Scan(&endpoint.ID, &endpoint.URL, &endpoint.Description, &endpoint.EventTypes, &endpoint.SectionID, &endpoint.Enabled, &endpoint.AutoDisabled, &endpoint.ConsecutiveFailures, &endpoint.SigningSecretSet, &endpoint.CustomHeaderName, &endpoint.CustomHeaderSet, &endpoint.CreatedAt, &endpoint.UpdatedAt)
	if endpoint.EventTypes == nil {
		endpoint.EventTypes = []string{}
	}
	return endpoint, err
}
