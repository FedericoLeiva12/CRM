package httpapi

import (
	"net/http"
	"strings"

	"siracrm/internal/domain"
	"siracrm/internal/store"
	"siracrm/internal/webhooks"
)

type webhookBody struct {
	URL                string                   `json:"url"`
	Description        string                   `json:"description"`
	EventTypes         []string                 `json:"event_types"`
	SectionID          string                   `json:"section_id"`
	Enabled            bool                     `json:"enabled"`
	ExcludedActors     []domain.WebhookActorRef `json:"excluded_actors"`
	SigningSecret      string                   `json:"signing_secret"`
	ClearSigningSecret bool                     `json:"clear_signing_secret"`
	CustomHeaderName   string                   `json:"custom_header_name"`
	CustomHeaderValue  string                   `json:"custom_header_value"`
	ClearCustomHeader  bool                     `json:"clear_custom_header"`
}

type webhookView struct {
	store.WebhookEndpoint
	FailureLimit int `json:"failure_limit"`
	MaxAttempts  int `json:"max_attempts"`
}

type deliveryView struct {
	store.WebhookDelivery
	MaxAttempts int `json:"max_attempts"`
}

func (server *Server) listWebhooks(writer http.ResponseWriter, request *http.Request) {
	endpoints, err := server.repository.ListWebhookEndpoints(request.Context())
	if err != nil {
		writeServiceError(writer, err)
		return
	}
	views := make([]webhookView, 0, len(endpoints))
	for _, endpoint := range endpoints {
		views = append(views, server.webhookView(endpoint))
	}
	writeJSON(writer, http.StatusOK, views)
}

func (server *Server) createWebhook(writer http.ResponseWriter, request *http.Request) {
	body, ok := decodeWebhook(writer, request)
	if !ok {
		return
	}
	if err := webhooks.ValidateDestination(request.Context(), body.URL, server.destinations()); err != nil {
		writeServiceError(writer, err)
		return
	}
	endpoint, err := server.repository.CreateWebhookEndpoint(request.Context(), userID(request), webhookInput(body), server.settings.AllowLoopbackWebhooks)
	if err != nil {
		writeServiceError(writer, err)
		return
	}
	writeJSON(writer, http.StatusCreated, server.webhookView(endpoint))
}

func (server *Server) updateWebhook(writer http.ResponseWriter, request *http.Request) {
	body, ok := decodeWebhook(writer, request)
	if !ok {
		return
	}
	current, err := server.repository.WebhookEndpoint(request.Context(), request.PathValue("id"))
	if err != nil {
		writeServiceError(writer, err)
		return
	}
	if strings.TrimSpace(body.URL) != current.URL {
		if err = webhooks.ValidateDestination(request.Context(), body.URL, server.destinations()); err != nil {
			writeServiceError(writer, err)
			return
		}
	}
	endpoint, err := server.repository.UpdateWebhookEndpoint(request.Context(), userID(request), current.ID, webhookInput(body), server.settings.AllowLoopbackWebhooks)
	if err != nil {
		writeServiceError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, server.webhookView(endpoint))
}

func (server *Server) deleteWebhook(writer http.ResponseWriter, request *http.Request) {
	identifier := request.PathValue("id")
	if err := server.repository.DeleteWebhookEndpoint(request.Context(), userID(request), identifier); err != nil {
		writeServiceError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]string{"id": identifier})
}

func (server *Server) enableWebhook(writer http.ResponseWriter, request *http.Request) {
	server.setWebhookEnabled(writer, request, true)
}

func (server *Server) disableWebhook(writer http.ResponseWriter, request *http.Request) {
	server.setWebhookEnabled(writer, request, false)
}

func (server *Server) setWebhookEnabled(writer http.ResponseWriter, request *http.Request, enabled bool) {
	endpoint, err := server.repository.SetWebhookEnabled(request.Context(), userID(request), request.PathValue("id"), enabled)
	if err != nil {
		writeServiceError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, server.webhookView(endpoint))
}

func (server *Server) testWebhook(writer http.ResponseWriter, request *http.Request) {
	identifier, err := server.repository.EnqueueTestEvent(request.Context(), "user:"+userID(request), request.PathValue("id"))
	if err != nil {
		writeServiceError(writer, err)
		return
	}
	writeJSON(writer, http.StatusAccepted, map[string]string{"event_id": identifier})
}

func (server *Server) listWebhookDeliveries(writer http.ResponseWriter, request *http.Request) {
	deliveries, err := server.repository.ListWebhookDeliveries(request.Context(), request.PathValue("id"), 50)
	if err != nil {
		writeServiceError(writer, err)
		return
	}
	views := make([]deliveryView, 0, len(deliveries))
	for _, delivery := range deliveries {
		views = append(views, deliveryView{WebhookDelivery: delivery, MaxAttempts: webhooks.DefaultMaxAttempts})
	}
	writeJSON(writer, http.StatusOK, views)
}

func (server *Server) webhookView(endpoint store.WebhookEndpoint) webhookView {
	return webhookView{WebhookEndpoint: endpoint, FailureLimit: webhooks.DefaultFailureLimit, MaxAttempts: webhooks.DefaultMaxAttempts}
}

func (server *Server) destinations() webhooks.Policy {
	return webhooks.Policy{AllowLoopback: server.settings.AllowLoopbackWebhooks}
}

func decodeWebhook(writer http.ResponseWriter, request *http.Request) (webhookBody, bool) {
	var body webhookBody
	if !decodeJSON(writer, request, &body) {
		return webhookBody{}, false
	}
	return body, true
}

func webhookInput(body webhookBody) store.WebhookInput {
	return store.WebhookInput{
		URL: body.URL, Description: body.Description, EventTypes: body.EventTypes, SectionID: body.SectionID, Enabled: body.Enabled,
		ExcludedActors: body.ExcludedActors,
		SigningSecret:  body.SigningSecret, ClearSigningSecret: body.ClearSigningSecret,
		CustomHeaderName: body.CustomHeaderName, CustomHeaderValue: body.CustomHeaderValue, ClearCustomHeader: body.ClearCustomHeader,
	}
}
