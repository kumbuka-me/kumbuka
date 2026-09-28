package wasm

import (
	"context"

	"github.com/kumbuka-me/kumbuka/pkg/plugin"
)

type invocationLocaleKey struct{}

// withInvocationLocale carries the host-resolved locale to the wire encoder.
func withInvocationLocale(ctx context.Context, pluginContext plugin.Context) context.Context {
	if pluginContext.Locale == "" {
		return ctx
	}
	return context.WithValue(ctx, invocationLocaleKey{}, pluginContext.Locale)
}

// invocationLocale returns the request locale attached by an adapter.
func invocationLocale(ctx context.Context) string {
	locale, _ := ctx.Value(invocationLocaleKey{}).(string)
	return locale
}
