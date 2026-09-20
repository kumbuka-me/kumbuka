package webview

import (
	"encoding/json"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/kumbuka-me/kumbuka/pkg/themes"
	"html/template"
)

// PublicData builds shared data for unauthenticated setup and login pages.
func (v *Views) PublicData(title string) (Layout, error) {
	preferences := domain.DefaultUserPreferences()
	activeTheme := themes.DefaultTheme
	preferences.Theme = activeTheme
	themeData, err := json.Marshal(v.themes)
	if err != nil {
		return Layout{}, err
	}

	return Layout{
		Title:         title,
		Preferences:   preferences,
		Version:       v.version,
		AssetVersion:  v.assetVersion,
		Commit:        v.commit,
		Runtime:       v.runtime,
		ThemeData:     template.JS(themeData),
		PluginModules: template.JS("[]"),
		Themes:        v.themes,
		ActiveTheme:   activeTheme,
	}, nil
}
