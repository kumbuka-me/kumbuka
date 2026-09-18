package wasm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/kumbuka-me/kumbuka/internal/renderprofile"
	"github.com/kumbuka-me/kumbuka/pkg/plugin"
	"github.com/kumbuka-me/sdk"
	"github.com/kumbuka-me/sdk/pluginpackage"
	"github.com/tetratelabs/wazero/api"
)

// Instance serializes calls into a reactor. The gate is released before host
// Markdown rendering, so recursive blocks never re-enter a suspended guest.
type Instance struct {
	// runtime owns executable plugin runtime operations.
	runtime *Runtime
	// compiled keeps the shared compiled module leased while this instance is alive.
	compiled *compiledLease
	// manifest contains the validated plugin manifest.
	manifest pluginpackage.Manifest
	// gate coordinates the state associated with gate.
	gate chan struct{}
	// module holds the active WebAssembly module instance.
	module api.Module
	// closed prevents calls after the instance has been shut down.
	closed bool
}

// Contributions returns the contributions owned by the instance.
func (i *Instance) Contributions() plugin.Contributions {
	var result plugin.Contributions

	for _, module := range i.manifest.Modules {
		if module.Type == "icon-resource" {
			continue
		}
		if module.Type == "admin-action" {
			result.AdminActions = append(result.AdminActions, plugin.AdminActionModule{
				ID: module.ID, Name: module.Name, Description: module.Description, Icon: module.Icon,
				Action: adminActionModule{rendererModule{instance: i, module: module}},
			})
			continue
		}
		if module.Type == "admin-resource" {
			result.AdminResources = append(result.AdminResources, plugin.AdminResource{ID: module.ID, Name: module.Name, Description: module.Description})
			continue
		}
		if module.Type == "editor-completion" {
			result.EditorCompletions = append(result.EditorCompletions, plugin.EditorCompletion{ID: module.ID, Resource: module.Resource, Trigger: module.Trigger, Replacement: module.Replacement, LabelField: module.LabelField, DetailField: module.DetailField})
			continue
		}
		if module.Type == "editor-insert" {
			result.EditorInserts = append(result.EditorInserts, plugin.EditorInsert{
				ID: module.ID, Name: module.Name, Description: module.Description,
				Markdown: module.Markdown, Suffix: module.Suffix, Placeholder: module.Placeholder,
				Mode: module.Mode, Group: module.Group, Icon: module.Icon, Inline: module.Inline,
			})
			continue
		}
		if module.Type == "content-substitution" {
			resource := manifestModule(i.manifest, module.Resource)
			result.ContentPreprocessors = append(result.ContentPreprocessors, resourceSubstitutionModule{owner: i.manifest.ID, module: module, resource: resource, storage: i.runtime.storage})
			continue
		}
		if module.Type == "code-highlighter" {
			adapter := codeHighlighterModule{rendererModule{instance: i, module: module}}
			result.CodeHighlighters = append(result.CodeHighlighters, plugin.CodeHighlighterModule{ID: module.ID, CSS: module.CSS, Highlighter: adapter})
			continue
		}
		if module.Type == "markdown-syntax" {
			result.MarkdownExtensions = append(result.MarkdownExtensions, syntaxModule{owner: i.manifest.ID, id: module.ID, syntax: module.Syntax, usage: sourceUsageRules(module.Usage)})
			continue
		}
		if module.Type == "settings" && len(module.Fields) == 0 {
			result.SettingsModules = append(result.SettingsModules, plugin.SettingsModule{ID: module.ID, Name: module.Name, Requires: module.Requires})
			continue
		}
		if module.Type == "content-style" {
			result.ContentStyles = append(result.ContentStyles, plugin.ContentStyle{ID: module.ID, CSS: module.CSS})
			continue
		}
		if module.Type == "render-policy" {
			result.RenderPolicies = append(result.RenderPolicies, plugin.RenderPolicy{ID: module.ID, Policy: module.Policy})
			continue
		}
		if module.Type == "browser-module" {
			result.BrowserModules = append(result.BrowserModules, plugin.BrowserModule{ID: module.ID, JavaScript: module.JavaScript, CSS: module.CSS})
			continue
		}
		if module.Type == "widget" {
			result.Widgets = append(result.Widgets, plugin.WidgetModule{ID: module.ID, Surface: module.Surface, Width: module.Width, Order: module.Order, Widget: widgetModule{rendererModule{instance: i, module: module}}})
			continue
		}
		if module.Type == "exporter" {
			result.Exporters = append(result.Exporters, plugin.ExporterModule{
				ID: module.ID, Name: module.Name, Description: module.Description, Icon: module.Icon, Order: module.Order,
				Exporter: exporterModule{rendererModule{instance: i, module: module}},
			})
			continue
		}
		if module.Type == "macro" {
			result.Macros = append(result.Macros, macroModule{rendererModule{instance: i, module: module}})
			continue
		}
		adapter := rendererModule{instance: i, module: module}
		switch module.Stage {
		case "content-preprocess":
			result.ContentPreprocessors = append(result.ContentPreprocessors, contentPreprocessorModule{rendererModule: adapter, priority: module.Priority})
		case "preprocess":
			result.Preprocessors = append(result.Preprocessors, adapter)
		case "postprocess":
			result.Postprocessors = append(result.Postprocessors, adapter)
		}
	}

	return result
}

