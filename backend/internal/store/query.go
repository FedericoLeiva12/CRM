package store

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"siracrm/internal/domain"
)

type cursorPayload struct {
	Field     string `json:"f"`
	Direction string `json:"d"`
	Null      bool   `json:"n,omitempty"`
	Value     any    `json:"v,omitempty"`
	ID        string `json:"id"`
}

type sortPlan struct {
	field     string
	direction string
	column    string
}

// QueryRecords returns one filtered page. Callers that omit every list argument
// keep using ListRecords so an empty call stays compatible.
func (repository *Repository) QueryRecords(ctx context.Context, sectionID string, query domain.ListQuery) (domain.ListPage, error) {
	fields, err := repository.ListFields(ctx, sectionID)
	if err != nil {
		return domain.ListPage{}, err
	}
	if query.Limit == 0 {
		query.Limit = domain.MaxPageSize
	}
	if err = domain.ValidateListQuery(fields, query); err != nil {
		return domain.ListPage{}, err
	}
	definitions := map[string]domain.Field{}
	for _, field := range fields {
		definitions[field.ID] = field
	}
	plan := sortPlan{field: "updated_at", direction: "desc", column: "updated_at"}
	if query.Sort != nil {
		plan.field = query.Sort.Field
		plan.direction = strings.ToLower(query.Sort.Direction)
		plan.column = "updated_at"
		if query.Sort.Field != "updated_at" {
			plan.column = valueColumn(definitions[query.Sort.Field].Type)
		}
	}
	terms, err := domain.SearchTerms(query.Search)
	if err != nil {
		return domain.ListPage{}, err
	}
	filterSQL, filterArgs, err := filterSQL(sectionID, query.Filters, terms, definitions)
	if err != nil {
		return domain.ListPage{}, err
	}
	transaction, err := repository.pool.Begin(ctx)
	if err != nil {
		return domain.ListPage{}, err
	}
	defer func() { _ = transaction.Rollback(ctx) }()
	var total int
	if err = transaction.QueryRow(ctx, "SELECT count(*) FROM records r WHERE r.section_id=$1"+filterSQL, filterArgs...).Scan(&total); err != nil {
		return domain.ListPage{}, err
	}
	pageArgs := append([]any{}, filterArgs...)
	join := ""
	orderExpr := "r.updated_at"
	if plan.column != "updated_at" {
		pageArgs = append(pageArgs, plan.field)
		join = fmt.Sprintf(" LEFT JOIN record_values sort_value ON sort_value.record_id = r.id AND sort_value.field_id = $%d", len(pageArgs))
		orderExpr = "sort_value." + plan.column
	}
	cursorSQL := ""
	if query.Cursor != "" {
		clause, clauseArgs, cursorErr := cursorClause(query.Cursor, plan, orderExpr, len(pageArgs))
		if cursorErr != nil {
			return domain.ListPage{}, cursorErr
		}
		cursorSQL = clause
		pageArgs = append(pageArgs, clauseArgs...)
	}
	direction := "ASC"
	if plan.direction == "desc" {
		direction = "DESC"
	}
	pageArgs = append(pageArgs, query.Limit+1)
	statement := fmt.Sprintf("SELECT r.id, r.data, r.updated_at FROM records r%s WHERE r.section_id=$1%s%s ORDER BY %s %s NULLS LAST, r.id ASC LIMIT $%d",
		join, filterSQL, cursorSQL, orderExpr, direction, len(pageArgs))
	rows, err := transaction.Query(ctx, statement, pageArgs...)
	if err != nil {
		return domain.ListPage{}, err
	}
	defer rows.Close()
	records := []domain.Record{}
	for rows.Next() {
		var record domain.Record
		if err = rows.Scan(&record.ID, &record.Data, &record.UpdatedAt); err != nil {
			return domain.ListPage{}, err
		}
		if record.Data == nil {
			record.Data = map[string]any{}
		}
		records = append(records, record)
	}
	if err = rows.Err(); err != nil {
		return domain.ListPage{}, err
	}
	if err = transaction.Commit(ctx); err != nil {
		return domain.ListPage{}, err
	}
	page := domain.ListPage{Records: records, Limit: query.Limit, Total: total}
	if len(records) > query.Limit {
		page.Records = records[:query.Limit]
		page.NextCursor, err = encodeCursor(page.Records[len(page.Records)-1], plan, definitions)
		if err != nil {
			return domain.ListPage{}, err
		}
	}
	return page, nil
}

