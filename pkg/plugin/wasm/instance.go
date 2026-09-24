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

	"github.com/kumbuka-me/kumbuka/pkg/plugin"
	"github.com/kumbuka-me/kumbuka/pkg/renderprofile"
	"github.com/kumbuka-me/sdk"
	"github.com/kumbuka-me/sdk/pluginpackage"
	"github.com/tetratelabs/wazero/api"
)

// Instance serializes calls into a reactor. The gate is released before host Markdown rendering, so recursive blocks never re-enter a suspended guest.
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
		if i.appendAdministrationContribution(&result, module) {
			continue
		}
		if i.appendPresentationContribution(&result, module) {
			continue
		}
		i.appendExecutableContribution(&result, module)
	}

	return result
}

// appendAdministrationContribution appends manifest-only administration and editor contributions.
func (i *Instance) appendAdministrationContribution(result *plugin.Contributions, module pluginpackage.Module) bool {
	switch plugin.ModuleType(module.Type) {
	case plugin.ModuleTypeIconResource:
		return true
	case plugin.ModuleTypeAdminAction:
		result.AdminActions = append(result.AdminActions, i.adminActionContribution(module))
	case plugin.ModuleTypeAdminResource:
		result.AdminResources = append(result.AdminResources, plugin.AdminResource{
			ID:          module.ID,
			Name:        module.Name,
			Description: module.Description,
		})
	case plugin.ModuleTypeEditorCompletion:
		result.EditorCompletions = append(result.EditorCompletions, plugin.EditorCompletion{
			ID:          module.ID,
			Resource:    module.Resource,
			Trigger:     module.Trigger,
			Replacement: module.Replacement,
			LabelField:  module.LabelField,
			DetailField: module.DetailField,
		})
	case plugin.ModuleTypeEditorInsert:
		result.EditorInserts = append(result.EditorInserts, editorInsertContribution(module))
	case plugin.ModuleTypeEditorMenu:
		result.EditorMenus = append(result.EditorMenus, plugin.EditorMenu{
			ID:            module.ID,
			Name:          module.Name,
			Description:   module.Description,
			Group:         module.Group,
			AllowedGroups: module.AllowedGroups,
			Icon:          module.Icon,
			Order:         module.Order,
			Children:      module.Children,
		})
	case plugin.ModuleTypeSettings:
		if len(module.Fields) == 0 {
			result.SettingsModules = append(result.SettingsModules, plugin.SettingsModule{
				ID:       module.ID,
				Name:     module.Name,
				Requires: module.Requires,
			})
		}
	default:
		return false
	}

	return true
}

// appendPresentationContribution appends passive Markdown and browser presentation contributions.
func (i *Instance) appendPresentationContribution(result *plugin.Contributions, module pluginpackage.Module) bool {
	switch plugin.ModuleType(module.Type) {
	case plugin.ModuleTypeMarkdownSyntax:
		result.MarkdownExtensions = append(result.MarkdownExtensions, syntaxModule{
			owner:  i.manifest.ID,
			id:     module.ID,
			syntax: module.Syntax,
			usage:  sourceUsageRules(module.Usage),
		})
	case plugin.ModuleTypeContentStyle:
		result.ContentStyles = append(result.ContentStyles, plugin.ContentStyle{
			ID:  module.ID,
			CSS: module.CSS,
		})
	case plugin.ModuleTypeRenderPolicy:
		result.RenderPolicies = append(result.RenderPolicies, plugin.RenderPolicy{
			ID:     module.ID,
			Policy: module.Policy,
		})
	case plugin.ModuleTypeBrowserModule:
		result.BrowserModules = append(result.BrowserModules, plugin.BrowserModule{
			ID:         module.ID,
			JavaScript: module.JavaScript,
			CSS:        module.CSS,
		})
	default:
		return false
	}

	return true
}

// appendExecutableContribution appends contributions backed by runtime code or resource substitution.
func (i *Instance) appendExecutableContribution(result *plugin.Contributions, module pluginpackage.Module) {
	switch plugin.ModuleType(module.Type) {
	case plugin.ModuleTypeContentSubstitution:
		resource := manifestModule(i.manifest, module.Resource)
		result.ContentPreprocessors = append(result.ContentPreprocessors, resourceSubstitutionModule{
			owner:    i.manifest.ID,
			module:   module,
			resource: resource,
			storage:  i.runtime.storage,
		})
	case plugin.ModuleTypeCodeHighlighter:
		adapter := codeHighlighterModule{rendererModule{instance: i, module: module}}
		result.CodeHighlighters = append(result.CodeHighlighters, plugin.CodeHighlighterModule{
			ID:          module.ID,
			CSS:         module.CSS,
			Highlighter: adapter,
		})
	case plugin.ModuleTypeWidget:
		result.Widgets = append(result.Widgets, plugin.WidgetModule{
			ID:      module.ID,
			Surface: module.Surface,
			Width:   module.Width,
			Order:   module.Order,
			Widget:  widgetModule{rendererModule{instance: i, module: module}},
		})
	case plugin.ModuleTypeExporter:
		result.Exporters = append(result.Exporters, i.exporterContribution(module))
	case plugin.ModuleTypeMacro:
		result.Macros = append(result.Macros, macroModule{rendererModule{instance: i, module: module}})
	default:
		i.appendRendererContribution(result, module)
	}
}

