package wasm

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/kumbuka-me/kumbuka/pkg/plugin"
	"github.com/kumbuka-me/sdk"
	pluginmarkdown "github.com/kumbuka-me/sdk/markdown"
	"github.com/kumbuka-me/sdk/pluginpackage"
)

// contentPreprocessorModule adapts a WASM content-preprocess renderer declaration.
type contentPreprocessorModule struct {
	// rendererModule embeds renderer module behavior in content preprocessor module.
	rendererModule
	// priority stores the priority setting for content preprocessor module.
	priority int
}

// Priority orders content preprocessing before normal Markdown rendering.
func (m contentPreprocessorModule) Priority() int { return m.priority }

// PreprocessContent invokes the guest and returns transformed Markdown.
func (m contentPreprocessorModule) PreprocessContent(ctx plugin.Context, source string) (plugin.PreparedContent, error) {
	if m.module.Capability != "" {
		if ctx.Capabilities[m.module.Capability] == nil {
			return plugin.PreparedContent{Markdown: source}, nil
		}
	}
	execution := ctx.Context
	if execution == nil {
		execution = context.Background()
	}
	execution = context.WithValue(execution, capabilitiesKey{}, ctx.Capabilities)
	result, err := m.instance.invoke(execution, sdk.RenderRequest{
		APIVersion: sdk.Version,
		Module:     m.module.ID,
		Stage:      string(plugin.RenderStageContentPreprocess),
		Source:     source,
		Features:   ctx.Features,
	})
	if err != nil {
		return plugin.PreparedContent{}, err
	}
	var output strings.Builder
	for _, part := range result.Parts {
		if part.Markdown != nil {
			return plugin.PreparedContent{}, errors.New("content preprocessors may return text only")
		}
		if len(part.Text) > m.instance.runtime.limits.WireBytes-output.Len() {
			return plugin.PreparedContent{}, errors.New("content preprocessor output exceeds size limit")
		}
		output.WriteString(part.Text)
	}
	return plugin.PreparedContent{Markdown: output.String()}, nil
}

// resourceSubstitutionModule expands inline resource-backed macros without rescanning inserted values.
type resourceSubstitutionModule struct {
	// owner stores the owner value used by resource substitution module.
	owner string
	// module stores the module value used by resource substitution module.
	module pluginpackage.Module
	// resource stores the resource value used by resource substitution module.
	resource pluginpackage.Module
	// storage stores the storage value used by resource substitution module.
	storage plugin.Storage
}

// Priority orders content substitutions after earlier content preprocessors such as includes.
func (m resourceSubstitutionModule) Priority() int { return m.module.Priority }

// SourceUsage lets Kumbuka index resource references without loading plugin storage.
func (m resourceSubstitutionModule) SourceUsage() plugin.SourceUsage {
	return plugin.SourceUsage{
		ModuleID: m.module.ID,
		Rules:    []plugin.SourceUsageRule{{Substitution: m.module.Prefix}},
	}
}

// PreprocessContent replaces matching macros with opaque tokens and contributes inspector/export metadata.
func (m resourceSubstitutionModule) PreprocessContent(ctx plugin.Context, source string) (plugin.PreparedContent, error) {
	overrides := exportOverrides(ctx, m.owner, m.module.ID)
	if !m.resourceSubstitutionNeeded(source, overrides) {
		return plugin.PreparedContent{Markdown: source}, nil
	}

	records, err := m.loadResourceRecords(ctx)
	if err != nil {
		return plugin.PreparedContent{}, err
	}
	if len(records) == 0 {
		if len(overrides) != 0 {
			return plugin.PreparedContent{}, unusedExportParameter(m.owner, m.module.ID, overrides)
		}
		return plugin.PreparedContent{Markdown: source}, nil
	}

	prefix, err := resourceReplacementPrefix()
	if err != nil {
		return plugin.PreparedContent{}, err
	}
	prepared, used, err := m.expandResourceSource(ctx, source, recordsByName(records), prefix)
	if err != nil {
		return plugin.PreparedContent{}, err
	}
	if err := validateUsedExportOverrides(m.owner, m.module.ID, overrides, used); err != nil {
		return plugin.PreparedContent{}, err
	}
	return prepared, nil
}

