package mcpserver

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"siracrm/internal/domain"
	"siracrm/internal/itemviews"
)

type configureViewsArguments struct {
	Section string               `json:"section" jsonschema:"Section identifier"`
	Views   []domain.SectionView `json:"views" jsonschema:"Complete ordered view configuration, including info enabled with empty config; use item_view_types for supported ids and configuration fields"`
}

func (handler *Handler) registerItemViewCatalogTool(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{Name: "item_view_types", Description: "List supported item views and their non-secret configuration fields. Section settings are returned by sections_schema."}, func(_ context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
		return nil, map[string]any{"views": itemviews.Catalog()}, nil
	})
}

func (handler *Handler) registerItemViewManagementTool(server *mcp.Server, agentID string) {
	mcp.AddTool(server, &mcp.Tool{Name: "section_views_configure", Description: "Replace the item view configuration of a section. Requires manage_schema. Info must remain enabled and cannot be removed. Disabling/removing optional views preserves record history. Configuration cannot store credentials; this does not grant record access."}, func(ctx context.Context, _ *mcp.CallToolRequest, args configureViewsArguments) (*mcp.CallToolResult, any, error) {
		if !handler.repository.CanManageSchema(ctx, agentID) {
			return nil, nil, errPermissionDenied
		}
		if err := handler.repository.ReplaceSectionViews(ctx, "agent:"+agentID, args.Section, args.Views); err != nil {
			return nil, nil, toolError(err)
		}
		views, err := handler.repository.ListSectionViews(ctx, args.Section)
		return nil, map[string]any{"section": args.Section, "views": views}, toolError(err)
	})
}