// adminActionContribution constructs one executable administration action contribution.
func (i *Instance) adminActionContribution(module pluginpackage.Module) plugin.AdminActionModule {
	return plugin.AdminActionModule{
		ID: module.ID, Name: module.Name, Description: module.Description, Icon: module.Icon,
		Action: adminActionModule{rendererModule{instance: i, module: module}},
	}
}

// editorInsertContribution constructs one editor insertion contribution from manifest metadata.
func editorInsertContribution(module pluginpackage.Module) plugin.EditorInsert {
	return plugin.EditorInsert{
		ID:            module.ID,
		Name:          module.Name,
		Description:   module.Description,
		Markdown:      module.Markdown,
		Suffix:        module.Suffix,
		Placeholder:   module.Placeholder,
		Mode:          plugin.EditorInsertMode(module.Mode),
		Group:         module.Group,
		AllowedGroups: module.AllowedGroups,
		Order:         module.Order,
		Icon:          module.Icon,
		Inline:        module.Inline,
	}
}

// exporterContribution constructs one executable export contribution.
func (i *Instance) exporterContribution(module pluginpackage.Module) plugin.ExporterModule {
	return plugin.ExporterModule{
		ID: module.ID, Name: module.Name, Description: module.Description, Icon: module.Icon, Order: module.Order,
		Exporter: exporterModule{rendererModule{instance: i, module: module}},
	}
}

// appendRendererContribution appends a stage-based renderer module to the matching contribution list.
func (i *Instance) appendRendererContribution(result *plugin.Contributions, module pluginpackage.Module) {
	adapter := rendererModule{instance: i, module: module}
	switch plugin.RenderStage(module.Stage) {
	case plugin.RenderStageContentPreprocess:
		result.ContentPreprocessors = append(result.ContentPreprocessors, contentPreprocessorModule{rendererModule: adapter, priority: module.Priority})
	case plugin.RenderStagePreprocess:
		result.Preprocessors = append(result.Preprocessors, adapter)
	case plugin.RenderStagePostprocess:
		result.Postprocessors = append(result.Postprocessors, adapter)
	}
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
	started := time.Now()
	if i.runtime.invocationObserver != nil {
		defer func() {
			i.runtime.invocationObserver.ObservePluginInvocation(
				i.manifest.ID,
				request.Module,
				request.Stage,
				time.Since(started),
				err,
			)
		}()
	}

	metrics, finishProfile := i.beginInvocationProfile(ctx, request)
	defer func() { finishProfile(err) }()

	ctx, cancel := context.WithTimeout(ctx, i.runtime.limits.CallTimeout)
	defer cancel()
	release, err := i.acquireInvocationGate(ctx, metrics)
	if err != nil {
		return sdk.RenderResult{}, err
	}
	defer release()

	if err := i.ensureInvocationModule(ctx, metrics); err != nil {
		return sdk.RenderResult{}, err
	}
	ctx = context.WithValue(ctx, callerKey{}, &invocationState{instance: i, remaining: 512})
	result, err = i.call(ctx, request, metrics, renderprofile.FromContext(ctx) != nil)
	if err != nil {
		i.discardInvocationModule()
	}
	return result, err
}

// beginInvocationProfile prepares optional WASM call profiling and returns its completion hook.
func (i *Instance) beginInvocationProfile(ctx context.Context, request sdk.RenderRequest) (*renderprofile.WASMCall, func(error)) {
	trace := renderprofile.FromContext(ctx)
	metrics := &renderprofile.WASMCall{}
	if trace == nil {
		return metrics, func(error) {}
	}
	metrics.PluginID = i.manifest.ID
	metrics.ModuleID = request.Module
	metrics.Stage = request.Stage
	started := time.Now()
	return metrics, func(err error) {
		metrics.Total = time.Since(started)
		metrics.Failed = err != nil
		trace.RecordWASM(*metrics)
	}
}

// acquireInvocationGate serializes guest access and records optional gate wait time.
func (i *Instance) acquireInvocationGate(ctx context.Context, metrics *renderprofile.WASMCall) (func(), error) {
	started := time.Now()
	select {
	case i.gate <- struct{}{}:
		metrics.GateWait = time.Since(started)
		return func() { <-i.gate }, nil
	case <-ctx.Done():
		metrics.GateWait = time.Since(started)
		return nil, ctx.Err()
	}
}

// ensureInvocationModule validates instance state and instantiates a fresh guest when needed.
func (i *Instance) ensureInvocationModule(ctx context.Context, metrics *renderprofile.WASMCall) error {
	if i.closed {
		return errors.New("WASM plugin is closed")
	}
	if i.compiled == nil {
		return errors.New("plugin has no executable WASM module")
	}
	if i.module != nil && !i.module.IsClosed() {
		return nil
	}
	started := time.Now()
	err := i.instantiate(ctx)
	metrics.Instantiate = time.Since(started)
	return err
}

