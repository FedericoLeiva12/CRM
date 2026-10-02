package httpapi

import (
	"net/http"
	"siracrm/internal/domain"
)

type sectionRequest struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func (server *Server) listSections(writer http.ResponseWriter, request *http.Request) {
	sections, err := server.repository.ListSections(request.Context())
	if err != nil {
		writeServiceError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, sections)
}
func (server *Server) createSection(writer http.ResponseWriter, request *http.Request) {
	var input sectionRequest
	if !decodeJSON(writer, request, &input) {
		return
	}
	if err := server.repository.CreateSection(request.Context(), input.ID, input.Name); err != nil {
		writeServiceError(writer, err)
		return
	}
	writeJSON(writer, http.StatusCreated, input)
}
func (server *Server) addField(writer http.ResponseWriter, request *http.Request) {
	var field domain.Field
	if !decodeJSON(writer, request, &field) {
		return
	}
	if err := server.repository.AddField(request.Context(), request.PathValue("section"), field); err != nil {
		writeServiceError(writer, err)
		return
	}
	writeJSON(writer, http.StatusCreated, field)
}
