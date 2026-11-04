package plugin

import (
	"errors"
	"io/fs"

	"github.com/kumbuka-me/sdk/editorlanguage"
	"github.com/kumbuka-me/sdk/pluginpackage"
)

// EditorCodeLanguage is a plugin-contributed fenced-code language for the editor.
type EditorCodeLanguage struct {
	// PluginID identifies the plugin that owns this language.
	PluginID string `json:"plugin_id"`
	// Value is the fenced-code info-string identifier.
	Value string `json:"value"`
	// Label is the name presented in the editor language picker.
	Label string `json:"label"`
	// Aliases are plugin-owned alternate fence names.
	Aliases []string `json:"aliases,omitempty"`
}

// editorCodeLanguagesFromPackage reads the SDK-validated plugin language catalog.
func editorCodeLanguagesFromPackage(pkg *pluginpackage.Package) ([]EditorCodeLanguage, string) {
	data, err := pkg.Asset(editorlanguage.AssetName)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, ""
	}
	if err != nil {
		return nil, "code-language catalog could not be read"
	}
	contract, err := editorlanguage.Parse(data)
	if err != nil {
		return nil, err.Error()
	}
	languages := make([]EditorCodeLanguage, 0, len(contract.Languages))
	for _, language := range contract.Languages {
		languages = append(languages, EditorCodeLanguage{
			PluginID: pkg.Manifest().ID,
			Value:    language.Value,
			Label:    language.Label,
			Aliases:  append([]string(nil), language.Aliases...),
		})
	}
	return languages, ""
}

// EditorCodeLanguages returns languages contributed by enabled plugins in deterministic order.
func (m *Manager) EditorCodeLanguages() []EditorCodeLanguage {
	m.mu.Lock()
	defer m.mu.Unlock()
	var languages []EditorCodeLanguage
	seen := make(map[string]bool)
	for _, id := range m.order {
		item, ok := m.loaded[id]
		if !ok || !item.metadata.Enabled || item.codeLanguageProblem != "" {
			continue
		}
		for _, language := range item.editorCodeLanguages {
			if seen[language.Value] {
				continue
			}
			seen[language.Value] = true
			languages = append(languages, language)
		}
	}
	return languages
}
