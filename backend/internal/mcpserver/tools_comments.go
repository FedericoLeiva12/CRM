package mcpserver

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"siracrm/internal/domain"
)

type commentArguments struct {
	ID       string   `json:"id" jsonschema:"Record ID"`
	Body     string   `json:"body" jsonschema:"Plain-text comment, 1-4000 characters. Write @handle to mention a team member or agent."`
	ParentID string   `json:"parent_id,omitempty" jsonschema:"Comment ID to reply to. Replies go one level deep: the parent must be a top-level comment on this record."`
	Mentions []string `json:"mentions,omitempty" jsonschema:"Optional explicit mentions as user or agent ids, in addition to any @handle in the body. Accepts a bare id, user:<id>, or agent:<id>."`
}
type commentsArguments struct {
	ID     string `json:"id" jsonschema:"Record ID"`
	Limit  *int   `json:"limit,omitempty" jsonschema:"Top-level comments per page, 1-200. Default 50."`
	Cursor string `json:"cursor,omitempty" jsonschema:"next_cursor from the previous page"`
}
type mentionsListArguments struct {
	UnreadOnly bool `json:"unread_only,omitempty" jsonschema:"Return only mentions not yet marked read"`
	Limit      *int `json:"limit,omitempty" jsonschema:"Maximum mentions to return, 1-500. Default 100."`
}
type mentionsMarkReadArguments struct {
	IDs []string `json:"ids,omitempty" jsonschema:"Mention ids from mentions_list; omit when all is true"`
	All bool     `json:"all,omitempty" jsonschema:"Mark all of this agent's mentions as read"`
}

func (handler *Handler) registerCommentWriteTool(server *mcp.Server, agentID string, section domain.Section) {
	handler.registerCommentMutationTools(server, agentID, section)
	mcp.AddTool(server, &mcp.Tool{Name: section.ID + "_comment", Description: "Add a comment to a record in " + section.Name + ", or reply to one with parent_id. @handle in the body mentions a team member or an agent that can read this section; mentioned principals are notified. Returns the comment and unresolved_mentions (handles or ids that matched nobody or cannot read this section)."}, func(ctx context.Context, _ *mcp.CallToolRequest, arguments commentArguments) (*mcp.CallToolResult, any, error) {
		if !handler.repository.CanAccess(ctx, agentID, section.ID, domain.WriteAccess) {
			return nil, nil, errPermissionDenied
		}
		result, err := handler.repository.CreateComment(ctx, "agent:"+agentID, section.ID, arguments.ID, arguments.Body, arguments.ParentID, arguments.Mentions)
		if err != nil {
			return nil, nil, toolError(err)
		}
		return nil, commentToolPayload(result), nil
	})
}

func (handler *Handler) registerCommentReadTool(server *mcp.Server, agentID string, section domain.Section) {
	handler.registerMentionCandidatesTool(server, agentID, section)
	mcp.AddTool(server, &mcp.Tool{Name: section.ID + "_comments", Description: "List comments on a record in " + section.Name + ": top-level comments newest first, each with its replies oldest first. Pass next_cursor back as cursor for the next page."}, func(ctx context.Context, _ *mcp.CallToolRequest, arguments commentsArguments) (*mcp.CallToolResult, any, error) {
		if !handler.repository.CanAccess(ctx, agentID, section.ID, domain.ReadAccess) {
			return nil, nil, errPermissionDenied
		}
		limit := 0
		if arguments.Limit != nil {
			limit = *arguments.Limit
			if limit == 0 {
				limit = -1
			}
		}
		page, err := handler.repository.ListComments(ctx, section.ID, arguments.ID, limit, arguments.Cursor)
		if err != nil {
			return nil, nil, toolError(err)
		}
		var next any
		if page.NextCursor != "" {
			next = page.NextCursor
		}
		return nil, map[string]any{"comments": page.Comments, "next_cursor": next}, nil
	})
}

// Mentions belong to the calling agent, not to a section, so these tools are always registered.
func (handler *Handler) registerMentionTools(server *mcp.Server, agentID string) {
	mcp.AddTool(server, &mcp.Tool{Name: "mentions_list", Description: "List comments that mention this agent, newest first, with an unread_count. Only mentions in sections this agent can currently read are returned."}, func(ctx context.Context, _ *mcp.CallToolRequest, arguments mentionsListArguments) (*mcp.CallToolResult, any, error) {
		limit := 0
		if arguments.Limit != nil {
			limit = *arguments.Limit
			if limit == 0 {
				limit = -1
			}
		}
		mentions, unread, err := handler.repository.ListMentions(ctx, domain.PrincipalAgent, agentID, arguments.UnreadOnly, limit)
		if err != nil {
			return nil, nil, toolError(err)
		}
		return nil, map[string]any{"mentions": mentions, "unread_count": unread}, nil
	})
	mcp.AddTool(server, &mcp.Tool{Name: "mentions_mark_read", Description: "Mark this agent's own mentions as read by ids, or all=true. Ids that belong to someone else or are already read are ignored. Returns how many changed."}, func(ctx context.Context, _ *mcp.CallToolRequest, arguments mentionsMarkReadArguments) (*mcp.CallToolResult, any, error) {
		marked, err := handler.repository.MarkMentionsRead(ctx, domain.PrincipalAgent, agentID, arguments.IDs, arguments.All)
		if err != nil {
			return nil, nil, toolError(err)
		}
		return nil, map[string]int{"marked": marked}, nil
	})
}
