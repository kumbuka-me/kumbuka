package plugin

import (
	"context"
	"encoding/json"
	"errors"
)

const pluginSettingsNamespace = "settings"

var (
	// ErrPluginValueAlreadyExists indicates that an atomic plugin-value replacement would overwrite another key.
	ErrPluginValueAlreadyExists = errors.New("plugin value already exists")
	// ErrPluginValueNotFound indicates that the source key of an atomic plugin-value replacement no longer exists.
	ErrPluginValueNotFound = errors.New("plugin value not found")
)

// Capability is a request-scoped, authorization-filtered host operation. It must
// respect cancellation and may return only public Plugin API values.
type Capability func(context.Context, json.RawMessage) (any, error)

// Storage is a trusted adapter. Identity and namespace are supplied by core,
// never decoded from plugin arguments.
type Storage interface {
	// ReadPluginValue reads plugin value.
	ReadPluginValue(context.Context, string, string, string) ([]byte, bool, error)
	// ListPluginValues lists values whose keys share prefix in deterministic key order.
	ListPluginValues(context.Context, string, string, string) (map[string][]byte, error)
	// WritePluginValue writes one plugin value.
	WritePluginValue(context.Context, string, string, string, []byte) error
	// WritePluginValues atomically writes a complete set of values in one namespace.
	WritePluginValues(context.Context, string, string, map[string][]byte) error
	// DeletePluginValue deletes one plugin value. Missing keys are ignored.
	DeletePluginValue(context.Context, string, string, string) error
	// ReplacePluginValue atomically replaces oldKey with newKey and rejects collisions.
	ReplacePluginValue(context.Context, string, string, string, string, []byte) error
}
