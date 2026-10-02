package mcpserver

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"siracrm/internal/domain"
	"siracrm/internal/store"
)

type recordIDArguments struct {
	ID string `json:"id"`
}
type updateArguments struct {
	ID   string         `json:"id"`
	Data map[string]any `json:"data" jsonschema:"Fields to change. Null clears a field. Omitted fields stay unchanged."`
}
type filterArgument struct {
	Field string `json:"field"`
	Op    string `json:"op" jsonschema:"eq, neq, contains, gt, gte, lt, lte, is_empty, or not_empty"`
	Value any    `json:"value,omitempty"`
}
type sortArgument struct {
	Field     string `json:"field" jsonschema:"Field id, or updated_at"`
	Direction string `json:"direction" jsonschema:"asc or desc"`
}
type listArguments struct {
	Filters []filterArgument `json:"filters,omitempty" jsonschema:"Combined with AND"`
	Sort    *sortArgument    `json:"sort,omitempty"`
	Limit   *int             `json:"limit,omitempty" jsonschema:"Maximum 500"`
	Cursor  string           `json:"cursor,omitempty"`
}
type activityArguments struct {
	ID      string `json:"id" jsonschema:"Record ID"`
	Type    string `json:"type" jsonschema:"Free-form activity type. A comment is type comment."`
	Date    string `json:"date" jsonschema:"YYYY-MM-DD or RFC3339"`
	Summary string `json:"summary"`
	Channel string `json:"channel,omitempty"`
	Ref     string `json:"ref,omitempty" jsonschema:"Optional external identifier"`
}
type convertArguments struct {
	ID        string            `json:"id"`
	Target    string            `json:"target" jsonschema:"Section that receives the new record"`
	Mapping   map[string]string `json:"mapping,omitempty" jsonschema:"Source field id to target field id, applied after same-id copy"`
	Overrides map[string]any    `json:"overrides,omitempty" jsonschema:"Values written on the new record after copying. Null clears a copied field."`
	Status    *string           `json:"status,omitempty" jsonschema:"Optional value written to the source status field"`
}

func (handler *Handler) registerReadTools(server *mcp.Server, agentID string, section domain.Section) {
	mcp.AddTool(server, &mcp.Tool{Name: section.ID + "_list", Description: "List records in " + section.Name + ". With no arguments, returns the latest 500 records. Optional filters, sort, limit, and cursor page the whole section."}, func(ctx context.Context, _ *mcp.CallToolRequest, arguments listArguments) (*mcp.CallToolResult, any, error) {
		// Discovery is not authorization: recheck at invocation to reject revoked grants.
		if !handler.repository.CanAccess(ctx, agentID, section.ID, domain.ReadAccess) {
			return nil, nil, errPermissionDenied
		}
		if arguments.Filters == nil && arguments.Sort == nil && arguments.Limit == nil && arguments.Cursor == "" {
			records, err := handler.repository.ListRecords(ctx, section.ID)
			return nil, map[string]any{"records": records, "limit": store.RecordListLimit}, toolError(err)
		}
		query := domain.ListQuery{Cursor: arguments.Cursor}
		if arguments.Sort != nil {
			query.Sort = &domain.Sort{Field: arguments.Sort.Field, Direction: arguments.Sort.Direction}
		}
		if arguments.Limit == nil {
			query.Limit = domain.MaxPageSize
		} else {
			query.Limit = *arguments.Limit
		}
		for _, filter := range arguments.Filters {
			query.Filters = append(query.Filters, domain.Filter{Field: filter.Field, Op: filter.Op, Value: filter.Value})
		}
		page, err := handler.repository.QueryRecords(ctx, section.ID, query)
		if err != nil {
			return nil, nil, toolError(err)
		}
		var nextCursor any
		if page.NextCursor != "" {
			nextCursor = page.NextCursor
		}
		return nil, map[string]any{"records": page.Records, "limit": page.Limit, "next_cursor": nextCursor, "total": page.Total}, nil
	})
	mcp.AddTool(server, &mcp.Tool{Name: section.ID + "_get", Description: "Get one record in " + section.Name + ", including links to records in other sections."}, func(ctx context.Context, _ *mcp.CallToolRequest, arguments recordIDArguments) (*mcp.CallToolResult, any, error) {
		if !handler.repository.CanAccess(ctx, agentID, section.ID, domain.ReadAccess) {
			return nil, nil, errPermissionDenied
		}
		record, links, err := handler.recordWithLinks(ctx, agentID, section.ID, arguments.ID)
		if err != nil {
			return nil, nil, toolError(err)
		}
		return nil, map[string]any{"id": record.ID, "data": record.Data, "updated_at": record.UpdatedAt, "links": links}, nil
	})
	mcp.AddTool(server, &mcp.Tool{Name: section.ID + "_activities", Description: "List timeline entries for a record in " + section.Name + ", newest first."}, func(ctx context.Context, _ *mcp.CallToolRequest, arguments recordIDArguments) (*mcp.CallToolResult, any, error) {
		if !handler.repository.CanAccess(ctx, agentID, section.ID, domain.ReadAccess) {
			return nil, nil, errPermissionDenied
		}
		activities, err := handler.repository.ListActivities(ctx, section.ID, arguments.ID)
		return nil, map[string]any{"activities": activities}, toolError(err)
	})
}