// manifestModule returns one validated module by ID.
func manifestModule(manifest pluginpackage.Manifest, id string) pluginpackage.Module {
	for _, module := range manifest.Modules {
		if module.ID == id {
			return module
		}
	}
	return pluginpackage.Module{}
}

// Close releases resources held by the receiver.
func (i *Instance) Close(ctx context.Context) error {
	// Close marks the instance unavailable after the currently executing call.
	// Guest calls always have a bounded deadline, so draining is bounded too.
	i.gate <- struct{}{}
	defer func() { <-i.gate }()

	if i.closed {
		return nil
	}
	i.closed = true

	var err error
	if i.module != nil {
		err = i.module.Close(ctx)
	}
	if i.compiled != nil {
		err = errors.Join(err, i.compiled.Close(ctx))
	}

	return err
}

// invoke executes one serialized guest render request with the current capability scope.
func (i *Instance) invoke(ctx context.Context, request sdk.RenderRequest) (result sdk.RenderResult, err error) {
	trace := renderprofile.FromContext(ctx)
	profiled := trace != nil
	metrics := renderprofile.WASMCall{}
	started := timingStarted(profiled)
	if profiled {
		metrics.PluginID = i.manifest.ID
		metrics.ModuleID = request.Module
		metrics.Stage = request.Stage
		defer func() {
			metrics.Total = time.Since(started)
			metrics.Failed = err != nil
			trace.RecordWASM(metrics)
		}()
	}

	ctx, cancel := context.WithTimeout(ctx, i.runtime.limits.CallTimeout)
	defer cancel()

	gateStarted := timingStarted(profiled)
	select {
	case i.gate <- struct{}{}:
		if profiled {
			metrics.GateWait = time.Since(gateStarted)
		}
		defer func() { <-i.gate }()
	case <-ctx.Done():
		if profiled {
			metrics.GateWait = time.Since(gateStarted)
		}
		return sdk.RenderResult{}, ctx.Err()
	}

	if i.closed {
		return sdk.RenderResult{}, errors.New("WASM plugin is closed")
	}
	if i.compiled == nil {
		return sdk.RenderResult{}, errors.New("plugin has no executable WASM module")
	}
	if i.module == nil || i.module.IsClosed() {
		instantiateStarted := timingStarted(profiled)
		err = i.instantiate(ctx)
		if profiled {
			metrics.Instantiate = time.Since(instantiateStarted)
		}
		if err != nil {
			return sdk.RenderResult{}, err
		}
	}

	ctx = context.WithValue(ctx, callerKey{}, &invocationState{instance: i, remaining: 512})
	result, err = i.call(ctx, request, &metrics, profiled)
	if err != nil {
		// Discard a trapped or malformed reactor. A later request gets a clean
		// instance; no partial output or poisoned memory reaches another request.
		_ = i.module.Close(context.Background())
		i.module = nil
	}

	return result, err
}

