package plugin

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/kumbuka-me/sdk"
)

// WidgetCommand invokes one active widget command while pinning its plugin lifetime.
func (m *Manager) WidgetCommand(
	ctx context.Context,
	pluginID, moduleID string,
	scope Context,
	request WidgetCommandRequest,
) (sdk.WidgetCommandResult, error) {
	if !validWidgetCommandRequest(request) {
		return sdk.WidgetCommandResult{}, fmt.Errorf("invalid widget command")
	}

	entry, release, ok := m.registry.AcquireEntry(pluginID)
	if !ok {
		return sdk.WidgetCommandResult{}, fmt.Errorf("plugin %s is not active", pluginID)
	}
	defer release()

	for _, module := range entry.Contributions.Widgets {
		if module.ID != moduleID {
			continue
		}
		if module.Surface != request.Surface {
			return sdk.WidgetCommandResult{}, fmt.Errorf("widget %s is not available on surface %s", moduleID, request.Surface)
		}
		commander, ok := module.Widget.(WidgetCommander)
		if !ok {
			return sdk.WidgetCommandResult{}, fmt.Errorf("widget %s does not accept commands", moduleID)
		}
		scope.Context = ctx
		result, err := Guard(pluginID, func() (sdk.WidgetCommandResult, error) { return commander.Command(scope, request) })
		if err != nil {
			return sdk.WidgetCommandResult{}, err
		}
		if result.Redirect != "" && !validWidgetCommandRedirect(result.Redirect) {
			return sdk.WidgetCommandResult{}, fmt.Errorf("plugin %s returned an invalid widget redirect", pluginID)
		}
		return result, nil
	}
	return sdk.WidgetCommandResult{}, fmt.Errorf("widget %s is not active in plugin %s", moduleID, pluginID)
}

// validWidgetCommandRequest reports whether a widget command names a supported surface and action identifier.
func validWidgetCommandRequest(request WidgetCommandRequest) bool {
	return sdk.ValidWidgetSurface(request.Surface) && validID.MatchString(request.Action)
}

// validWidgetCommandRedirect accepts bounded local application paths only.
func validWidgetCommandRedirect(value string) bool {
	if !validWidgetCommandRedirectShape(value) {
		return false
	}
	parsed, err := url.ParseRequestURI(value)
	return err == nil && parsed.Host == "" && !parsed.IsAbs()
}

// validWidgetCommandRedirectShape reports whether a redirect has the bounded local-path shape required before URL parsing.
func validWidgetCommandRedirectShape(value string) bool {
	return len(value) > 0 && len(value) <= 4096 && strings.HasPrefix(value, "/") &&
		!strings.HasPrefix(value, "//") && !strings.ContainsAny(value, "\\\r\n\t")
}