func (handler *Handler) registerRecordWriteTools(server *mcp.Server, agentID string, section domain.Section) {
	mcp.AddTool(server, &mcp.Tool{Name: section.ID + "_update", Description: "Change only the supplied fields on a record in " + section.Name + ". Null clears a field. Returns the full record."}, func(ctx context.Context, _ *mcp.CallToolRequest, arguments updateArguments) (*mcp.CallToolResult, any, error) {
		if !handler.repository.CanAccess(ctx, agentID, section.ID, domain.WriteAccess) {
			return nil, nil, errPermissionDenied
		}
		record, err := handler.repository.UpdateRecord(ctx, "agent:"+agentID, section.ID, arguments.ID, arguments.Data)
		return nil, record, toolError(err)
	})
	mcp.AddTool(server, &mcp.Tool{Name: section.ID + "_log_activity", Description: "Append a timeline entry to a record in " + section.Name + ". Entries remain when the record is edited."}, func(ctx context.Context, _ *mcp.CallToolRequest, arguments activityArguments) (*mcp.CallToolResult, any, error) {
		if !handler.repository.CanAccess(ctx, agentID, section.ID, domain.WriteAccess) {
			return nil, nil, errPermissionDenied
		}
		activity, err := handler.repository.LogActivity(ctx, "agent:"+agentID, section.ID, arguments.ID, arguments.Type, arguments.Date, arguments.Summary, arguments.Channel, arguments.Ref)
		return nil, activity, toolError(err)
	})
}

func (handler *Handler) registerConvertTool(server *mcp.Server, agentID string, section domain.Section) {
	mcp.AddTool(server, &mcp.Tool{Name: section.ID + "_convert", Description: "Create a record in another section from a record in " + section.Name + ". Matching field ids are copied. The same source cannot be converted into the same target section twice."}, func(ctx context.Context, _ *mcp.CallToolRequest, arguments convertArguments) (*mcp.CallToolResult, any, error) {
		if !handler.repository.CanAccess(ctx, agentID, section.ID, domain.ReadAccess) || !handler.repository.CanAccess(ctx, agentID, section.ID, domain.WriteAccess) || !handler.repository.CanAccess(ctx, agentID, arguments.Target, domain.WriteAccess) {
			return nil, nil, errPermissionDenied
		}
		source, target, err := handler.repository.ConvertRecord(ctx, "agent:"+agentID, section.ID, arguments.ID, arguments.Target, arguments.Mapping, arguments.Overrides, arguments.Status)
		if err != nil {
			return nil, nil, toolError(err)
		}
		sourceLinks, err := handler.visibleLinks(ctx, agentID, section.ID, source.ID)
		if err != nil {
			return nil, nil, toolError(err)
		}
		targetLinks, err := handler.visibleLinks(ctx, agentID, arguments.Target, target.ID)
		if err != nil {
			return nil, nil, toolError(err)
		}
		return nil, map[string]any{
			"source": map[string]any{"id": source.ID, "data": source.Data, "updated_at": source.UpdatedAt, "links": sourceLinks},
			"target": map[string]any{"id": target.ID, "data": target.Data, "updated_at": target.UpdatedAt, "links": targetLinks},
		}, nil
	})
}

func (handler *Handler) recordWithLinks(ctx context.Context, agentID, sectionID, recordID string) (domain.Record, []domain.RecordLink, error) {
	record, err := handler.repository.GetRecord(ctx, sectionID, recordID)
	if err != nil {
		return domain.Record{}, nil, err
	}
	links, err := handler.visibleLinks(ctx, agentID, sectionID, recordID)
	if err != nil {
		return domain.Record{}, nil, err
	}
	return record, links, nil
}

// Names from the other section are included only when this agent can read it.
func (handler *Handler) visibleLinks(ctx context.Context, agentID, sectionID, recordID string) ([]domain.RecordLink, error) {
	links, err := handler.repository.ListLinks(ctx, sectionID, recordID)
	if err != nil {
		return nil, err
	}
	visible := make([]domain.RecordLink, 0, len(links))
	for _, link := range links {
		if !handler.repository.CanAccess(ctx, agentID, link.SectionID, domain.ReadAccess) {
			link.Name = ""
		}
		visible = append(visible, link)
	}
	return visible, nil
}