// call writes one request into guest memory and decodes its bounded response.
func (i *Instance) call(
	ctx context.Context,
	request sdk.RenderRequest,
	metrics *renderprofile.WASMCall,
	profiled bool,
) (sdk.RenderResult, error) {
	input, err := i.encodeRequest(request, metrics, profiled)
	if err != nil {
		return sdk.RenderResult{}, err
	}

	pointer, err := i.writeRequest(ctx, input, metrics, profiled)
	if err != nil {
		return sdk.RenderResult{}, err
	}

	response, err := i.executeRequest(ctx, pointer, len(input), metrics, profiled)
	if err != nil {
		return sdk.RenderResult{}, err
	}

	result, err := decodeRenderResult(response, metrics, profiled)
	if err != nil {
		return sdk.RenderResult{}, err
	}
	if err := i.validateRenderResult(result, request.Stage, metrics, profiled); err != nil {
		return result, err
	}

	return result, nil
}

// encodeRequest serializes one guest request and enforces the wire-size limit.
func (i *Instance) encodeRequest(
	request sdk.RenderRequest,
	metrics *renderprofile.WASMCall,
	profiled bool,
) ([]byte, error) {
	started := timingStarted(profiled)
	input, err := json.Marshal(request)
	if profiled {
		metrics.Encode = time.Since(started)
		metrics.RequestBytes = len(input)
	}
	if err != nil {
		return nil, err
	}
	if len(input) > i.runtime.limits.WireBytes {
		return nil, errors.New("plugin request exceeds size limit")
	}

	return input, nil
}

// writeRequest allocates guest memory and copies the serialized request into it.
func (i *Instance) writeRequest(
	ctx context.Context,
	input []byte,
	metrics *renderprofile.WASMCall,
	profiled bool,
) (uint32, error) {
	started := timingStarted(profiled)
	allocated, err := i.module.ExportedFunction("kumbuka_alloc").Call(ctx, uint64(len(input)))
	if profiled {
		metrics.Allocate = time.Since(started)
	}
	if err != nil {
		return 0, fmt.Errorf("allocate plugin request: %w", err)
	}

	pointer := uint32(allocated[0])
	started = timingStarted(profiled)
	written := pointer != 0 && i.module.Memory().Write(pointer, input)
	if profiled {
		metrics.MemoryWrite = time.Since(started)
	}
	if !written {
		return 0, errors.New("plugin returned invalid request memory")
	}

	return pointer, nil
}

// executeRequest invokes the guest transform and returns its bounded response bytes.
func (i *Instance) executeRequest(
	ctx context.Context,
	pointer uint32,
	inputLength int,
	metrics *renderprofile.WASMCall,
	profiled bool,
) ([]byte, error) {
	started := timingStarted(profiled)
	output, err := i.module.ExportedFunction("kumbuka_transform").Call(ctx, uint64(pointer), uint64(inputLength))
	if profiled {
		metrics.Execute = time.Since(started)
	}
	if err != nil {
		return nil, fmt.Errorf("call plugin: %w", err)
	}

	pointer, length := uint32(output[0]), uint32(output[0]>>32)
	if profiled {
		metrics.ResponseBytes = int(length)
	}
	if length == 0 || uint64(length) > uint64(i.runtime.limits.WireBytes) {
		return nil, errors.New("plugin response exceeds size limit or is empty")
	}

	started = timingStarted(profiled)
	memory, ok := i.module.Memory().Read(pointer, length)
	if profiled {
		metrics.MemoryRead = time.Since(started)
	}
	if !ok {
		return nil, errors.New("plugin returned invalid response memory")
	}

	return memory, nil
}

