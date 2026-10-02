package httpapi

import (
	"net/http"
	"strconv"

	"siracrm/internal/domain"
	"siracrm/internal/store"
)

type commentRequest struct {
	Body     string   `json:"body"`
	ParentID string   `json:"parent_id"`
	Mentions []string `json:"mentions"`
}
type markReadRequest struct {
	IDs []string `json:"ids"`
	All bool     `json:"all"`
}

func queryLimit(request *http.Request) (int, error) {
	value := request.URL.Query().Get("limit")
	if value == "" {
		return 0, nil
	}
	limit, err := strconv.Atoi(value)
	if err != nil {
		return 0, domain.Invalid("Limit must be a number")
	}
	return limit, nil
}

func commentResponse(result store.CommentResult) map[string]any {
	unresolved := result.UnresolvedMentions
	if unresolved == nil {
		unresolved = []string{}
	}
	return map[string]any{"comment": result.Comment, "unresolved_mentions": unresolved}
}

func (server *Server) listComments(writer http.ResponseWriter, request *http.Request) {
	limit, err := queryLimit(request)
	if err != nil {
		writeServiceError(writer, err)
		return
	}
	page, err := server.repository.ListComments(request.Context(), request.PathValue("section"), request.PathValue("id"), limit, request.URL.Query().Get("cursor"))
	if err != nil {
		writeServiceError(writer, err)
		return
	}
	var next any
	if page.NextCursor != "" {
		next = page.NextCursor
	}
	writeJSON(writer, http.StatusOK, map[string]any{"comments": page.Comments, "next_cursor": next})
}
func (server *Server) createComment(writer http.ResponseWriter, request *http.Request) {
	var input commentRequest
	if !decodeJSON(writer, request, &input) {
		return
	}
	result, err := server.repository.CreateComment(request.Context(), "user:"+userID(request), request.PathValue("section"), request.PathValue("id"), input.Body, input.ParentID, input.Mentions)
	if err != nil {
		writeServiceError(writer, err)
		return
	}
	writeJSON(writer, http.StatusCreated, commentResponse(result))
}
func (server *Server) editComment(writer http.ResponseWriter, request *http.Request) {
	var input commentRequest
	if !decodeJSON(writer, request, &input) {
		return
	}
	if input.ParentID != "" {
		writeError(writer, http.StatusBadRequest, "A reply cannot be moved to another comment")
		return
	}
	result, err := server.repository.EditComment(request.Context(), "user:"+userID(request), request.PathValue("id"), input.Body, input.Mentions)
	if err != nil {
		writeServiceError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, commentResponse(result))
}
func (server *Server) deleteComment(writer http.ResponseWriter, request *http.Request) {
	identifier := request.PathValue("id")
	if err := server.repository.DeleteComment(request.Context(), "user:"+userID(request), identifier); err != nil {
		writeServiceError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]string{"id": identifier})
}
func (server *Server) mentionCandidates(writer http.ResponseWriter, request *http.Request) {
	users, agents, err := server.repository.MentionCandidates(request.Context(), request.PathValue("section"))
	if err != nil {
		writeServiceError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{"users": users, "agents": agents})
}
func (server *Server) listMentions(writer http.ResponseWriter, request *http.Request) {
	limit, err := queryLimit(request)
	if err != nil {
		writeServiceError(writer, err)
		return
	}
	unreadOnly := request.URL.Query().Get("unread_only") == "true"
	mentions, unread, err := server.repository.ListMentions(request.Context(), domain.PrincipalUser, userID(request), unreadOnly, limit)
	if err != nil {
		writeServiceError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{"mentions": mentions, "unread_count": unread})
}
func (server *Server) markMentionsRead(writer http.ResponseWriter, request *http.Request) {
	var input markReadRequest
	if !decodeJSON(writer, request, &input) {
		return
	}
	marked, err := server.repository.MarkMentionsRead(request.Context(), domain.PrincipalUser, userID(request), input.IDs, input.All)
	if err != nil {
		writeServiceError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]int{"marked": marked})
}
