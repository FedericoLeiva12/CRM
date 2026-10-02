package domain

import (
	"fmt"
	"math"
	"reflect"
	"strings"
	"time"
)

const (
	OpEq       = "eq"
	OpNeq      = "neq"
	OpContains = "contains"
	OpGt       = "gt"
	OpGte      = "gte"
	OpLt       = "lt"
	OpLte      = "lte"
	OpIsEmpty  = "is_empty"
	OpNotEmpty = "not_empty"
	OpIn       = "in"
)

// Limits keep one list request from building an unbounded SQL statement.
const (
	MaxInValues     = 100
	MaxSearchLength = 200
	MaxSearchTerms  = 8
)

// MergeRecord applies a partial update. A nil value clears that field.
func MergeRecord(existing, patch map[string]any) map[string]any {
	merged := CloneValues(existing)
	for key, value := range patch {
		if value == nil {
			delete(merged, key)
			continue
		}
		merged[key] = value
	}
	return merged
}

func CloneValues(values map[string]any) map[string]any {
	cloned := make(map[string]any, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}

func ValuesEqual(left, right map[string]any) bool {
	return reflect.DeepEqual(CloneValues(left), CloneValues(right))
}

// ConvertValues copies matching field ids, then applies an explicit mapping and overrides.
func ConvertValues(sourceFields, targetFields []Field, source map[string]any, mapping map[string]string, overrides map[string]any) (map[string]any, error) {
	sourceByID := fieldIndex(sourceFields)
	targetByID := fieldIndex(targetFields)
	values := map[string]any{}
	for _, field := range targetFields {
		value, present := source[field.ID]
		if !present || value == nil {
			continue
		}
		values[field.ID] = value
	}
	for from, to := range mapping {
		if _, known := sourceByID[from]; !known {
			return nil, Invalid(fmt.Sprintf("Unknown field: %s", from))
		}
		if _, known := targetByID[to]; !known {
			return nil, Invalid(fmt.Sprintf("Unknown field: %s", to))
		}
		value, present := source[from]
		if !present || value == nil {
			delete(values, to)
			continue
		}
		values[to] = value
	}
	for key, value := range overrides {
		if _, known := targetByID[key]; !known {
			return nil, Invalid(fmt.Sprintf("Unknown field: %s", key))
		}
		if value == nil {
			delete(values, key)
			continue
		}
		values[key] = value
	}
	if err := ValidateRecord(targetFields, values); err != nil {
		return nil, err
	}
	return values, nil
}

// ApplyStatus sets or clears the source status field when a conversion asks for it.
func ApplyStatus(fields []Field, values map[string]any, status *string) (map[string]any, error) {
	if status == nil {
		return values, nil
	}
	if _, known := fieldIndex(fields)["status"]; !known {
		return nil, Invalid("Unknown field: status")
	}
	next := CloneValues(values)
	if strings.TrimSpace(*status) == "" {
		delete(next, "status")
	} else {
		next["status"] = *status
	}
	if err := ValidateRecord(fields, next); err != nil {
		return nil, err
	}
	return next, nil
}

func ValidateListQuery(fields []Field, query ListQuery) error {
	if query.Limit < 1 || query.Limit > MaxPageSize {
		return Invalid(fmt.Sprintf("Limit must be between 1 and %d", MaxPageSize))
	}
	if _, err := SearchTerms(query.Search); err != nil {
		return err
	}
	definitions := fieldIndex(fields)
	for _, filter := range query.Filters {
		field, known := definitions[filter.Field]
		if !known {
			return Invalid(fmt.Sprintf("Unknown field: %s", filter.Field))
		}
		if err := validateFilter(field, filter); err != nil {
			return err
		}
	}
	if query.Sort != nil {
		direction := strings.ToLower(query.Sort.Direction)
		if direction != "asc" && direction != "desc" {
			return Invalid("Sort direction must be asc or desc")
		}
		if query.Sort.Field != "updated_at" {
			if _, known := definitions[query.Sort.Field]; !known {
				return Invalid(fmt.Sprintf("Unknown field: %s", query.Sort.Field))
			}
		}
	}
	return nil
}

func validateFilter(field Field, filter Filter) error {
	switch filter.Op {
	case OpIsEmpty, OpNotEmpty:
		return nil
	case OpEq, OpNeq:
		return validateFilterValue(field, filter.Value)
	case OpIn:
		return validateInValues(field, filter.Value)
	case OpContains:
		if field.Type != FieldText && field.Type != FieldEmail {
			return Invalid(fmt.Sprintf("%s does not support contains", field.Label))
		}
		return validateFilterValue(field, filter.Value)
	case OpGt, OpGte, OpLt, OpLte:
		if field.Type != FieldNumber && field.Type != FieldDate {
			return Invalid(fmt.Sprintf("%s does not support range filters", field.Label))
		}
		return validateFilterValue(field, filter.Value)
	default:
		return Invalid("Unsupported filter")
	}
}

// validateInValues accepts a bounded list of values of the field's own type.
// Booleans are excluded because a two-value set is just eq or is_empty.
func validateInValues(field Field, value any) error {
	if field.Type == FieldBoolean || field.Type == FieldDate {
		return Invalid(fmt.Sprintf("%s does not support is any of", field.Label))
	}
	values, isList := value.([]any)
	if !isList || len(values) == 0 || len(values) > MaxInValues {
		return Invalid(fmt.Sprintf("Choose between 1 and %d values for %s", MaxInValues, field.Label))
	}
	for _, item := range values {
		if err := validateFilterValue(field, item); err != nil {
			return err
		}
	}
	return nil
}

// SearchTerms splits a free-text search into the words that must each match.
func SearchTerms(search string) ([]string, error) {
	search = strings.TrimSpace(search)
	if len(search) > MaxSearchLength {
		return nil, Invalid(fmt.Sprintf("Search must be at most %d characters", MaxSearchLength))
	}
	terms := strings.Fields(search)
	if len(terms) > MaxSearchTerms {
		return nil, Invalid(fmt.Sprintf("Search accepts at most %d words", MaxSearchTerms))
	}
	return terms, nil
}

func validateFilterValue(field Field, value any) error {
	switch field.Type {
	case FieldNumber:
		number, isNumber := value.(float64)
		if !isNumber || math.IsNaN(number) || math.IsInf(number, 0) {
			return Invalid(fmt.Sprintf("%s must be a finite number", field.Label))
		}
	case FieldBoolean:
		if _, isBoolean := value.(bool); !isBoolean {
			return Invalid(fmt.Sprintf("%s must be true or false", field.Label))
		}
	case FieldText, FieldEmail, FieldDate:
		text, isText := value.(string)
		if !isText || len(text) > 10000 {
			return Invalid(fmt.Sprintf("Invalid %s", field.Label))
		}
		if field.Type == FieldDate {
			if _, err := time.Parse("2006-01-02", text); err != nil {
				return Invalid("Enter a date in YYYY-MM-DD format")
			}
		}
	default:
		return Invalid("Unsupported field type")
	}
	return nil
}

// ValidateActivity checks a timeline entry. Type is free text, not a fixed list.
func ValidateActivity(activityType, date, summary, channel, ref string) (time.Time, error) {
	activityType = strings.TrimSpace(activityType)
	summary = strings.TrimSpace(summary)
	if activityType == "" || len(activityType) > 80 || strings.ContainsAny(activityType, "\r\n") {
		return time.Time{}, Invalid("Enter an activity type")
	}
	if summary == "" || len(summary) > 4000 {
		return time.Time{}, Invalid("Enter an activity summary")
	}
	if len(channel) > 80 || len(ref) > 500 {
		return time.Time{}, Invalid("Activity channel or reference is too long")
	}
	if parsed, err := time.Parse(time.RFC3339, date); err == nil {
		return parsed.UTC(), nil
	}
	if parsed, err := time.Parse("2006-01-02", date); err == nil {
		return parsed.UTC(), nil
	}
	return time.Time{}, Invalid("Enter a date in YYYY-MM-DD or RFC3339 format")
}

func fieldIndex(fields []Field) map[string]Field {
	index := make(map[string]Field, len(fields))
	for _, field := range fields {
		index[field.ID] = field
	}
	return index
}

// StatusText reads the status field. Missing and blank values are empty.
func StatusText(values map[string]any) string {
	text, isText := values["status"].(string)
	if !isText {
		return ""
	}
	return strings.TrimSpace(text)
}

func HasField(fields []Field, identifier string) bool {
	_, known := fieldIndex(fields)[identifier]
	return known
}