// resourceSubstitutionNeeded reports whether storage is needed for source replacement or export overrides.
func (m resourceSubstitutionModule) resourceSubstitutionNeeded(source string, overrides map[string]string) bool {
	return m.storage != nil && (strings.Contains(source, "{{"+m.module.Prefix+":") || len(overrides) != 0)
}

// loadResourceRecords reads this module's declared resource records with a usable execution context.
func (m resourceSubstitutionModule) loadResourceRecords(ctx plugin.Context) ([]plugin.ResourceRecord, error) {
	execution := ctx.Context
	if execution == nil {
		execution = context.Background()
	}
	return plugin.ReadResourceRecords(execution, m.storage, m.owner, m.resource)
}

// recordsByName indexes resource records by their case-insensitive key.
func recordsByName(records []plugin.ResourceRecord) map[string]plugin.ResourceRecord {
	result := make(map[string]plugin.ResourceRecord, len(records))
	for _, record := range records {
		result[strings.ToLower(record.Key)] = record
	}
	return result
}

// resourceReplacementPrefix creates a request-local opaque replacement token prefix.
func resourceReplacementPrefix() (string, error) {
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return "", err
	}
	return "kumbukapluginvalue" + hex.EncodeToString(nonce[:]) + "n", nil
}

// expandResourceSource expands substitutions outside fenced code and records metadata.
func (m resourceSubstitutionModule) expandResourceSource(ctx plugin.Context, source string, records map[string]plugin.ResourceRecord, prefix string) (plugin.PreparedContent, map[string]int, error) {
	prepared := plugin.PreparedContent{Markdown: source}
	used := make(map[string]int)
	lines := strings.Split(source, "\n")
	fence := ""
	for index, line := range lines {
		if next, skip := nextResourceFence(line, fence); skip {
			fence = next
			continue
		}
		expanded, err := m.expandLine(ctx, line, records, prefix, used, &prepared)
		if err != nil {
			return plugin.PreparedContent{}, nil, err
		}
		lines[index] = expanded
	}
	prepared.Markdown = strings.Join(lines, "\n")
	return prepared, used, nil
}

// nextResourceFence updates fenced-code state and reports whether the current line must be skipped.
func nextResourceFence(line, fence string) (string, bool) {
	if fence != "" {
		if pluginmarkdown.Closes(line, fence) {
			return "", true
		}
		return fence, true
	}
	marker := pluginmarkdown.Fence(line)
	if marker != "" {
		return marker, true
	}
	return "", false
}

// validateUsedExportOverrides rejects overrides for values not referenced by the page.
func validateUsedExportOverrides(pluginID, moduleID string, overrides map[string]string, used map[string]int) error {
	for key := range overrides {
		if _, ok := used[strings.ToLower(key)]; !ok {
			return &plugin.ParameterError{
				PluginID: pluginID, ModuleID: moduleID, Key: key,
				Message: "Only values used by this page can be overridden.",
			}
		}
	}
	return nil
}

// exportOverrides returns request-local overrides for one plugin module.
func exportOverrides(ctx plugin.Context, pluginID, moduleID string) map[string]string {
	if byPlugin := ctx.ExportParameters[pluginID]; byPlugin != nil {
		return byPlugin[moduleID]
	}
	return nil
}

// unusedExportParameter returns a deterministic error for a module with no used values.
func unusedExportParameter(pluginID, moduleID string, overrides map[string]string) error {
	for key := range overrides {
		return &plugin.ParameterError{PluginID: pluginID, ModuleID: moduleID, Key: key, Message: "Only values used by this page can be overridden."}
	}
	return nil
}

// expandLine replaces every valid module macro in one non-code source line.
func (m resourceSubstitutionModule) expandLine(
	ctx plugin.Context,
	line string,
	records map[string]plugin.ResourceRecord,
	tokenPrefix string,
	used map[string]int,
	prepared *plugin.PreparedContent,
) (string, error) {
	opening := "{{" + m.module.Prefix + ":"
	var output strings.Builder
	for {
		start := strings.Index(line, opening)
		if start < 0 {
			output.WriteString(line)
			break
		}
		output.WriteString(line[:start])
		body := line[start+len(opening):]
		close := strings.Index(body, "}}")
		if close < 0 {
			output.WriteString(line[start:])
			break
		}
		name := strings.TrimSpace(body[:close])
		if !validResourceMacroName(name) {
			output.WriteString(line[start : start+len(opening)+close+2])
			line = body[close+2:]
			continue
		}

		position, err := m.resolveReplacement(ctx, name, records, tokenPrefix, used, prepared)
		if err != nil {
			return "", err
		}
		output.WriteString(prepared.Replacements[position].Token)
		m.recordReplacementOccurrence(position, prepared)
		line = body[close+2:]
	}
	return output.String(), nil
}