// decodeRenderResult decodes exactly one strict JSON response value from the guest.
func decodeRenderResult(
	memory []byte,
	metrics *renderprofile.WASMCall,
	profiled bool,
) (sdk.RenderResult, error) {
	started := timingStarted(profiled)
	if profiled {
		defer func() { metrics.Decode = time.Since(started) }()
	}

	var result sdk.RenderResult
	decoder := json.NewDecoder(bytes.NewReader(memory))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&result); err != nil {
		return result, fmt.Errorf("decode plugin response: %w", err)
	}

	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return result, errors.New("plugin response must contain one JSON value")
	}

	return result, nil
}

// validateRenderResult enforces the response contract for one plugin invocation stage.
func (i *Instance) validateRenderResult(
	result sdk.RenderResult,
	stage string,
	metrics *renderprofile.WASMCall,
	profiled bool,
) error {
	started := timingStarted(profiled)
	if profiled {
		defer func() { metrics.Validate = time.Since(started) }()
	}

	if result.Error != "" {
		return fmt.Errorf("plugin returned error: %.1024s", result.Error)
	}
	if stage == "admin-action" {
		if result.Matched || len(result.Invocation) != 0 || len(result.Parts) != 0 || len(result.Actions) != 0 || result.File != nil || result.WidgetCommand != nil {
			return errors.New("invalid plugin admin action response")
		}
		return nil
	}
	if stage == "widget-command" {
		if len(result.Parts) != 0 || len(result.Actions) != 0 || result.File != nil || result.WidgetCommand == nil ||
			(result.WidgetCommand.Redirect != "" && !validWidgetActionURL(result.WidgetCommand.Redirect)) {
			return errors.New("invalid plugin widget command response")
		}
		return nil
	}
	if result.WidgetCommand != nil {
		return errors.New("unexpected plugin widget command response")
	}
	if stage == "export" {
		if len(result.Parts) != 0 || len(result.Actions) != 0 || result.File == nil {
			return errors.New("invalid plugin export response")
		}
		return nil
	}
	if result.File != nil {
		return errors.New("unexpected plugin export file")
	}
	if len(result.Parts) > i.runtime.limits.Parts {
		return errors.New("plugin returned too many fragments")
	}
	for _, part := range result.Parts {
		if !validRenderPart(part, stage) {
			return errors.New("invalid plugin render fragment")
		}
	}
	if len(result.Actions) > 32 {
		return errors.New("plugin returned too many widget actions")
	}
	for _, action := range result.Actions {
		if !validWidgetAction(action, stage) {
			return errors.New("invalid plugin widget action")
		}
	}

	return nil
}

// timingStarted returns the current time only when profiling is enabled.
func timingStarted(enabled bool) time.Time {
	if !enabled {
		return time.Time{}
	}

	return time.Now()
}

// validRenderPart reports whether one guest fragment is valid for the requested render stage.
func validRenderPart(part sdk.RenderPart, stage string) bool {
	return part.Markdown == nil || (part.Text == "" && stage == "preprocess")
}

// validWidgetAction validates bounded host-rendered widget action metadata.
func validWidgetAction(action sdk.WidgetAction, stage string) bool {
	if stage != "widget" {
		return false
	}
	if !validWidgetIdentifier(action.ID) || len(action.Label) == 0 || len(action.Label) > 256 {
		return false
	}
	if action.Icon != "" && !validWidgetIdentifier(action.Icon) {
		return false
	}
	switch action.Kind {
	case "link", "dialog":
		return action.Confirm == "" && validWidgetActionURL(action.URL)
	case "command":
		return action.URL == "" && len(action.Confirm) <= 512
	default:
		return false
	}
}