func filterSQL(sectionID string, filters []domain.Filter, searchTerms []string, definitions map[string]domain.Field) (string, []any, error) {
	args := []any{sectionID}
	var builder strings.Builder
	// Every word must appear in some text or email value of the record.
	for _, term := range searchTerms {
		fmt.Fprintf(&builder, " AND EXISTS (SELECT 1 FROM record_values v WHERE v.record_id = r.id AND v.section_id = r.section_id AND v.text_value ILIKE %s ESCAPE '\\')", placeholder(&args, containsPattern(term)))
	}
	for _, filter := range filters {
		field := definitions[filter.Field]
		switch filter.Op {
		case domain.OpIsEmpty:
			fmt.Fprintf(&builder, " AND NOT EXISTS (SELECT 1 FROM record_values v WHERE v.record_id = r.id AND v.section_id = r.section_id AND v.field_id = %s)", placeholder(&args, field.ID))
		case domain.OpNotEmpty:
			fmt.Fprintf(&builder, " AND EXISTS (SELECT 1 FROM record_values v WHERE v.record_id = r.id AND v.section_id = r.section_id AND v.field_id = %s)", placeholder(&args, field.ID))
		case domain.OpEq, domain.OpNeq:
			column := valueColumn(field.Type)
			cast := ""
			if field.Type == domain.FieldDate {
				cast = "::date"
			}
			exists := fmt.Sprintf("EXISTS (SELECT 1 FROM record_values v WHERE v.record_id = r.id AND v.section_id = r.section_id AND v.field_id = %s AND v.%s = %s%s)", placeholder(&args, field.ID), column, placeholder(&args, filter.Value), cast)
			if filter.Op == domain.OpNeq {
				fmt.Fprintf(&builder, " AND NOT %s", exists)
			} else {
				fmt.Fprintf(&builder, " AND %s", exists)
			}
		case domain.OpIn:
			list, err := inList(field, filter.Value)
			if err != nil {
				return "", nil, err
			}
			arrayType := map[string]string{"text_value": "text[]", "number_value": "float8[]"}[valueColumn(field.Type)]
			fmt.Fprintf(&builder, " AND EXISTS (SELECT 1 FROM record_values v WHERE v.record_id = r.id AND v.section_id = r.section_id AND v.field_id = %s AND v.%s = ANY(%s::%s))", placeholder(&args, field.ID), valueColumn(field.Type), placeholder(&args, list), arrayType)
		case domain.OpContains:
			fmt.Fprintf(&builder, " AND EXISTS (SELECT 1 FROM record_values v WHERE v.record_id = r.id AND v.section_id = r.section_id AND v.field_id = %s AND v.text_value ILIKE %s ESCAPE '\\')", placeholder(&args, field.ID), placeholder(&args, containsPattern(filter.Value.(string))))
		case domain.OpGt, domain.OpGte, domain.OpLt, domain.OpLte:
			column := valueColumn(field.Type)
			operator := map[string]string{domain.OpGt: ">", domain.OpGte: ">=", domain.OpLt: "<", domain.OpLte: "<="}[filter.Op]
			cast := ""
			if field.Type == domain.FieldDate {
				cast = "::date"
			}
			fmt.Fprintf(&builder, " AND EXISTS (SELECT 1 FROM record_values v WHERE v.record_id = r.id AND v.section_id = r.section_id AND v.field_id = %s AND v.%s %s %s%s)", placeholder(&args, field.ID), column, operator, placeholder(&args, filter.Value), cast)
		default:
			return "", nil, domain.Invalid("Unsupported filter")
		}
	}
	return builder.String(), args, nil
}

func inList(field domain.Field, value any) (any, error) {
	items, ok := value.([]any)
	if !ok {
		return nil, domain.Invalid("Unsupported filter")
	}
	if field.Type == domain.FieldNumber {
		numbers := make([]float64, 0, len(items))
		for _, item := range items {
			number, isNumber := item.(float64)
			if !isNumber {
				return nil, domain.Invalid("Unsupported filter")
			}
			numbers = append(numbers, number)
		}
		return numbers, nil
	}
	texts := make([]string, 0, len(items))
	for _, item := range items {
		text, isText := item.(string)
		if !isText {
			return nil, domain.Invalid("Unsupported filter")
		}
		texts = append(texts, text)
	}
	return texts, nil
}

