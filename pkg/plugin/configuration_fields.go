package plugin

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/kumbuka-me/sdk/pluginpackage"
)

var (
	// ErrSecretEncryptionUnavailable reports that secret-backed plugin configuration cannot be persisted safely.
	ErrSecretEncryptionUnavailable = errors.New("plugin secret encryption is unavailable")
)

// ConfigurationFieldError reports a safe validation problem for one declarative configuration field.
type ConfigurationFieldError struct {
	// Field identifies the manifest field that failed validation.
	Field string
	// Message contains the safe administrator-facing validation message.
	Message string
}

// Error returns the safe configuration field validation message.
func (e *ConfigurationFieldError) Error() string {
	return e.Message
}

// normalizeConfigurationValue validates and normalizes one typed plugin configuration value.
func normalizeConfigurationValue(field pluginpackage.ConfigurationField, value string) (string, error) {
	value = normalizeConfigurationWhitespace(field, value)

	if err := validateConfigurationText(field, value); err != nil {
		return "", err
	}
	if len(value) > configurationValueLimit(field) {
		return "", configurationFieldError(field, field.Name+" is too long.")
	}

	return validateConfigurationType(field, value)
}

// normalizeConfigurationWhitespace trims field types whose surrounding whitespace is never meaningful.
func normalizeConfigurationWhitespace(field pluginpackage.ConfigurationField, value string) string {
	if field.Key || field.Type == "text" || field.Type == "url" || field.Type == "select" || field.Type == "boolean" || field.Type == "color" {
		return strings.TrimSpace(value)
	}

	return value
}

// validateConfigurationText enforces the shared UTF-8 and control-character policy.
func validateConfigurationText(field pluginpackage.ConfigurationField, value string) error {
	if !utf8.ValidString(value) || strings.ContainsRune(value, '\x00') {
		return configurationFieldError(field, field.Name+" must contain valid UTF-8 text.")
	}
	if field.Type == "secret" && strings.IndexFunc(value, unicode.IsControl) >= 0 {
		return configurationFieldError(field, field.Name+" must not contain control characters.")
	}

	return nil
}

// configurationValueLimit returns the explicit or type-specific byte limit for one field.
func configurationValueLimit(field pluginpackage.ConfigurationField) int {
	if field.MaxBytes > 0 {
		return field.MaxBytes
	}
	if field.Key {
		return maxResourceKeyBytes
	}
	if field.Type == "textarea" || field.Type == "list" {
		return 48 << 10
	}

	return 4096
}

// validateConfigurationType validates type-specific syntax after shared text policy succeeds.
func validateConfigurationType(field pluginpackage.ConfigurationField, value string) (string, error) {
	switch field.Type {
	case "text", "textarea", "secret":
		return value, nil
	case "color":
		return validateColorConfiguration(field, value)
	case "list":
		return validateListConfiguration(field, value)
	case "boolean":
		return validateBooleanConfiguration(field, value)
	case "select":
		return validateSelectConfiguration(field, value)
	case "url":
		return validateURLConfiguration(field, value)
	default:
		return "", configurationFieldError(field, field.Name+" uses an unsupported field type.")
	}
}

// validateColorConfiguration accepts canonical six-digit CSS hex colors.
func validateColorConfiguration(field pluginpackage.ConfigurationField, value string) (string, error) {
	if value == "" && !field.Required {
		return "", nil
	}
	if len(value) != 7 || value[0] != '#' {
		return "", configurationFieldError(field, field.Name+" must be a six-digit hex color.")
	}
	for _, char := range value[1:] {
		if char >= '0' && char <= '9' || char >= 'a' && char <= 'f' || char >= 'A' && char <= 'F' {
			continue
		}
		return "", configurationFieldError(field, field.Name+" must be a six-digit hex color.")
	}
	return strings.ToLower(value), nil
}

