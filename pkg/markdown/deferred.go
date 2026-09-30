package markdown

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/kumbuka-me/kumbuka/pkg/plugin"
)

// ErrDeferredMacroNotFound reports that a fragment reference no longer points
// at the expected macro invocation in the saved page revision.
var ErrDeferredMacroNotFound = errors.New("deferred macro not found")

// ShouldDeferMacro reports whether an enabled macro belongs to a plugin that can perform network HTTP requests. Keeping this policy in the host makes all remote-content macros non-blocking without exposing their credentials or provider URLs to browser code.
func (r *Renderer) ShouldDeferMacro(pluginID, moduleID string) bool {
	if r == nil || r.manager == nil || pluginID == "" || moduleID == "" {
		return false
	}
	for _, loaded := range r.manager.Plugins() {
		if !loaded.Enabled || loaded.Manifest.ID != pluginID ||
			!slices.Contains(loaded.Manifest.Permissions, "network:http") {
			continue
		}
		for _, module := range loaded.Manifest.Modules {
			if module.Type == "macro" && module.ID == moduleID {
				return true
			}
		}
	}
	return false
}

// RenderDeferredMacro re-derives one macro invocation from canonical Markdown and renders only that fragment. Callers must authorize the page and verify its revision before invoking this method.
func (r *Renderer) RenderDeferredMacro(
	source string,
	index int,
	pluginID string,
	moduleID string,
	resolve func(string) string,
	options Options,
	functions Functions,
) (string, error) {
	if index < 0 || pluginID == "" || moduleID == "" {
		return "", ErrDeferredMacroNotFound
	}
	execution := functions.Context
	if execution == nil {
		execution = context.Background()
	}
	execution, cancel := context.WithTimeout(execution, 30*time.Second)
	defer cancel()
	functions.Context = execution
	functions.DeferMacro = nil
	functions.DeferredVersion = ""

	plan, release := r.registry.AcquireRenderPlan()
	defer release()
	options.pipeline = newRenderPipeline(plan, r.pluginFeatures(), functions, source, r.iconCatalog)

	prepared, err := options.pipeline.prepareContent(source, r.moduleContext(resolve, options))
	if err != nil {
		return "", err
	}
	options.pipeline.setUsageSource(prepared.Markdown)
	options.pipeline.opaqueReplacements = len(prepared.Replacements) != 0
	options.annotations = prepared.Replacements
	pagePlan := options.pipeline.pagePlanForSource(prepared.Markdown)
	_, invocations, err := options.pipeline.preprocessMacros(
		prepared.Markdown,
		r.moduleContext(resolve, options),
		pagePlan,
	)
	if err != nil {
		return "", err
	}
	if index >= len(invocations) {
		return "", ErrDeferredMacroNotFound
	}
	invocation := invocations[index]
	if invocation.owner != pluginID || invocation.module != moduleID {
		return "", ErrDeferredMacroNotFound
	}

	result, err := plugin.Guard(invocation.owner, func() (string, error) {
		return invocation.macro.Render(r.moduleContext(resolve, options), invocation.arguments)
	})
	if err != nil {
		return "", err
	}
	return r.sanitizer.Sanitize(result), nil
}
