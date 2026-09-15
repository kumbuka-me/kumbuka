package markdown

import (
	"crypto/sha256"
	"encoding/hex"
	"slices"
	"strings"

	"github.com/kumbuka-me/kumbuka/internal/plugin"
	"github.com/kumbuka-me/kumbuka/internal/pluginusage"
	pluginmarkdown "github.com/kumbuka-me/sdk/markdown"
	"github.com/kumbuka-me/sdk/pluginpackage"
)

type usageSet map[string]bool

func usageKey(pluginID, moduleID string) string { return pluginID + "\x00" + moduleID }

// AnalyzeUsage derives rebuildable plugin usage metadata for persisted pages.
func (r *Renderer) AnalyzeUsage(source string) pluginusage.Index {
	plan, release := r.registry.AcquireRenderPlan()
	defer release()
	return analyzeUsage(source, plan)
}

func analyzeUsage(source string, plan *plugin.RenderPlan) pluginusage.Index {
	scanner := newUsageScanner(source)
	index := pluginusage.Index{
		Version:     pluginusage.Version,
		Fingerprint: plan.UsageFingerprint,
		SourceHash:  usageSourceHash(source),
	}
	for _, descriptor := range plan.SourceUsage {
		matched, values := scanner.match(descriptor.Usage.Rules)
		if !matched {
			continue
		}
		index.Modules = append(index.Modules, pluginusage.Module{
			PluginID: descriptor.PluginID,
			ModuleID: descriptor.Usage.ModuleID,
			Values:   values,
		})
	}
	return index
}

func usageSetFromIndex(index pluginusage.Index) usageSet {
	result := make(usageSet, len(index.Modules))
	for _, module := range index.Modules {
		result[usageKey(module.PluginID, module.ModuleID)] = true
	}
	return result
}

func currentUsageIndex(index *pluginusage.Index, plan *plugin.RenderPlan, source string) bool {
	if index == nil || index.Version != pluginusage.Version || index.SourceHash != usageSourceHash(source) {
		return false
	}
	return index.Fingerprint == plan.UsageFingerprint
}

func usageSourceHash(source string) string {
	digest := sha256.Sum256([]byte(source))
	return hex.EncodeToString(digest[:])
}

type usageScanner struct {
	source    string
	outside   []string
	languages []string
}

func newUsageScanner(source string) usageScanner {
	lines := strings.Split(source, "\n")
	scanner := usageScanner{source: source}
	fence := ""
	for _, line := range lines {
		if fence != "" {
			if pluginmarkdown.Closes(line, fence) {
				fence = ""
			}
			continue
		}
		marker := pluginmarkdown.Fence(line)
		if marker != "" {
			trimmed := strings.TrimSpace(line)
			info := strings.TrimSpace(trimmed[len(marker):])
			if fields := strings.Fields(info); len(fields) > 0 {
				scanner.languages = append(scanner.languages, strings.ToLower(fields[0]))
			} else {
				scanner.languages = append(scanner.languages, "")
			}
			fence = marker
			continue
		}
		scanner.outside = append(scanner.outside, line)
	}
	return scanner
}

func (s usageScanner) match(rules []plugin.SourceUsageRule) (bool, []string) {
	matched := false
	var values []string
	seen := map[string]bool{}
	add := func(value string) {
		if value == "" || seen[value] {
			return
		}
		seen[value] = true
		values = append(values, value)
	}
	for _, rule := range rules {
		switch {
		case rule.Contains != "":
			matched = matched || strings.Contains(s.source, rule.Contains)
		case rule.Fence != "":
			for _, language := range s.languages {
				if rule.Fence == "*" || strings.EqualFold(language, rule.Fence) {
					matched = true
					add(language)
				}
			}
		case rule.Macro != "":
			for _, line := range s.outside {
				if value, ok := macroUsage(line, rule.Macro); ok {
					matched = true
					add(value)
				}
			}
		case rule.Substitution != "":
			for _, line := range s.outside {
				for _, value := range substitutionUsage(line, rule.Substitution) {
					matched = true
					add(value)
				}
			}
		}
	}
	return matched, values
}

func macroUsage(line, name string) (string, bool) {
	line = strings.TrimSpace(line)
	opening := "{{" + name
	if !strings.HasPrefix(line, opening) || !strings.HasSuffix(line, "}}") {
		return "", false
	}
	rest := line[len(opening) : len(line)-2]
	if rest != "" && rest[0] != ' ' && rest[0] != '\t' {
		return "", false
	}
	return strings.TrimSpace(rest), true
}

func substitutionUsage(line, prefix string) []string {
	opening := "{{" + prefix + ":"
	var values []string
	for {
		start := strings.Index(line, opening)
		if start < 0 {
			return values
		}
		body := line[start+len(opening):]
		close := strings.Index(body, "}}")
		if close < 0 {
			return values
		}
		value := strings.TrimSpace(body[:close])
		if value != "" && !strings.ContainsAny(value, "{}\r\n") {
			values = append(values, value)
		}
		line = body[close+2:]
	}
}

func (p *renderPipeline) pagePlanForSource(source string) pageRenderPlan {
	if source == p.usageSource {
		return p.pagePlan
	}
	stop := p.trace.Measure("usage_analysis")
	index := analyzeUsage(source, p.plan)
	stop()
	stop = p.trace.Measure("page_plan")
	usage := usageSetFromIndex(index)
	page := newPageRenderPlan(p.plan, usage, p.exportParameters)
	stop()
	return page
}

func (p *renderPipeline) setUsageSource(source string) {
	if source == p.usageSource {
		return
	}
	p.usageSource = source
	stop := p.trace.Measure("usage_analysis")
	index := analyzeUsage(source, p.plan)
	stop()
	stop = p.trace.Measure("page_plan")
	usage := usageSetFromIndex(index)
	p.pagePlan = newPageRenderPlan(p.plan, usage, p.exportParameters)
	stop()
}

// RequiredPluginIDs returns declared plugin packages that can affect at least one source.
// Modules with no usage rules are intentionally treated as always active, matching the
// runtime selector semantics. Browser/admin/editor-only modules do not select a package.
func RequiredPluginIDs(sources []string, manifests []pluginpackage.Manifest) []string {
	scanners := make([]usageScanner, 0, len(sources))
	for _, source := range sources {
		scanners = append(scanners, newUsageScanner(source))
	}

	result := make([]string, 0, len(manifests))
	for _, manifest := range manifests {
		if manifestRequiredBySources(scanners, manifest) {
			result = append(result, manifest.ID)
		}
	}
	slices.Sort(result)
	return result
}

func manifestRequiredBySources(scanners []usageScanner, manifest pluginpackage.Manifest) bool {
	for _, module := range manifest.Modules {
		if !staticRenderModule(module.Type) {
			continue
		}
		if len(module.Usage) == 0 {
			return true
		}
		rules := make([]plugin.SourceUsageRule, 0, len(module.Usage))
		for _, rule := range module.Usage {
			rules = append(rules, plugin.SourceUsageRule{
				Contains:     rule.Contains,
				Fence:        rule.Fence,
				Macro:        rule.Macro,
				Substitution: rule.Substitution,
			})
		}
		for _, scanner := range scanners {
			matched, _ := scanner.match(rules)
			if matched {
				return true
			}
		}
	}
	return false
}

func staticRenderModule(moduleType string) bool {
	switch moduleType {
	case "markdown-syntax", "code-highlighter", "content-style", "render-policy", "renderer-extension", "macro", "content-substitution":
		return true
	default:
		return false
	}
}
