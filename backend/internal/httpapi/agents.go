package httpapi

import (
	"net/http"
	"siracrm/internal/domain"
)

type agentRequest struct {
	Name string `json:"name"`
}
type permissionsRequest struct {
	Permissions  []domain.Permission `json:"permissions"`
	ManageSchema bool                `json:"manage_schema"`
}

func (server *Server) listAgents(writer http.ResponseWriter, request *http.Request) {
	agents, err := server.repository.ListAgents(request.Context())
	if err != nil {
		writeServiceError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, agents)
}
func (server *Server) createAgent(writer http.ResponseWriter, request *http.Request) {
	var input agentRequest
	if !decodeJSON(writer, request, &input) {
		return
	}
	identifier, bearerToken, err := server.repository.CreateAgent(request.Context(), input.Name)
	if err != nil {
		writeServiceError(writer, err)
		return
	}
	writeJSON(writer, http.StatusCreated, map[string]string{"id": identifier, "token": bearerToken})
}
func (server *Server) revokeAgent(writer http.ResponseWriter, request *http.Request) {
	if err := server.repository.RevokeAgent(request.Context(), request.PathValue("id")); err != nil {
		writeServiceError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]bool{"ok": true})
}
func (server *Server) setPermissions(writer http.ResponseWriter, request *http.Request) {
	var input permissionsRequest
	if !decodeJSON(writer, request, &input) {
		return
	}
	if err := server.repository.SetPermissions(request.Context(), request.PathValue("id"), input.Permissions, input.ManageSchema); err != nil {
		writeServiceError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]bool{"ok": true})
}
