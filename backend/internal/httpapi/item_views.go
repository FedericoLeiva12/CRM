package httpapi

import (
	"net/http"

	"siracrm/internal/domain"
	"siracrm/internal/itemviews"
)

func (server *Server) itemViewCatalog(writer http.ResponseWriter, request *http.Request) {
	writeJSON(writer, http.StatusOK, itemviews.Catalog())
}

func (server *Server) configureItemViews(writer http.ResponseWriter, request *http.Request) {
	var input struct {
		Views []domain.SectionView `json:"views"`
	}
	if !decodeJSON(writer, request, &input) {
		return
	}
	sectionID := request.PathValue("section")
	if err := server.repository.ReplaceSectionViews(request.Context(), "user:"+userID(request), sectionID, input.Views); err != nil {
		writeServiceError(writer, err)
		return
	}
	views, err := server.repository.ListSectionViews(request.Context(), sectionID)
	if err != nil {
		writeServiceError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, views)
}
