package httpapi

import (
	"context"
	"errors"
	"net/http"
	"siracrm/internal/store"
)

type contextKey int

const authenticatedUserKey contextKey = 0

func userID(request *http.Request) string {
	identifier, _ := request.Context().Value(authenticatedUserKey).(string)
	return identifier
}
func (server *Server) requireSession(next http.HandlerFunc) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		cookie, err := request.Cookie(sessionCookieName)
		if err != nil {
			writeError(writer, http.StatusUnauthorized, "Sign in required")
			return
		}
		identifier, err := server.authentication.SessionUser(request.Context(), cookie.Value)
		if errors.Is(err, store.ErrNotFound) {
			writeError(writer, http.StatusUnauthorized, "Sign in required")
			return
		}
		if err != nil {
			writeServiceError(writer, err)
			return
		}
		next(writer, request.WithContext(context.WithValue(request.Context(), authenticatedUserKey, identifier)))
	}
}
func (server *Server) checkOrigin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("X-Content-Type-Options", "nosniff")
		writer.Header().Set("Cache-Control", "no-store")
		origin := request.Header.Get("Origin")
		isMCP := request.URL.Path == "/mcp"
		// Native MCP clients omit Origin; browser clients must still match the configured origin.
		invalidOrigin := (isMCP && origin != "" && origin != server.settings.AppOrigin) || (!isMCP && request.Method != "GET" && origin != server.settings.AppOrigin)
		if invalidOrigin {
			writeError(writer, http.StatusForbidden, "Invalid origin")
			return
		}
		next.ServeHTTP(writer, request)
	})
}