func cursorClause(cursor string, plan sortPlan, orderExpr string, argCount int) (string, []any, error) {
	payload, err := decodeCursor(cursor)
	if err != nil {
		return "", nil, err
	}
	if payload.Field != plan.field || payload.Direction != plan.direction || payload.ID == "" {
		return "", nil, domain.Invalid("Invalid cursor")
	}
	if payload.Null {
		if plan.column == "updated_at" {
			return "", nil, domain.Invalid("Invalid cursor")
		}
		return fmt.Sprintf(" AND %s IS NULL AND r.id > $%d", orderExpr, argCount+1), []any{payload.ID}, nil
	}
	value, err := cursorCompareValue(plan, payload.Value)
	if err != nil {
		return "", nil, err
	}
	comparison := ">"
	if plan.direction == "desc" {
		comparison = "<"
	}
	cast := ""
	if plan.column == "date_value" {
		cast = "::date"
	}
	clause := fmt.Sprintf(" AND (%s %s $%d%s OR (%s = $%d%s AND r.id > $%d) OR %s IS NULL)", orderExpr, comparison, argCount+1, cast, orderExpr, argCount+1, cast, argCount+2, orderExpr)
	return clause, []any{value, payload.ID}, nil
}

func cursorCompareValue(plan sortPlan, value any) (any, error) {
	if plan.column == "updated_at" {
		text, ok := value.(string)
		if !ok {
			return nil, domain.Invalid("Invalid cursor")
		}
		parsed, err := time.Parse(time.RFC3339Nano, text)
		if err != nil {
			return nil, domain.Invalid("Invalid cursor")
		}
		return parsed, nil
	}
	if plan.column == "date_value" {
		text, ok := value.(string)
		if !ok {
			return nil, domain.Invalid("Invalid cursor")
		}
		if _, err := time.Parse("2006-01-02", text); err != nil {
			return nil, domain.Invalid("Invalid cursor")
		}
	}
	return value, nil
}

func encodeCursor(record domain.Record, plan sortPlan, definitions map[string]domain.Field) (string, error) {
	payload := cursorPayload{Field: plan.field, Direction: plan.direction, ID: record.ID}
	if plan.column == "updated_at" {
		payload.Value = record.UpdatedAt.UTC().Truncate(time.Microsecond).Format(time.RFC3339Nano)
	} else if value, present := projectedValue(record.Data[plan.field], definitions[plan.field].Type); !present {
		payload.Null = true
	} else {
		payload.Value = value
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func projectedValue(value any, fieldType domain.FieldType) (any, bool) {
	if value == nil {
		return nil, false
	}
	switch fieldType {
	case domain.FieldText, domain.FieldEmail, domain.FieldDate:
		text, ok := value.(string)
		if !ok || text == "" {
			return nil, false
		}
		return text, true
	case domain.FieldNumber:
		number, ok := value.(float64)
		if !ok {
			return nil, false
		}
		return number, true
	case domain.FieldBoolean:
		flag, ok := value.(bool)
		if !ok {
			return nil, false
		}
		return flag, true
	default:
		return nil, false
	}
}

func decodeCursor(cursor string) (cursorPayload, error) {
	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return cursorPayload{}, domain.Invalid("Invalid cursor")
	}
	var payload cursorPayload
	if err = json.Unmarshal(raw, &payload); err != nil {
		return cursorPayload{}, domain.Invalid("Invalid cursor")
	}
	return payload, nil
}

func valueColumn(fieldType domain.FieldType) string {
	switch fieldType {
	case domain.FieldNumber:
		return "number_value"
	case domain.FieldDate:
		return "date_value"
	case domain.FieldBoolean:
		return "bool_value"
	default:
		return "text_value"
	}
}

func placeholder(args *[]any, value any) string {
	*args = append(*args, value)
	return fmt.Sprintf("$%d", len(*args))
}

func containsPattern(value string) string {
	var builder strings.Builder
	builder.WriteByte('%')
	for _, character := range value {
		if character == '%' || character == '_' || character == '\\' {
			builder.WriteByte('\\')
		}
		builder.WriteRune(character)
	}
	builder.WriteByte('%')
	return builder.String()
}
