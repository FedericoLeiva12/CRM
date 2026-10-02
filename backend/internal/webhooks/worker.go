package webhooks

import (
	"bytes"
	"context"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"siracrm/internal/domain"
	"siracrm/internal/store"
)

const (
	DefaultMaxAttempts  = 5
	DefaultFailureLimit = 10
	DefaultTimeout      = 10 * time.Second
	DefaultBaseBackoff  = 5 * time.Second
	DefaultMaxBackoff   = 5 * time.Minute
	DefaultPollInterval = time.Second
	DefaultRetention    = 30 * 24 * time.Hour
	ResponseBodyLimit   = 500
)

type Options struct {
	AllowLoopback bool
	MaxAttempts   int
	BaseBackoff   time.Duration
	MaxBackoff    time.Duration
	FailureLimit  int
	Timeout       time.Duration
	Retention     time.Duration
	PollInterval  time.Duration
	Resolver      Resolver
}

type Worker struct {
	repository   *store.Repository
	policy       Policy
	client       *http.Client
	maxAttempts  int
	baseBackoff  time.Duration
	maxBackoff   time.Duration
	failureLimit int
	retention    time.Duration
	poll         time.Duration
	timeout      time.Duration
}

func NewWorker(repository *store.Repository, options Options) *Worker {
	if options.MaxAttempts <= 0 {
		options.MaxAttempts = DefaultMaxAttempts
	}
	if options.BaseBackoff <= 0 {
		options.BaseBackoff = DefaultBaseBackoff
	}
	if options.MaxBackoff <= 0 {
		options.MaxBackoff = DefaultMaxBackoff
	}
	if options.FailureLimit <= 0 {
		options.FailureLimit = DefaultFailureLimit
	}
	if options.Timeout <= 0 {
		options.Timeout = DefaultTimeout
	}
	if options.Retention <= 0 {
		options.Retention = DefaultRetention
	}
	if options.PollInterval <= 0 {
		options.PollInterval = DefaultPollInterval
	}
	policy := Policy{AllowLoopback: options.AllowLoopback, Resolver: options.Resolver}
	transport := &http.Transport{
		Proxy:                 func(*http.Request) (*url.URL, error) { return nil, nil },
		DialContext:           policy.DialContext,
		TLSHandshakeTimeout:   5 * time.Second,
		ResponseHeaderTimeout: options.Timeout,
		ExpectContinueTimeout: time.Second,
		DisableKeepAlives:     true,
	}
	return &Worker{
		repository:   repository,
		policy:       policy,
		client:       &http.Client{Timeout: options.Timeout, Transport: transport, CheckRedirect: refuseRedirect},
		maxAttempts:  options.MaxAttempts,
		baseBackoff:  options.BaseBackoff,
		maxBackoff:   options.MaxBackoff,
		failureLimit: options.FailureLimit,
		retention:    options.Retention,
		poll:         options.PollInterval,
		timeout:      options.Timeout,
	}
}

func refuseRedirect(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }

func Backoff(attempt int, base, max time.Duration) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	if base <= 0 {
		base = DefaultBaseBackoff
	}
	delay := base
	for step := 1; step < attempt; step++ {
		if delay > max/2 {
			return max
		}
		delay *= 2
	}
	if delay > max || delay <= 0 {
		return max
	}
	return delay
}

func (worker *Worker) Run(ctx context.Context) {
	worker.prune(ctx)
	ticker := time.NewTicker(worker.poll)
	defer ticker.Stop()
	pruneTicker := time.NewTicker(15 * time.Minute)
	defer pruneTicker.Stop()
	for {
		if err := worker.DeliverOnce(ctx); err != nil && ctx.Err() == nil {
			log.Printf("webhook delivery: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-pruneTicker.C:
			worker.prune(ctx)
		case <-ticker.C:
		}
	}
}

func (worker *Worker) prune(ctx context.Context) {
	if err := worker.repository.PruneWebhookHistory(ctx, time.Now().Add(-worker.retention)); err != nil && ctx.Err() == nil {
		log.Printf("webhook retention: %v", err)
	}
}

func (worker *Worker) DeliverOnce(ctx context.Context) error {
	if err := worker.repository.RequeueStaleDeliveries(ctx, time.Now().Add(-(worker.timeout + 15*time.Second)), worker.maxAttempts, worker.failureLimit); err != nil {
		return err
	}
	claimed, err := worker.repository.ClaimDueDeliveries(ctx, 20)
	if err != nil {
		return err
	}
	for _, delivery := range claimed {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		worker.deliver(ctx, delivery)
	}
	return nil
}

func (worker *Worker) deliver(ctx context.Context, delivery store.ClaimedDelivery) {
	requestCtx, cancel := context.WithTimeout(ctx, worker.timeout)
	started := time.Now()
	status, body, err := worker.post(requestCtx, delivery)
	latency := time.Since(started)
	cancel()
	if ctx.Err() != nil {
		return
	}
	outcome := store.DeliveryOutcome{
		DeliveryID:   delivery.ID,
		EndpointID:   delivery.EndpointID,
		EventType:    delivery.EventType,
		Attempt:      delivery.Attempt,
		MaxAttempts:  worker.maxAttempts,
		FailureLimit: worker.failureLimit,
		StatusCode:   status,
		Latency:      latency,
		Response:     body,
		NextAttempt:  time.Now().Add(Backoff(delivery.Attempt, worker.baseBackoff, worker.maxBackoff)),
	}
	if err != nil {
		outcome.Response = safeDeliveryError(err)
		outcome.StatusCode = 0
	} else if status >= 200 && status < 300 {
		outcome.Success = true
		outcome.NextAttempt = time.Now()
	}
	disabled, finishErr := worker.repository.FinishDelivery(ctx, outcome)
	if finishErr != nil && ctx.Err() == nil {
		log.Printf("webhook result endpoint=%s event=%s: %v", delivery.EndpointID, delivery.EventID, finishErr)
	}
	if disabled {
		log.Printf("webhook endpoint %s paused after consecutive failures", delivery.EndpointID)
	}
}

func (worker *Worker) post(ctx context.Context, delivery store.ClaimedDelivery) (int, string, error) {
	if err := ValidateDestination(ctx, delivery.URL, worker.policy); err != nil {
		return 0, "", err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, delivery.URL, bytes.NewReader(delivery.Body))
	if err != nil {
		return 0, "", err
	}
	timestamp := time.Now().Unix()
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("User-Agent", UserAgent)
	request.Header.Set(HeaderIdempotencyKey, delivery.EventID)
	request.Header.Set(HeaderTimestamp, strconv.FormatInt(timestamp, 10))
	request.Header.Set(HeaderEvent, delivery.EventType)
	if delivery.SigningSecret != "" {
		request.Header.Set(HeaderSignature, SignaturePrefix+Sign(delivery.SigningSecret, timestamp, delivery.Body))
	}
	if delivery.HeaderName != "" {
		request.Header.Set(delivery.HeaderName, delivery.HeaderValue)
	}
	response, err := worker.client.Do(request)
	if err != nil {
		return 0, "", err
	}
	defer response.Body.Close()
	limited, _ := io.ReadAll(io.LimitReader(response.Body, ResponseBodyLimit+32))
	text := string(limited)
	if text == "" {
		text = response.Status
	}
	return response.StatusCode, text, nil
}

func safeDeliveryError(err error) string {
	if domain.IsValidationError(err) {
		return err.Error()
	}
	return "Connection failed"
}
