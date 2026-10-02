// Package mcpserver exposes section tools through authenticated Streamable HTTP.
package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"siracrm/internal/domain"
	"siracrm/internal/store"
)

var errPermissionDenied = errors.New("permission denied")

type Handler struct{ repository *store.Repository }

func New(repository *store.Repository) *Handler { return &Handler{repository: repository} }
func (handler *Handler) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	bearerToken, valid := strings.CutPrefix(request.Header.Get("Authorization"), "Bearer ")
	if !valid || bearerToken == "" {
		unauthorized(writer)
		return
	}
	agentID, err := handler.repository.AgentForToken(request.Context(), bearerToken)
	if errors.Is(err, store.ErrNotFound) {
		unauthorized(writer)
		return
	}
	if err != nil {
		log.Printf("MCP authentication failed: %v", err)
		http.Error(writer, "Unable to authenticate", http.StatusServiceUnavailable)
		return
	}
	sections, err := handler.repository.ListSections(request.Context())
	if err != nil {
		log.Printf("MCP discovery failed: %v", err)
		http.Error(writer, "Unable to discover sections", http.StatusServiceUnavailable)
		return
	}
	server := mcp.NewServer(&mcp.Implementation{Name: "sira-crm", Version: "1.0.0"}, nil)
	handler.registerSchemaTool(server, agentID)
	for _, section := range sections {
		if handler.repository.CanAccess(request.Context(), agentID, section.ID, domain.ReadAccess) {
			handler.registerReadTool(server, agentID, section)
		}
		if handler.repository.CanAccess(request.Context(), agentID, section.ID, domain.WriteAccess) {
			handler.registerWriteTools(server, agentID, section)
		}
	}
	// Rebuild discovery for every stateless request, so registry and grant changes are immediate.
	transport := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, &mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true})
	transport.ServeHTTP(writer, request)
}
func unauthorized(writer http.ResponseWriter) {
	writer.Header().Set("WWW-Authenticate", "Bearer")
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(http.StatusUnauthorized)
	if err := json.NewEncoder(writer).Encode(map[string]string{"error": "Valid agent token required"}); err != nil {
		log.Printf("encode MCP authentication response: %v", err)
	}
}

// Database details stay in server logs; validation messages can be safely shown to agents.
func toolError(err error) error {
	if err == nil || domain.IsValidationError(err) || errors.Is(err, store.ErrNotFound) || errors.Is(err, store.ErrConflict) {
		return err
	}
	log.Printf("MCP operation failed: %v", err)
	return errors.New("unable to complete this operation")
}
func (handler *Handler) registerSchemaTool(server *mcp.Server, agentID string) {
	mcp.AddTool(server, &mcp.Tool{Name: "sections_schema", Description: "Field definitions for sections this agent may access"}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
		sections, err := handler.repository.ListSections(ctx)
		if err != nil {
			return nil, nil, toolError(err)
		}
		accessible := []domain.Section{}
		for _, section := range sections {
			if handler.repository.CanAccess(ctx, agentID, section.ID, domain.ReadAccess) || handler.repository.CanAccess(ctx, agentID, section.ID, domain.WriteAccess) {
				accessible = append(accessible, section)
			}
		}
		return nil, map[string]any{"sections": accessible}, nil
	})
}
