package plugin

import (
	"slices"
	"sort"

	"github.com/kumbuka-me/kumbuka/pkg/domain"
)

var editorToolbarGroups = []struct{ ID, Label string }{
	{ID: "text", Label: "Text formatting"},
	{ID: "blocks", Label: "Blocks"},
	{ID: "insert", Label: "Insert"},
	{ID: "tools", Label: "Tools"},
	{ID: "plugins", Label: "Plugins"},
}

// ToolbarContribution is one resolved direct action or plugin-owned submenu.
type ToolbarContribution struct {
	// ID is the stable plugin ID and module ID pair.
	ID string `json:"id"`
	// PluginID identifies the owning plugin.
	PluginID string `json:"plugin_id"`
	// PluginName is the human-readable owning plugin name.
	PluginName string `json:"plugin_name"`
	// ModuleID identifies the manifest contribution.
	ModuleID string `json:"module_id"`
	// Name and Description label the contribution.
	Name string `json:"name"`
	// Description explains the contribution.
	Description string `json:"description,omitempty"`
	// Icon is the validated host icon or the safe fallback.
	Icon string `json:"icon"`
	// Group is the resolved host-owned toolbar group.
	Group string `json:"group"`
	// DefaultGroup is the plugin's preferred host group.
	DefaultGroup string `json:"default_group"`
	// AllowedGroups are the administrator placement choices.
	AllowedGroups []string `json:"allowed_groups"`
	// Order is the resolved ordering value.
	Order int `json:"order"`
	// DefaultOrder is the plugin's ordering hint.
	DefaultOrder int `json:"default_order"`
	// Hidden reports whether an administrator hid the contribution.
	Hidden bool `json:"hidden"`
	// Overridden reports whether persisted settings differ from plugin defaults.
	Overridden bool `json:"overridden"`
	// Action contains a direct editor insertion action.
	Action *EditorInsertContribution `json:"action,omitempty"`
	// Children contains plugin-owned submenu actions in manifest order.
	Children []EditorInsertContribution `json:"children,omitempty"`
}

// ToolbarGroup is one host-owned semantic slot with resolved plugin contributions.
type ToolbarGroup struct {
	// ID is the stable semantic group identifier.
	ID string `json:"id"`
	// Label is the accessible group name.
	Label string `json:"label"`
	// Contributions are visible resolved contributions in final order.
	Contributions []ToolbarContribution `json:"contributions"`
}

// ResolveEditorToolbar applies valid global overrides and deterministic ordering to active contributions.
func (m *Manager) ResolveEditorToolbar(overrides []domain.EditorToolbarOverride) []ToolbarGroup {
	m.mu.Lock()
	defer m.mu.Unlock()

	overrideByID := make(map[string]domain.EditorToolbarOverride, len(overrides))
	for _, override := range overrides {
		overrideByID[override.ID] = override
	}
	groups := make([]ToolbarGroup, len(editorToolbarGroups))
	for index, group := range editorToolbarGroups {
		groups[index] = ToolbarGroup{ID: group.ID, Label: group.Label, Contributions: []ToolbarContribution{}}
	}

	for pluginID, item := range m.loaded {
		if !item.metadata.Enabled {
			continue
		}
		actions := make(map[string]EditorInsertContribution)
		menuChildren := map[string]bool{}
		for _, module := range item.metadata.Manifest.Modules {
			if ModuleType(module.Type) == ModuleTypeEditorMenu {
				for _, child := range module.Children {
					menuChildren[child] = true
				}
				continue
			}
			if ModuleType(module.Type) == ModuleTypeEditorInsert {
				actions[module.ID] = editorInsertView(pluginID, module)
			}
		}
		for _, module := range item.metadata.Manifest.Modules {
			if ModuleType(module.Type) == ModuleTypeEditorInsert && !menuChildren[module.ID] {
				action := actions[module.ID]
				contribution := toolbarContribution(pluginID, item.metadata.Manifest.Name, module.ID, module.Name, module.Description, module.Icon, module.Group, module.AllowedGroups, module.Order, overrideByID[pluginID+":"+module.ID])
				contribution.Action = &action
				appendToolbarContribution(groups, contribution)
			}
			if ModuleType(module.Type) == ModuleTypeEditorMenu {
				contribution := toolbarContribution(pluginID, item.metadata.Manifest.Name, module.ID, module.Name, module.Description, module.Icon, module.Group, module.AllowedGroups, module.Order, overrideByID[pluginID+":"+module.ID])
				for _, child := range module.Children {
					contribution.Children = append(contribution.Children, actions[child])
				}
				appendToolbarContribution(groups, contribution)
			}
		}
	}
	for index := range groups {
		sort.Slice(groups[index].Contributions, func(i, j int) bool {
			left, right := groups[index].Contributions[i], groups[index].Contributions[j]
			if left.Order != right.Order {
				return left.Order < right.Order
			}
			return left.ID < right.ID
		})
	}
	return groups
}

// toolbarContribution resolves one contribution's defaults and valid administrator override.
func toolbarContribution(pluginID, pluginName, moduleID, name, description, icon, group string, allowed []string, order int, override domain.EditorToolbarOverride) ToolbarContribution {
	if group == "" {
		group = "insert"
	}
	if len(allowed) == 0 {
		allowed = []string{group}
	}
	if icon == "" {
		icon = "braces-lucide"
	}
	result := ToolbarContribution{ID: pluginID + ":" + moduleID, PluginID: pluginID, PluginName: pluginName, ModuleID: moduleID, Name: name, Description: description, Icon: icon, Group: group, DefaultGroup: group, AllowedGroups: slices.Clone(allowed), Order: order, DefaultOrder: order}
	if override.ID == result.ID {
		result.Hidden = override.Hidden
		if override.Group != "" && slices.Contains(allowed, override.Group) {
			result.Group = override.Group
		}
		result.Order = override.Order
		result.Overridden = result.Hidden || result.Group != result.DefaultGroup || result.Order != result.DefaultOrder
	}
	return result
}

// appendToolbarContribution adds one visible contribution to its resolved host group.
func appendToolbarContribution(groups []ToolbarGroup, contribution ToolbarContribution) {
	for index := range groups {
		if groups[index].ID == contribution.Group {
			groups[index].Contributions = append(groups[index].Contributions, contribution)
			return
		}
	}
}
