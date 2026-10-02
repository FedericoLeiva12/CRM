package httpapi

import (
	"net/http"

	"siracrm/internal/domain"
)

type recordRequest struct {
	Data map[string]any `json:"data"`
}

func (server *Server) listRecords(writer http.ResponseWriter, request *http.Request) {
	records, err := server.repository.ListRecords(request.Context(), request.PathValue("section"))
	if err != nil {
		writeServiceError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, records)
}
func (server *Server) getRecord(writer http.ResponseWriter, request *http.Request) {
	sectionID := request.PathValue("section")
	recordID := request.PathValue("id")
	record, err := server.repository.GetRecord(request.Context(), sectionID, recordID)
	if err != nil {
		writeServiceError(writer, err)
		return
	}
	links, err := server.repository.ListLinks(request.Context(), sectionID, recordID)
	if err != nil {
		writeServiceError(writer, err)
		return
	}
	activities, err := server.repository.ListActivities(request.Context(), sectionID, recordID)
	if err != nil {
		writeServiceError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, struct {
		ID         string              `json:"id"`
		Data       map[string]any      `json:"data"`
		UpdatedAt  any                 `json:"updated_at"`
		Links      []domain.RecordLink `json:"links"`
		Activities []domain.Activity   `json:"activities"`
	}{ID: record.ID, Data: record.Data, UpdatedAt: record.UpdatedAt, Links: links, Activities: activities})
}
func (server *Server) saveRecord(writer http.ResponseWriter, request *http.Request) {
	var input recordRequest
	if !decodeJSON(writer, request, &input) {
		return
	}
	identifier, err := server.repository.SaveRecord(request.Context(), "user:"+userID(request), request.PathValue("section"), request.PathValue("id"), input.Data)
	if err != nil {
		writeServiceError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]string{"id": identifier})
}
func (server *Server) deleteRecord(writer http.ResponseWriter, request *http.Request) {
	identifier := request.PathValue("id")
	if err := server.repository.DeleteRecord(request.Context(), "user:"+userID(request), request.PathValue("section"), identifier); err != nil {
		writeServiceError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]string{"id": identifier})
}
