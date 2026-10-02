package httpapi

import (
	"net/http"

	"siracrm/internal/domain"
	"siracrm/internal/store"
)

const defaultPageSize = 50

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

type queryRequest struct {
	Filters []domain.Filter `json:"filters"`
	Search  string          `json:"search"`
	Sort    *domain.Sort    `json:"sort"`
	Limit   int             `json:"limit"`
	Cursor  string          `json:"cursor"`
}

// queryRecords gives the browser the same filtered, sorted, cursor-paged query
// the MCP list tools use. A signed-in user can read every section, as in listRecords.
func (server *Server) queryRecords(writer http.ResponseWriter, request *http.Request) {
	var input queryRequest
	if !decodeJSON(writer, request, &input) {
		return
	}
	sectionID := request.PathValue("section")
	exists, err := server.repository.SectionExists(request.Context(), sectionID)
	if err != nil {
		writeServiceError(writer, err)
		return
	}
	if !exists {
		writeServiceError(writer, store.ErrNotFound)
		return
	}
	if input.Limit == 0 {
		input.Limit = defaultPageSize
	}
	page, err := server.repository.QueryRecords(request.Context(), sectionID, domain.ListQuery{
		Filters: input.Filters, Search: input.Search, Sort: input.Sort, Limit: input.Limit, Cursor: input.Cursor,
	})
	if err != nil {
		writeServiceError(writer, err)
		return
	}
	var nextCursor any
	if page.NextCursor != "" {
		nextCursor = page.NextCursor
	}
	writeJSON(writer, http.StatusOK, map[string]any{"records": page.Records, "limit": page.Limit, "next_cursor": nextCursor, "total": page.Total})
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
