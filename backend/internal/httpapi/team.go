package httpapi

import (
	"errors"
	"net/http"
	"siracrm/internal/auth"
	"siracrm/internal/store"
)

type inviteRequest struct {
	Email string `json:"email"`
	Role  string `json:"role"`
}
type acceptInviteRequest struct {
	Name     string `json:"name"`
	Password string `json:"password"`
}
type roleRequest struct {
	Role string `json:"role"`
}

func (server *Server) currentUser(writer http.ResponseWriter, request *http.Request) {
	user, err := server.repository.UserByID(request.Context(), userID(request))
	if err != nil {
		writeServiceError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, user)
}
func (server *Server) listUsers(writer http.ResponseWriter, request *http.Request) {
	users, err := server.repository.ListUsers(request.Context())
	if err != nil {
		writeServiceError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, users)
}
func (server *Server) listInvites(writer http.ResponseWriter, request *http.Request) {
	invites, err := server.repository.ListInvites(request.Context())
	if err != nil {
		writeServiceError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, invites)
}
func (server *Server) createInvite(writer http.ResponseWriter, request *http.Request) {
	var input inviteRequest
	if !decodeJSON(writer, request, &input) {
		return
	}
	email, err := server.authentication.NormalizeEmail(input.Email)
	if err != nil {
		writeServiceError(writer, err)
		return
	}
	created, err := server.repository.CreateInvite(request.Context(), userID(request), email, input.Role)
	if err != nil {
		writeServiceError(writer, err)
		return
	}
	writeJSON(writer, http.StatusCreated, map[string]any{
		"id": created.ID, "email": created.Email, "role": created.Role, "created_at": created.CreatedAt, "expires_at": created.ExpiresAt,
		"token": created.Token, "link": server.settings.AppOrigin + "/invite/" + created.Token,
	})
}
func (server *Server) revokeInvite(writer http.ResponseWriter, request *http.Request) {
	if err := server.repository.RevokeInvite(request.Context(), userID(request), request.PathValue("id")); err != nil {
		writeServiceError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]bool{"ok": true})
}
func (server *Server) previewInvite(writer http.ResponseWriter, request *http.Request) {
	invite, err := server.repository.InviteByToken(request.Context(), request.PathValue("token"))
	if err != nil {
		writeInviteError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, invite)
}
func (server *Server) acceptInvite(writer http.ResponseWriter, request *http.Request) {
	var input acceptInviteRequest
	if !decodeJSON(writer, request, &input) {
		return
	}
	sessionToken, err := server.authentication.AcceptInvite(request.Context(), request.PathValue("token"), input.Name, input.Password)
	if err != nil {
		writeInviteError(writer, err)
		return
	}
	http.SetCookie(writer, server.sessionCookie(sessionToken, int(auth.SessionLifetime.Seconds())))
	writeJSON(writer, http.StatusCreated, map[string]bool{"ok": true})
}
func (server *Server) setUserRole(writer http.ResponseWriter, request *http.Request) {
	var input roleRequest
	if !decodeJSON(writer, request, &input) {
		return
	}
	if err := server.repository.SetUserRole(request.Context(), userID(request), request.PathValue("id"), input.Role); err != nil {
		writeServiceError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]bool{"ok": true})
}
func (server *Server) removeUser(writer http.ResponseWriter, request *http.Request) {
	if err := server.repository.RemoveUser(request.Context(), userID(request), request.PathValue("id")); err != nil {
		writeServiceError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]bool{"ok": true})
}
func writeInviteError(writer http.ResponseWriter, err error) {
	if errors.Is(err, store.ErrNotFound) {
		writeError(writer, http.StatusNotFound, "This invitation is no longer valid")
		return
	}
	writeServiceError(writer, err)
}
