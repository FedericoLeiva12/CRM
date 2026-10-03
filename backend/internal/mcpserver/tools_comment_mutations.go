package mcpserver

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"siracrm/internal/domain"
	"siracrm/internal/store"
)

type editCommentArguments struct {
	ID       string   `json:"id" jsonschema:"Comment ID, not record ID; must be this agent's own comment in this section"`
	Body     string   `json:"body" jsonschema:"Replacement plain-text body, 1-4000 characters"`
	Mentions []string `json:"mentions,omitempty" jsonschema:"Explicit mention ids in addition to @handles in the replacement body"`
}

type deleteCommentArguments struct {
	ID string `json:"id" jsonschema:"Comment ID, not record ID; must be this agent's own comment in this section"`
}

// Comment mutation is separate from record deletion: writers can maintain their own
// contributions, but ownership never grants access outside the authorized section.
func (handler *Handler) registerCommentMutationTools(server *mcp.Server, agentID string, section domain.Section) {
	mcp.AddTool(server, &mcp.Tool{Name: section.ID + "_comment_update", Description: "Edit this agent's own comment in " + section.Name + ". Replaces its body and mention set; newly mentioned principals are notified. Replies cannot be moved."}, func(ctx context.Context, _ *mcp.CallToolRequest, args editCommentArguments) (*mcp.CallToolResult, any, error) {
		if !handler.repository.CanAccess(ctx, agentID, section.ID, domain.WriteAccess) {
			return nil, nil, errPermissionDenied
		}
		result, err := handler.repository.EditSectionComment(ctx, "agent:"+agentID, section.ID, args.ID, args.Body, args.Mentions)
		if err != nil {
			return nil, nil, toolError(err)
		}
		return nil, commentToolPayload(result), nil
	})
	mcp.AddTool(server, &mcp.Tool{Name: section.ID + "_comment_delete", Description: "Delete this agent's own comment in " + section.Name + ". A parent with replies remains as a deleted placeholder."}, func(ctx context.Context, _ *mcp.CallToolRequest, args deleteCommentArguments) (*mcp.CallToolResult, any, error) {
		if !handler.repository.CanAccess(ctx, agentID, section.ID, domain.WriteAccess) {
			return nil, nil, errPermissionDenied
		}
		err := handler.repository.DeleteSectionComment(ctx, "agent:"+agentID, section.ID, args.ID)
		return nil, map[string]string{"id": args.ID}, toolError(err)
	})
}

func commentToolPayload(result store.CommentResult) map[string]any {
	unresolved := result.UnresolvedMentions
	if unresolved == nil {
		unresolved = []string{}
	}
	return map[string]any{"comment": result.Comment, "unresolved_mentions": unresolved}
}

func (handler *Handler) registerMentionCandidatesTool(server *mcp.Server, agentID string, section domain.Section) {
	mcp.AddTool(server, &mcp.Tool{Name: section.ID + "_mentionables", Description: "List names, handles and ids that can be mentioned in " + section.Name + ": team members and agents with read access. Does not expose credentials or grant settings."}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
		if !handler.repository.CanAccess(ctx, agentID, section.ID, domain.ReadAccess) {
			return nil, nil, errPermissionDenied
		}
		users, agents, err := handler.repository.MentionCandidates(ctx, section.ID)
		return nil, map[string]any{"users": users, "agents": agents}, toolError(err)
	})
}
