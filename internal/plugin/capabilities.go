package plugin

import (
	"context"
	"encoding/json"
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
	// WritePluginValue writes plugin value.
	WritePluginValue(context.Context, string, string, string, []byte) error
	// DeletePluginValue deletes one plugin value. Missing keys are ignored.
	DeletePluginValue(context.Context, string, string, string) error
}