// discardInvocationModule removes a trapped or malformed reactor before the next request.
func (i *Instance) discardInvocationModule() {
	if i.module != nil {
		_ = i.module.Close(context.Background())
	}
	i.module = nil
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
	if err := i.validateRenderResult(result, plugin.RenderStage(request.Stage), metrics, profiled); err != nil {
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
	stage plugin.RenderStage,
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
	if err := validateStageSpecificRenderResult(result, stage); err != nil {
		return err
	}
	if stage == plugin.RenderStageAdminAction || stage == plugin.RenderStageWidgetCommand || stage == plugin.RenderStageExport {
		return nil
	}
	return i.validateGeneralRenderResult(result, stage)
}

// validateStageSpecificRenderResult validates response shapes reserved for special invocation stages.
func validateStageSpecificRenderResult(result sdk.RenderResult, stage plugin.RenderStage) error {
	switch stage {
	case plugin.RenderStageAdminAction:
		if !validAdminActionResult(result) {
			return errors.New("invalid plugin admin action response")
		}
	case plugin.RenderStageWidgetCommand:
		if !validWidgetCommandResult(result) {
			return errors.New("invalid plugin widget command response")
		}
	case plugin.RenderStageExport:
		if !validExportResult(result) {
			return errors.New("invalid plugin export response")
		}
	default:
		if result.WidgetCommand != nil {
			return errors.New("unexpected plugin widget command response")
		}
		if result.File != nil {
			return errors.New("unexpected plugin export file")
		}
	}
	return nil
}

// validateGeneralRenderResult validates bounded render fragments and widget actions.
func (i *Instance) validateGeneralRenderResult(result sdk.RenderResult, stage plugin.RenderStage) error {
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

// validAdminActionResult reports whether an admin action returned only its permitted response fields.
func validAdminActionResult(result sdk.RenderResult) bool {
	return !result.Matched &&
		len(result.Invocation) == 0 &&
		len(result.Parts) == 0 &&
		len(result.Actions) == 0 &&
		result.File == nil &&
		result.WidgetCommand == nil
}

// validWidgetCommandResult reports whether a widget command returned only a valid command response.
func validWidgetCommandResult(result sdk.RenderResult) bool {
	if !hasOnlyWidgetCommandResult(result) {
		return false
	}

	return result.WidgetCommand.Redirect == "" || validWidgetActionURL(result.WidgetCommand.Redirect)
}

// validExportResult reports whether an export invocation returned exactly one file response.
func validExportResult(result sdk.RenderResult) bool {
	return len(result.Parts) == 0 && len(result.Actions) == 0 && result.File != nil
}

// timingStarted returns the current time only when profiling is enabled.
func timingStarted(enabled bool) time.Time {
	if !enabled {
		return time.Time{}
	}

	return time.Now()
}

// validRenderPart reports whether one guest fragment is valid for the requested render stage.
func validRenderPart(part sdk.RenderPart, stage plugin.RenderStage) bool {
	return part.Markdown == nil || (part.Text == "" && stage == plugin.RenderStagePreprocess)
}

// validWidgetAction validates bounded host-rendered widget action metadata.
func validWidgetAction(action sdk.WidgetAction, stage plugin.RenderStage) bool {
	if stage != plugin.RenderStageWidget {
		return false
	}
	if !validWidgetActionIdentity(action) {
		return false
	}
	if action.Icon != "" && !validWidgetIdentifier(action.Icon) {
		return false
	}
	switch plugin.WidgetActionKind(action.Kind) {
	case plugin.WidgetActionLink, plugin.WidgetActionDialog:
		return action.Confirm == "" && validWidgetActionURL(action.URL)
	case plugin.WidgetActionCommand:
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
	if !validWidgetIdentifierBoundary(value) {
		return false
	}
	for index := 1; index < len(value); index++ {
		if validWidgetIdentifierCharacter(value[index]) {
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

// hasOnlyWidgetCommandResult reports whether a render result contains exactly one widget command response.
func hasOnlyWidgetCommandResult(result sdk.RenderResult) bool {
	return len(result.Parts) == 0 && len(result.Actions) == 0 && result.File == nil && result.WidgetCommand != nil
}

// validWidgetActionIdentity reports whether action has a valid identifier and bounded non-empty label.
func validWidgetActionIdentity(action sdk.WidgetAction) bool {
	return validWidgetIdentifier(action.ID) && len(action.Label) > 0 && len(action.Label) <= 256
}

// validWidgetIdentifierBoundary reports whether a widget identifier has a valid length and first character.
func validWidgetIdentifierBoundary(value string) bool {
	return len(value) > 0 && len(value) <= 128 && asciiAlphanumeric(value[0])
}

// validWidgetIdentifierCharacter reports whether a non-leading widget identifier byte is supported.
func validWidgetIdentifierCharacter(value byte) bool {
	return asciiAlphanumeric(value) || value == '-' || value == '_' || value == '.'
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
	if plugin.ModuleType(m.module.Type) == plugin.ModuleTypeMacro && m.module.Name != "" {
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
