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
	rendererModule
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
		Stage:      "content-preprocess",
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
	owner    string
	module   pluginpackage.Module
	resource pluginpackage.Module
	storage  plugin.Storage
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
	if m.storage == nil {
		return plugin.PreparedContent{Markdown: source}, nil
	}
	overrides := exportOverrides(ctx, m.owner, m.module.ID)
	if !strings.Contains(source, "{{"+m.module.Prefix+":") && len(overrides) == 0 {
		return plugin.PreparedContent{Markdown: source}, nil
	}
	execution := ctx.Context
	if execution == nil {
		execution = context.Background()
	}
	records, err := plugin.ReadResourceRecords(execution, m.storage, m.owner, m.resource)
	if err != nil {
		return plugin.PreparedContent{}, err
	}
	if len(records) == 0 {
		if len(overrides) != 0 {
			return plugin.PreparedContent{}, unusedExportParameter(m.owner, m.module.ID, overrides)
		}
		return plugin.PreparedContent{Markdown: source}, nil
	}
	byName := make(map[string]plugin.ResourceRecord, len(records))
	for _, record := range records {
		byName[strings.ToLower(record.Key)] = record
	}

	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return plugin.PreparedContent{}, err
	}
	prefix := "kumbukapluginvalue" + hex.EncodeToString(nonce[:]) + "n"
	prepared := plugin.PreparedContent{Markdown: source}
	used := make(map[string]int)
	lines := strings.Split(source, "\n")
	fence := ""
	for index, line := range lines {
		marker := pluginmarkdown.Fence(line)
		if fence != "" {
			if pluginmarkdown.Closes(line, fence) {
				fence = ""
			}
			continue
		}
		if marker != "" {
			fence = marker
			continue
		}
		expanded, err := m.expandLine(ctx, line, byName, prefix, used, &prepared)
		if err != nil {
			return plugin.PreparedContent{}, err
		}
		lines[index] = expanded
	}
	prepared.Markdown = strings.Join(lines, "\n")
	for key := range overrides {
		if _, ok := used[strings.ToLower(key)]; !ok {
			return plugin.PreparedContent{}, &plugin.ParameterError{
				PluginID: m.owner, ModuleID: m.module.ID, Key: key,
				Message: "Only values used by this page can be overridden.",
			}
		}
	}
	return prepared, nil
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
		if name == "" || strings.ContainsAny(name, "{}\r\n") {
			output.WriteString(line[start : start+len(opening)+close+2])
			line = body[close+2:]
			continue
		}
		record, ok := records[strings.ToLower(name)]
		if !ok {
			return "", fmt.Errorf("%s %q not found", m.module.Prefix, name)
		}
		canonical := record.Key
		position, found := used[strings.ToLower(canonical)]
		if !found {
			position = len(used)
			used[strings.ToLower(canonical)] = position
			value := record.Values[m.module.ValueField]
			if m.module.Export {
				if override, ok := exportOverride(ctx, m.owner, m.module.ID, canonical); ok {
					value = override
				}
			}
			token := tokenPrefix + strconv.Itoa(position) + "end"
			annotation := ""
			if m.module.Inspect {
				annotation = annotationID(m.owner, m.module.ID, canonical)
			}
			prepared.Replacements = append(prepared.Replacements, plugin.Replacement{Token: token, Value: value, Annotation: annotation})
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
				prepared.Inspectors[0].Items = append(prepared.Inspectors[0].Items, plugin.InspectorItem{Key: canonical, Label: label, Value: record.Values[m.module.ValueField], Description: detail, Annotation: annotation})
			}
			if m.module.Export {
				prepared.ExportFields = append(prepared.ExportFields, plugin.ExportField{PluginID: m.owner, ModuleID: m.module.ID, Key: canonical, Label: label, Value: record.Values[m.module.ValueField], Description: detail})
			}
		}
		token := prepared.Replacements[position].Token
		output.WriteString(token)
		if m.module.Inspect && len(prepared.Inspectors) != 0 {
			prepared.Inspectors[0].Items[position].Occurrences++
		}
		line = body[close+2:]
	}
	return output.String(), nil
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
