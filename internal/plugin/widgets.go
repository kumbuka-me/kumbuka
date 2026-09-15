package plugin

// WidgetKey returns the stable user-preference key for one plugin widget module.
func WidgetKey(pluginID, moduleID string) string {
	return pluginID + "/" + moduleID
}
