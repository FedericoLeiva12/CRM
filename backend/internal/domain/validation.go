package domain

import (
	"errors"
	"fmt"
	"math"
	"net/mail"
	"regexp"
	"strings"
	"time"
)

var identifierPattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,47}$`)

func ValidIdentifier(value string) bool { return identifierPattern.MatchString(value) }

// ValidationError contains a message safe to return to the person editing a record.
type ValidationError struct{ Message string }

func (err *ValidationError) Error() string { return err.Message }
func Invalid(message string) error         { return &ValidationError{Message: message} }
func IsValidationError(err error) bool {
	var validationError *ValidationError
	return errors.As(err, &validationError)
}
func ValidateSection(identifier, name string) error {
	if !identifierPattern.MatchString(identifier) || strings.TrimSpace(name) == "" || len(name) > 80 {
		return Invalid("Use a valid identifier and section name")
	}
	// Per-section tools are named <section>_<verb>; this id would collide with the global mentions_* tools.
	if identifier == "mentions" {
		return Invalid("That section identifier is reserved")
	}
	return nil
}
func ValidateField(field Field) error {
	if !identifierPattern.MatchString(field.ID) || strings.TrimSpace(field.Label) == "" || len(field.Label) > 80 {
		return Invalid("Use a valid field identifier and label")
	}
	switch field.Type {
	case FieldText, FieldEmail, FieldNumber, FieldDate, FieldBoolean:
		return nil
	default:
		return Invalid("Unsupported field type")
	}
}

// ValidateRecord is shared by HTTP and MCP, so agents cannot bypass form rules.
func ValidateRecord(fields []Field, values map[string]any) error {
	definitions := make(map[string]Field, len(fields))
	for _, field := range fields {
		definitions[field.ID] = field
		value, present := values[field.ID]
		if field.Required && (!present || isEmpty(value)) {
			return Invalid(fmt.Sprintf("%s is required", field.Label))
		}
	}
	for identifier, value := range values {
		field, known := definitions[identifier]
		if !known {
			return Invalid(fmt.Sprintf("Unknown field: %s", identifier))
		}
		if isEmpty(value) {
			continue
		}
		if err := validateValue(field, value); err != nil {
			return err
		}
	}
	return nil
}
func isEmpty(value any) bool {
	if value == nil {
		return true
	}
	text, isText := value.(string)
	return isText && strings.TrimSpace(text) == ""
}
func validateValue(field Field, value any) error {
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
		if field.Type == FieldEmail {
			address, err := mail.ParseAddress(text)
			if err != nil || address.Address != text {
				return Invalid("Enter a valid email address")
			}
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
