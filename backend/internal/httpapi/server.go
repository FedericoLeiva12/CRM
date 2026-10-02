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
	router.HandleFunc("POST /api/logout", server.requireSession(server.logout))
	router.HandleFunc("POST /api/password", server.requireSession(server.changePassword))
	router.HandleFunc("GET /api/sections", server.requireSession(server.listSections))
	router.HandleFunc("POST /api/sections", server.requireSession(server.createSection))
	router.HandleFunc("POST /api/sections/{section}/fields", server.requireSession(server.addField))
	router.HandleFunc("GET /api/sections/{section}/records", server.requireSession(server.listRecords))
	router.HandleFunc("POST /api/sections/{section}/records", server.requireSession(server.saveRecord))
	router.HandleFunc("PUT /api/sections/{section}/records/{id}", server.requireSession(server.saveRecord))
	router.HandleFunc("DELETE /api/sections/{section}/records/{id}", server.requireSession(server.deleteRecord))
	router.HandleFunc("GET /api/agents", server.requireSession(server.listAgents))
	router.HandleFunc("POST /api/agents", server.requireSession(server.createAgent))
	router.HandleFunc("PUT /api/agents/{id}/permissions", server.requireSession(server.setPermissions))
	router.HandleFunc("DELETE /api/agents/{id}", server.requireSession(server.revokeAgent))
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
