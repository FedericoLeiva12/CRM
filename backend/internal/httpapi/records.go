package httpapi

import "net/http"

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
