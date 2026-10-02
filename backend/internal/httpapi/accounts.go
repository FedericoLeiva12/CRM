package httpapi

import (
	"errors"
	"net/http"
	"siracrm/internal/auth"
)

const sessionCookieName = "sira_session"

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}
type passwordRequest struct {
	Current  string `json:"current"`
	Password string `json:"password"`
}

func (server *Server) sessionCookie(value string, maxAge int) *http.Cookie {
	return &http.Cookie{Name: sessionCookieName, Value: value, Path: "/", HttpOnly: true, Secure: server.settings.SecureCookies, SameSite: http.SameSiteStrictMode, MaxAge: maxAge}
}
func (server *Server) login(writer http.ResponseWriter, request *http.Request) {
	var input loginRequest
	if !decodeJSON(writer, request, &input) {
		return
	}
	sessionToken, err := server.authentication.Login(request.Context(), input.Email, input.Password)
	switch {
	case errors.Is(err, auth.ErrRateLimited):
		writeError(writer, http.StatusTooManyRequests, err.Error())
		return
	case errors.Is(err, auth.ErrInvalidCredentials):
		writeError(writer, http.StatusUnauthorized, err.Error())
		return
	case err != nil:
		writeServiceError(writer, err)
		return
	}
	http.SetCookie(writer, server.sessionCookie(sessionToken, int(auth.SessionLifetime.Seconds())))
	writeJSON(writer, http.StatusOK, map[string]bool{"ok": true})
}
func (server *Server) logout(writer http.ResponseWriter, request *http.Request) {
	cookie, _ := request.Cookie(sessionCookieName)
	if err := server.authentication.Logout(request.Context(), cookie.Value); err != nil {
		writeServiceError(writer, err)
		return
	}
	http.SetCookie(writer, server.sessionCookie("", -1))
	writeJSON(writer, http.StatusOK, map[string]bool{"ok": true})
}
func (server *Server) changePassword(writer http.ResponseWriter, request *http.Request) {
	var input passwordRequest
	if !decodeJSON(writer, request, &input) {
		return
	}
	if err := server.authentication.ChangePassword(request.Context(), userID(request), input.Current, input.Password); err != nil {
		writeServiceError(writer, err)
		return
	}
	http.SetCookie(writer, server.sessionCookie("", -1))
	writeJSON(writer, http.StatusOK, map[string]bool{"ok": true})
}
