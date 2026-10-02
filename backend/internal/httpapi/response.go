package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"siracrm/internal/domain"
	"siracrm/internal/store"
)

const maxRequestBytes = 1 << 20

func writeJSON(writer http.ResponseWriter, status int, value any) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	if err := json.NewEncoder(writer).Encode(value); err != nil {
		log.Printf("encode HTTP response: %v", err)
	}
}
func writeError(writer http.ResponseWriter, status int, message string) {
	writeJSON(writer, status, map[string]string{"error": message})
}
func decodeJSON(writer http.ResponseWriter, request *http.Request, value any) bool {
	request.Body = http.MaxBytesReader(writer, request.Body, maxRequestBytes)
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		writeError(writer, http.StatusBadRequest, "Invalid request body")
		return false
	}
	// Reject multiple JSON values instead of silently ignoring a trailing document.
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		writeError(writer, http.StatusBadRequest, "Expected one JSON document")
		return false
	}
	return true
}
func writeServiceError(writer http.ResponseWriter, err error) {
	switch {
	case domain.IsValidationError(err):
		writeError(writer, http.StatusBadRequest, err.Error())
	case errors.Is(err, store.ErrNotFound):
		writeError(writer, http.StatusNotFound, err.Error())
	case errors.Is(err, store.ErrForbidden):
		writeError(writer, http.StatusForbidden, err.Error())
	case errors.Is(err, store.ErrConflict):
		writeError(writer, http.StatusConflict, err.Error())
	default:
		log.Printf("API operation failed: %v", err)
		writeError(writer, http.StatusInternalServerError, "Unable to complete this request")
	}
}
