package mcpserver

import (
	"context"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"siracrm/internal/domain"
	"siracrm/internal/store"
)

type saveArguments struct {
	ID   string         `json:"id,omitempty" jsonschema:"Existing record ID for complete replacement; omit to create"`
	Data map[string]any `json:"data" jsonschema:"Complete record values keyed by field ID"`
}
type deleteArguments struct {
	ID string `json:"id"`
}

func (handler *Handler) registerReadTool(server *mcp.Server, agentID string, section domain.Section) {
	mcp.AddTool(server, &mcp.Tool{Name: section.ID + "_list", Description: "List up to 500 most recently updated records in " + section.Name}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
		// Discovery is not authorization: recheck at invocation to reject revoked grants.
		if !handler.repository.CanAccess(ctx, agentID, section.ID, domain.ReadAccess) {
			return nil, nil, errPermissionDenied
		}
		records, err := handler.repository.ListRecords(ctx, section.ID)
		return nil, map[string]any{"records": records, "limit": store.RecordListLimit}, toolError(err)
	})
}
func (handler *Handler) registerWriteTools(server *mcp.Server, agentID string, section domain.Section) {
	mcp.AddTool(server, &mcp.Tool{Name: section.ID + "_save", Description: "Create or replace a record in " + section.Name + ". Use sections_schema for field definitions."}, func(ctx context.Context, _ *mcp.CallToolRequest, arguments saveArguments) (*mcp.CallToolResult, any, error) {
		if !handler.repository.CanAccess(ctx, agentID, section.ID, domain.WriteAccess) {
			return nil, nil, errPermissionDenied
		}
		identifier, err := handler.repository.SaveRecord(ctx, "agent:"+agentID, section.ID, arguments.ID, arguments.Data)
		return nil, map[string]string{"id": identifier}, toolError(err)
	})
	mcp.AddTool(server, &mcp.Tool{Name: section.ID + "_delete", Description: "Permanently delete a record from " + section.Name}, func(ctx context.Context, _ *mcp.CallToolRequest, arguments deleteArguments) (*mcp.CallToolResult, any, error) {
		if !handler.repository.CanAccess(ctx, agentID, section.ID, domain.WriteAccess) {
			return nil, nil, errPermissionDenied
		}
		err := handler.repository.DeleteRecord(ctx, "agent:"+agentID, section.ID, arguments.ID)
		return nil, map[string]string{"id": arguments.ID}, toolError(err)
	})
}
