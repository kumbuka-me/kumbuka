package webview

import (
	"encoding/json"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/kumbuka-me/kumbuka/pkg/pluginbrowser"
	"github.com/kumbuka-me/kumbuka/pkg/themes"
	"html/template"
)

// PublicData builds shared data for unauthenticated setup and login pages.
func (v *Views) PublicData(title string) (Data, error) {
	preferences := domain.DefaultUserPreferences()
	activeTheme := themes.DefaultTheme
	preferences.Theme = activeTheme
	themeData, err := json.Marshal(v.themes)
	if err != nil {
		return Data{}, err
	}

	return Data{
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

// PublicPluginData builds unauthenticated view data with active browser plugin presentation assets.
func (v *Views) PublicPluginData(title string, modules []pluginbrowser.Module, stylesVersion string) (Data, error) {
	data, err := v.PublicData(title)
	if err != nil {
		return Data{}, err
	}

	encoded, err := json.Marshal(modules)
	if err != nil {
		return Data{}, err
	}
	data.PluginModules = template.JS(encoded)
	data.PluginStylesVersion = stylesVersion

	return data, nil
}