// validWidgetActionURL reports whether an action target is a bounded local path.
func validWidgetActionURL(value string) bool {
	return len(value) > 0 && len(value) <= 4096 &&
		strings.HasPrefix(value, "/") &&
		!strings.HasPrefix(value, "//") &&
		!strings.Contains(value, "\\")
}

// validWidgetIdentifier reports whether a widget identifier uses the supported ASCII syntax.
func validWidgetIdentifier(value string) bool {
	if len(value) == 0 || len(value) > 128 || !asciiAlphanumeric(value[0]) {
		return false
	}
	for index := 1; index < len(value); index++ {
		if asciiAlphanumeric(value[index]) || value[index] == '-' || value[index] == '_' || value[index] == '.' {
			continue
		}
		return false
	}
	return true
}

// asciiAlphanumeric reports whether a byte is an ASCII letter or digit.
func asciiAlphanumeric(value byte) bool {
	return value >= 'a' && value <= 'z' || value >= 'A' && value <= 'Z' || value >= '0' && value <= '9'
}

// rendererModule adapts one WASM renderer declaration to a pipeline stage.
type rendererModule struct {
	// instance owns the active executable plugin instance.
	instance *Instance
	// module holds the active WebAssembly module instance.
	module pluginpackage.Module
}

// SourceUsage exposes declarative source selectors without invoking guest code.
func (m rendererModule) SourceUsage() plugin.SourceUsage {
	rules := sourceUsageRules(m.module.Usage)
	if m.module.Type == "macro" && m.module.Name != "" {
		rules = append([]plugin.SourceUsageRule{{Macro: m.module.Name}}, rules...)
	}
	return plugin.SourceUsage{ModuleID: m.module.ID, Rules: rules}
}

// sourceUsageRules converts validated package selectors into internal immutable metadata.
func sourceUsageRules(rules []pluginpackage.UsageRule) []plugin.SourceUsageRule {
	result := make([]plugin.SourceUsageRule, 0, len(rules))
	for _, rule := range rules {
		result = append(result, plugin.SourceUsageRule{Contains: rule.Contains, Fence: rule.Fence, Macro: rule.Macro, Substitution: rule.Substitution})
	}
	return result
}

// Preprocess transforms Markdown before the core parser runs.
func (m rendererModule) Preprocess(ctx plugin.Context, source string) (string, error) {
	return m.render(ctx, source)
}

// Postprocess transforms rendered HTML before central sanitization.
func (m rendererModule) Postprocess(ctx plugin.Context, source string) (string, error) {
	return m.render(ctx, source)
}

// render invokes the guest renderer and assembles its returned fragments.
func (m rendererModule) render(ctx plugin.Context, source string) (string, error) {
	if enabled, configured := ctx.Features[m.instance.manifest.ID]; configured && !enabled {
		return source, nil
	}
	execution := ctx.Context
	if execution == nil {
		execution = context.Background()
	}

	execution = context.WithValue(execution, capabilitiesKey{}, ctx.Capabilities)
	result, err := m.instance.invoke(execution, sdk.RenderRequest{
		APIVersion: sdk.Version, Module: m.module.ID, Stage: m.module.Stage, Source: source, Features: ctx.Features,
	})
	if err != nil {
		return "", err
	}

	var output strings.Builder
	for _, part := range result.Parts {
		if err := execution.Err(); err != nil {
			return "", err
		}
		content := part.Text
		if part.Markdown != nil {
			if ctx.RenderMarkdown == nil {
				return "", errors.New("markdown rendering capability is unavailable")
			}
			content, err = ctx.RenderMarkdown(*part.Markdown)
			if err != nil {
				return "", err
			}
		}
		if len(content) > m.instance.runtime.limits.WireBytes-output.Len() {
			return "", errors.New("rendered plugin output exceeds size limit")
		}
		output.WriteString(content)
	}

	return output.String(), nil
}
