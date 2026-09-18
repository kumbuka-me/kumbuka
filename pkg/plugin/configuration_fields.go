package plugin

import (
	"errors"
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
	if field.Type == "text" || field.Type == "url" || field.Type == "select" || field.Type == "boolean" || field.Key {
		value = strings.TrimSpace(value)
	}
	if !utf8.ValidString(value) || strings.ContainsRune(value, '\x00') {
		return "", configurationFieldError(field, field.Name+" must contain valid UTF-8 text.")
	}
	if field.Type == "secret" && strings.IndexFunc(value, unicode.IsControl) >= 0 {
		return "", configurationFieldError(field, field.Name+" must not contain control characters.")
	}

	limit := field.MaxBytes
	if limit == 0 {
		switch {
		case field.Key:
			limit = maxResourceKeyBytes
		case field.Type == "textarea":
			limit = 48 << 10
		default:
			limit = 4096
		}
	}
	if len(value) > limit {
		return "", configurationFieldError(field, field.Name+" is too long.")
	}

	switch field.Type {
	case "text", "textarea", "secret":
		return value, nil
	case "boolean":
		if value != "true" && value != "false" {
			return "", configurationFieldError(field, field.Name+" must be true or false.")
		}
		return value, nil
	case "select":
		if value == "" && !field.Required {
			return "", nil
		}
		for _, option := range field.Options {
			if value == option {
				return value, nil
			}
		}
		return "", configurationFieldError(field, field.Name+" has an unsupported value.")
	case "url":
		if value == "" {
			return value, nil
		}
		parsed, err := url.ParseRequestURI(value)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil {
			return "", configurationFieldError(field, field.Name+" must be an absolute HTTP or HTTPS URL.")
		}
		return value, nil
	default:
		return "", configurationFieldError(field, field.Name+" uses an unsupported field type.")
	}
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