// validateListConfiguration normalizes one repeatable structured configuration value as canonical JSON.
func validateListConfiguration(field pluginpackage.ConfigurationField, value string) (string, error) {
	if strings.TrimSpace(value) == "" {
		if field.Required {
			return "", configurationFieldError(field, field.Name+" requires at least one row.")
		}
		return "", nil
	}

	var rows []map[string]string
	if err := json.Unmarshal([]byte(value), &rows); err != nil {
		return "", configurationFieldError(field, field.Name+" contains invalid rows.")
	}
	limit := field.MaxItems
	if limit == 0 {
		limit = 16
	}
	if len(rows) == 0 && field.Required {
		return "", configurationFieldError(field, field.Name+" requires at least one row.")
	}
	if len(rows) > limit {
		return "", configurationFieldError(field, fmt.Sprintf("%s allows at most %d rows.", field.Name, limit))
	}

	columns := make(map[string]pluginpackage.ConfigurationField, len(field.Columns))
	for _, column := range field.Columns {
		columns[column.ID] = column
	}

	normalizedRows := make([]map[string]string, 0, len(rows))
	for rowIndex, row := range rows {
		for key := range row {
			if _, ok := columns[key]; !ok {
				return "", configurationFieldError(field, fmt.Sprintf("%s row %d contains an unknown column.", field.Name, rowIndex+1))
			}
		}

		normalizedRow := make(map[string]string, len(field.Columns))
		for _, column := range field.Columns {
			normalized, err := normalizeConfigurationValue(column, row[column.ID])
			if err != nil {
				var fieldErr *ConfigurationFieldError
				if errors.As(err, &fieldErr) {
					return "", configurationFieldError(field, fmt.Sprintf("%s row %d: %s", field.Name, rowIndex+1, fieldErr.Message))
				}
				return "", err
			}
			if column.Required && normalized == "" {
				return "", configurationFieldError(field, fmt.Sprintf("%s row %d: %s is required.", field.Name, rowIndex+1, column.Name))
			}
			normalizedRow[column.ID] = normalized
		}
		normalizedRows = append(normalizedRows, normalizedRow)
	}

	encoded, err := json.Marshal(normalizedRows)
	if err != nil {
		return "", configurationFieldError(field, field.Name+" contains invalid rows.")
	}
	if len(encoded) > configurationValueLimit(field) {
		return "", configurationFieldError(field, field.Name+" is too long.")
	}
	return string(encoded), nil
}

// validateBooleanConfiguration accepts the canonical persisted boolean values only.
func validateBooleanConfiguration(field pluginpackage.ConfigurationField, value string) (string, error) {
	if value != "true" && value != "false" {
		return "", configurationFieldError(field, field.Name+" must be true or false.")
	}

	return value, nil
}

// validateSelectConfiguration checks optional emptiness and membership in the declared options.
func validateSelectConfiguration(field pluginpackage.ConfigurationField, value string) (string, error) {
	if value == "" && !field.Required {
		return "", nil
	}

	for _, option := range field.Options {
		if value == option {
			return value, nil
		}
	}

	return "", configurationFieldError(field, field.Name+" has an unsupported value.")
}

// validateURLConfiguration accepts empty values or absolute credential-free HTTP and HTTPS URLs.
func validateURLConfiguration(field pluginpackage.ConfigurationField, value string) (string, error) {
	if value == "" {
		return "", nil
	}

	parsed, err := url.ParseRequestURI(value)
	if err != nil || parsed.Host == "" || parsed.User != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return "", configurationFieldError(field, field.Name+" must be an absolute HTTP or HTTPS URL.")
	}

	return value, nil
}

// requiredConfigurationValueMissing reports whether a required field has neither a submitted value nor a preserved secret.
func requiredConfigurationValueMissing(field pluginpackage.ConfigurationField, value string, previous map[string]string) bool {
	if !field.Required || value != "" {
		return false
	}

	return field.Type != "secret" || previous[field.ID] == ""
}

// encryptConfigurationSecrets replaces submitted plaintext secrets with encrypted persisted values.
func (m *Manager) encryptConfigurationSecrets(module pluginpackage.Module, values, previous map[string]string) error {
	for _, field := range module.Fields {
		if field.Type != "secret" {
			continue
		}

		plain := values[field.ID]
		if plain == "" && previous[field.ID] != "" {
			values[field.ID] = previous[field.ID]
			continue
		}
		if plain == "" {
			if field.Required {
				return configurationFieldError(field, field.Name+" is required.")
			}
			continue
		}
		if m.secrets == nil || !m.secrets.Configured() {
			return ErrSecretEncryptionUnavailable
		}

		encrypted, err := m.secrets.Encrypt(plain)
		if err != nil {
			return errors.New("could not encrypt plugin secret")
		}
		values[field.ID] = encrypted
	}
	return nil
}

// configurationFieldError creates one safe field-scoped plugin configuration validation error.
func configurationFieldError(field pluginpackage.ConfigurationField, message string) error {
	return &ConfigurationFieldError{Field: field.ID, Message: message}
}