// validResourceMacroName reports whether a parsed substitution name is safe to resolve.
func validResourceMacroName(name string) bool {
	return name != "" && !strings.ContainsAny(name, "{}\r\n")
}

// resolveReplacement returns the stable replacement position, creating its metadata on first use.
func (m resourceSubstitutionModule) resolveReplacement(
	ctx plugin.Context,
	name string,
	records map[string]plugin.ResourceRecord,
	tokenPrefix string,
	used map[string]int,
	prepared *plugin.PreparedContent,
) (int, error) {
	record, ok := records[strings.ToLower(name)]
	if !ok {
		return 0, fmt.Errorf("%s %q not found", m.module.Prefix, name)
	}
	canonical := record.Key
	key := strings.ToLower(canonical)
	if position, found := used[key]; found {
		return position, nil
	}

	position := len(used)
	used[key] = position
	value := record.Values[m.module.ValueField]
	if m.module.Export {
		if override, ok := exportOverride(ctx, m.owner, m.module.ID, canonical); ok {
			value = override
		}
	}
	annotation := ""
	if m.module.Inspect {
		annotation = annotationID(m.owner, m.module.ID, canonical)
	}
	prepared.Replacements = append(prepared.Replacements, plugin.Replacement{
		Token: tokenPrefix + strconv.Itoa(position) + "end", Value: value, Annotation: annotation,
	})
	m.appendReplacementMetadata(record, canonical, annotation, prepared)
	return position, nil
}

// appendReplacementMetadata adds inspector and export metadata for one newly used resource value.
func (m resourceSubstitutionModule) appendReplacementMetadata(
	record plugin.ResourceRecord,
	canonical, annotation string,
	prepared *plugin.PreparedContent,
) {
	label := canonical
	if m.module.LabelField != "" && record.Values[m.module.LabelField] != "" {
		label = record.Values[m.module.LabelField]
	}
	detail := ""
	if m.module.DetailField != "" {
		detail = record.Values[m.module.DetailField]
	}
	if m.module.Inspect {
		if len(prepared.Inspectors) == 0 {
			prepared.Inspectors = append(prepared.Inspectors, plugin.Inspector{ID: m.module.ID, PluginID: m.owner, Name: m.resource.Name})
		}
		prepared.Inspectors[0].Items = append(prepared.Inspectors[0].Items, plugin.InspectorItem{
			Key: canonical, Label: label, Value: record.Values[m.module.ValueField], Description: detail, Annotation: annotation,
		})
	}
	if m.module.Export {
		prepared.ExportFields = append(prepared.ExportFields, plugin.ExportField{
			PluginID: m.owner, ModuleID: m.module.ID, Key: canonical, Label: label,
			Value: record.Values[m.module.ValueField], Description: detail,
		})
	}
}

// recordReplacementOccurrence increments inspector occurrence metadata when inspection is enabled.
func (m resourceSubstitutionModule) recordReplacementOccurrence(position int, prepared *plugin.PreparedContent) {
	if m.module.Inspect && len(prepared.Inspectors) != 0 {
		prepared.Inspectors[0].Items[position].Occurrences++
	}
}

// exportOverride finds a request override using the resource's case-insensitive key semantics.
func exportOverride(ctx plugin.Context, pluginID, moduleID, canonical string) (string, bool) {
	for key, value := range exportOverrides(ctx, pluginID, moduleID) {
		if strings.EqualFold(key, canonical) {
			return value, true
		}
	}
	return "", false
}

// annotationID returns a sanitizer-safe stable identifier for one resource item.
func annotationID(owner, moduleID, key string) string {
	digest := sha256.Sum256([]byte(owner + "\x00" + moduleID + "\x00" + strings.ToLower(key)))
	return "a-" + hex.EncodeToString(digest[:16])
}
