// Package httpapi adapts CRM services to the authenticated browser API.
package httpapi

import (
	"net/http"
	"siracrm/internal/auth"
	"siracrm/internal/config"
	"siracrm/internal/mcpserver"
	"siracrm/internal/store"
)

type Server struct {
	repository     *store.Repository
	authentication *auth.Service
	settings       config.Config
}

func New(repository *store.Repository, authentication *auth.Service, settings config.Config) *Server {
	return &Server{repository: repository, authentication: authentication, settings: settings}
}

// Handler is the single place where routes and their authentication requirements are declared.
func (server *Server) Handler() http.Handler {
	router := http.NewServeMux()
	router.HandleFunc("GET /healthz", server.health)
	router.HandleFunc("POST /api/login", server.login)
	router.HandleFunc("GET /api/invite/{token}", server.previewInvite)
	router.HandleFunc("POST /api/invite/{token}", server.acceptInvite)
	router.HandleFunc("POST /api/logout", server.requireSession(server.logout))
	router.HandleFunc("POST /api/password", server.requireSession(server.changePassword))
	router.HandleFunc("GET /api/me", server.requireSession(server.currentUser))
	router.HandleFunc("GET /api/sections", server.requireSession(server.listSections))
	router.HandleFunc("POST /api/sections", server.requireAdmin(server.createSection))
	router.HandleFunc("POST /api/sections/{section}/fields", server.requireAdmin(server.addField))
	router.HandleFunc("GET /api/sections/{section}/records", server.requireSession(server.listRecords))
	router.HandleFunc("GET /api/sections/{section}/records/{id}", server.requireSession(server.getRecord))
	router.HandleFunc("POST /api/sections/{section}/records", server.requireSession(server.saveRecord))
	router.HandleFunc("PUT /api/sections/{section}/records/{id}", server.requireSession(server.saveRecord))
	router.HandleFunc("DELETE /api/sections/{section}/records/{id}", server.requireSession(server.deleteRecord))
	router.HandleFunc("GET /api/agents", server.requireAdmin(server.listAgents))
	router.HandleFunc("POST /api/agents", server.requireAdmin(server.createAgent))
	router.HandleFunc("PUT /api/agents/{id}/permissions", server.requireAdmin(server.setPermissions))
	router.HandleFunc("DELETE /api/agents/{id}", server.requireAdmin(server.revokeAgent))
	router.HandleFunc("GET /api/users", server.requireAdmin(server.listUsers))
	router.HandleFunc("PUT /api/users/{id}/role", server.requireAdmin(server.setUserRole))
	router.HandleFunc("DELETE /api/users/{id}", server.requireAdmin(server.removeUser))
	router.HandleFunc("GET /api/invites", server.requireAdmin(server.listInvites))
	router.HandleFunc("POST /api/invites", server.requireAdmin(server.createInvite))
	router.HandleFunc("DELETE /api/invites/{id}", server.requireAdmin(server.revokeInvite))
	router.HandleFunc("GET /api/webhooks", server.requireAdmin(server.listWebhooks))
	router.HandleFunc("POST /api/webhooks", server.requireAdmin(server.createWebhook))
	router.HandleFunc("PUT /api/webhooks/{id}", server.requireAdmin(server.updateWebhook))
	router.HandleFunc("DELETE /api/webhooks/{id}", server.requireAdmin(server.deleteWebhook))
	router.HandleFunc("POST /api/webhooks/{id}/enable", server.requireAdmin(server.enableWebhook))
	router.HandleFunc("POST /api/webhooks/{id}/disable", server.requireAdmin(server.disableWebhook))
	router.HandleFunc("POST /api/webhooks/{id}/test", server.requireAdmin(server.testWebhook))
	router.HandleFunc("GET /api/webhooks/{id}/deliveries", server.requireAdmin(server.listWebhookDeliveries))
	router.Handle("/mcp", mcpserver.New(server.repository))
	return server.checkOrigin(router)
}
func (server *Server) health(writer http.ResponseWriter, request *http.Request) {
	if err := server.repository.Health(request.Context()); err != nil {
		writeError(writer, http.StatusServiceUnavailable, "Database unavailable")
		return
	}
	writeJSON(writer, http.StatusOK, map[string]bool{"ok": true})
}
