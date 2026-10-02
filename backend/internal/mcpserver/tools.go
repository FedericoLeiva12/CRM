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
type createSectionArguments struct {
	ID   string `json:"id" jsonschema:"Section identifier: a lowercase letter followed by lowercase letters, digits, or underscores"`
	Name string `json:"name" jsonschema:"Display name, 1–80 characters"`
}
type addFieldArguments struct {
	Section  string           `json:"section" jsonschema:"Section identifier"`
	ID       string           `json:"id" jsonschema:"Field identifier: a lowercase letter followed by lowercase letters, digits, or underscores"`
	Label    string           `json:"label" jsonschema:"Display label, 1–80 characters"`
	Type     domain.FieldType `json:"type" jsonschema:"One of text, email, number, date, boolean"`
	Required bool             `json:"required" jsonschema:"Rejected while the section already has records"`
}

func (handler *Handler) registerSchemaManagementTools(server *mcp.Server, agentID string) {
	mcp.AddTool(server, &mcp.Tool{Name: "sections_create", Description: "Create a section with a required name field. Does not grant read or write on the new section. Definitions cannot be renamed, deleted, or have their type changed."}, func(ctx context.Context, _ *mcp.CallToolRequest, arguments createSectionArguments) (*mcp.CallToolResult, any, error) {
		if !handler.repository.CanManageSchema(ctx, agentID) {
			return nil, nil, errPermissionDenied
		}
		err := handler.repository.CreateSection(ctx, "agent:"+agentID, arguments.ID, arguments.Name)
		return nil, map[string]string{"id": arguments.ID, "name": arguments.Name}, toolError(err)
	})
	mcp.AddTool(server, &mcp.Tool{Name: "fields_add", Description: "Add a field to a section using the same rules as the admin UI. New required fields are rejected while records exist."}, func(ctx context.Context, _ *mcp.CallToolRequest, arguments addFieldArguments) (*mcp.CallToolResult, any, error) {
		if !handler.repository.CanManageSchema(ctx, agentID) {
			return nil, nil, errPermissionDenied
		}
		field := domain.Field{ID: arguments.ID, Label: arguments.Label, Type: arguments.Type, Required: arguments.Required}
		err := handler.repository.AddField(ctx, "agent:"+agentID, arguments.Section, field)
		return nil, field, toolError(err)
	})
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
