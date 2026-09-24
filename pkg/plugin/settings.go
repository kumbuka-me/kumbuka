package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"strings"

	"github.com/kumbuka-me/sdk/pluginpackage"
)

const (
	featureSettingKeyPrefix = "feature:"
	settingGroupKeyPrefix   = "setting:"
	maxSettingGroupBytes    = 60 << 10
)

// SettingGroup contains one administrator-managed typed settings group.
type SettingGroup struct {
	// Module contains the validated settings module declaration.
	Module pluginpackage.Module
	// Values contains administrator-visible values keyed by field ID.
	Values map[string]string
	// SecretFields reports which secret fields already contain a persisted value.
	SecretFields map[string]bool
}

// loadSettings returns persisted boolean feature settings for every untyped settings module in manifest. Feature settings default to enabled when no value has been stored yet.
func (m *Manager) loadSettings(ctx context.Context, manifest pluginpackage.Manifest) (map[string]bool, error) {
	settings := make(map[string]bool)

	for _, module := range manifest.Modules {
		if ModuleType(module.Type) != ModuleTypeSettings || len(module.Fields) != 0 {
			continue
		}

		settings[module.ID] = true
		if m.values == nil {
			continue
		}

		value, found, err := m.values.ReadPluginValue(ctx, manifest.ID, StorageNamespaceSettings, featureSettingStorageKey(module.ID))
		if err != nil {
			return nil, fmt.Errorf("read plugin setting %s.%s: %w", manifest.ID, module.ID, err)
		}
		if !found {
			continue
		}

		var enabled bool
		if err := json.Unmarshal(value, &enabled); err != nil {
			return nil, fmt.Errorf("decode plugin setting %s.%s: %w", manifest.ID, module.ID, err)
		}
		settings[module.ID] = enabled
	}

	return settings, nil
}

// UpdateSettings persists the complete boolean feature-setting state for one plugin.
func (m *Manager) UpdateSettings(ctx context.Context, id string, settings map[string]bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	item, err := m.find(id)
	if err != nil {
		return err
	}
	declared := declaredFeatureSettings(item.metadata.Manifest)
	if err := validateFeatureSettings(declared, settings); err != nil {
		return err
	}
	if err := validateFeatureSettingDependencies(declared, settings); err != nil {
		return err
	}
	if err := m.persistFeatureSettings(ctx, id, settings); err != nil {
		return err
	}

	item.metadata.Settings = maps.Clone(settings)
	m.loaded[id] = item
	return nil
}

// declaredFeatureSettings returns untyped settings modules keyed by their manifest identifier.
func declaredFeatureSettings(manifest pluginpackage.Manifest) map[string]pluginpackage.Module {
	declared := make(map[string]pluginpackage.Module)
	for _, module := range manifest.Modules {
		if ModuleType(module.Type) == ModuleTypeSettings && len(module.Fields) == 0 {
			declared[module.ID] = module
		}
	}
	return declared
}

// validateFeatureSettings requires a complete state containing only declared settings.
func validateFeatureSettings(declared map[string]pluginpackage.Module, settings map[string]bool) error {
	if len(declared) == 0 {
		return errors.New("plugin does not expose feature settings")
	}
	if len(settings) != len(declared) {
		return errors.New("plugin settings are incomplete")
	}
	for key := range settings {
		if _, ok := declared[key]; !ok {
			return fmt.Errorf("unknown plugin setting %q", key)
		}
	}
	return nil
}

// validateFeatureSettingDependencies rejects enabled settings whose declared feature dependencies are disabled.
func validateFeatureSettingDependencies(declared map[string]pluginpackage.Module, settings map[string]bool) error {
	for id, module := range declared {
		if !settings[id] {
			continue
		}
		for _, dependency := range module.Requires {
			required, ok := declared[dependency]
			if ok && !settings[required.ID] {
				return fmt.Errorf("%s requires %s to be enabled", module.Name, required.Name)
			}
		}
	}
	return nil
}

// persistFeatureSettings stores every feature value while the manager lock keeps the operation atomic to callers.
func (m *Manager) persistFeatureSettings(ctx context.Context, pluginID string, settings map[string]bool) error {
	if m.values == nil {
		return nil
	}
	for key, enabled := range settings {
		value, err := json.Marshal(enabled)
		if err != nil {
			return err
		}
		if err := m.values.WritePluginValue(ctx, pluginID, StorageNamespaceSettings, featureSettingStorageKey(key), value); err != nil {
			return fmt.Errorf("save plugin setting %s.%s: %w", pluginID, key, err)
		}
	}
	return nil
}

// SettingGroups returns administrator-safe typed settings groups for one installed plugin.
func (m *Manager) SettingGroups(ctx context.Context, pluginID string) ([]SettingGroup, error) {
	modules, err := m.settingGroupModules(pluginID)
	if err != nil {
		return nil, err
	}

	groups := make([]SettingGroup, 0, len(modules))
	for _, module := range modules {
		values, found, err := readSettingGroup(ctx, m.values, pluginID, module)
		if err != nil {
			return nil, err
		}
		values = settingGroupDefaults(module, values, found)

		secrets := make(map[string]bool)
		for _, field := range module.Fields {
			if ConfigurationFieldType(field.Type) != ConfigurationFieldSecret {
				continue
			}
			secrets[field.ID] = found && values[field.ID] != ""
			values[field.ID] = ""
		}

		groups = append(groups, SettingGroup{Module: module, Values: values, SecretFields: secrets})
	}

	return groups, nil
}

// SaveSettingGroup validates and atomically persists one complete typed settings group.
func (m *Manager) SaveSettingGroup(ctx context.Context, pluginID, moduleID string, values map[string]string) error {
	module, err := m.settingGroupModule(pluginID, moduleID)
	if err != nil {
		return err
	}
	if m.values == nil {
		return errors.New("plugin settings storage is unavailable")
	}

	previous, _, err := readSettingGroup(ctx, m.values, pluginID, module)
	if err != nil {
		return err
	}
	validated, err := normalizeSettingGroup(module, values, previous)
	if err != nil {
		return err
	}
	if err := m.encryptConfigurationSecrets(module, validated, previous); err != nil {
		return err
	}

	encoded, err := json.Marshal(validated)
	if err != nil {
		return err
	}
	if len(encoded) > maxSettingGroupBytes {
		return errors.New("plugin settings group is too large")
	}

	return m.values.WritePluginValue(ctx, pluginID, StorageNamespaceSettings, settingGroupStorageKey(module.ID), encoded)
}

// ReadDeclaredSetting resolves one manifest-declared typed setting, applying defaults and decrypting secrets.
func ReadDeclaredSetting(
	ctx context.Context,
	storage Storage,
	pluginID string,
	manifest pluginpackage.Manifest,
	key string,
	codec SecretCodec,
) ([]byte, bool, error) {
	module, field, declared := declaredSettingField(manifest, key)
	if !declared {
		return nil, false, nil
	}

	values, found, err := readSettingGroup(ctx, storage, pluginID, module)
	if err != nil {
		return nil, true, err
	}
	value := field.Default
	if found {
		if stored, ok := values[field.ID]; ok {
			value = stored
		}
	}
	if ConfigurationFieldType(field.Type) == ConfigurationFieldSecret && value != "" {
		if codec == nil || !codec.Configured() {
			return nil, true, ErrSecretEncryptionUnavailable
		}
		value, err = codec.Decrypt(value)
		if err != nil {
			return nil, true, fmt.Errorf("decrypt plugin setting %s.%s: %w", module.ID, field.ID, err)
		}
	}

	return []byte(value), true, nil
}

// DeclaredSetting reports whether key names one manifest-declared typed setting.
func DeclaredSetting(manifest pluginpackage.Manifest, key string) bool {
	_, _, ok := declaredSettingField(manifest, key)
	return ok
}

// ReservedSettingStorageKey reports whether a raw settings key belongs to host-managed configuration.
func ReservedSettingStorageKey(key string) bool {
	return strings.HasPrefix(key, featureSettingKeyPrefix) ||
		strings.HasPrefix(key, settingGroupKeyPrefix) ||
		strings.HasPrefix(key, "r:")
}

// FeatureSettings returns request feature flags contributed by enabled plugins.
func (m *Manager) FeatureSettings() map[string]bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	features := make(map[string]bool)
	for _, id := range m.order {
		item, ok := m.loaded[id]
		if !ok || !item.metadata.Enabled {
			continue
		}
		for key, enabled := range item.metadata.Settings {
			features[id+"."+key] = enabled
		}
	}

	return features
}

// settingGroupModules returns typed settings modules from one installed plugin in manifest order.
func (m *Manager) settingGroupModules(pluginID string) ([]pluginpackage.Module, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	item, ok := m.loaded[pluginID]
	if !ok {
		return nil, errors.New("plugin is not installed")
	}

	modules := make([]pluginpackage.Module, 0)
	for _, module := range item.metadata.Manifest.Modules {
		if ModuleType(module.Type) == ModuleTypeSettings && len(module.Fields) != 0 {
			modules = append(modules, module)
		}
	}
	return modules, nil
}

// settingGroupModule returns one typed settings module from an installed plugin.
func (m *Manager) settingGroupModule(pluginID, moduleID string) (pluginpackage.Module, error) {
	modules, err := m.settingGroupModules(pluginID)
	if err != nil {
		return pluginpackage.Module{}, err
	}
	for _, module := range modules {
		if module.ID == moduleID {
			return module, nil
		}
	}
	return pluginpackage.Module{}, errors.New("plugin settings group is not declared")
}

// readSettingGroup decodes one complete typed settings group from trusted plugin storage.
func readSettingGroup(ctx context.Context, storage Storage, pluginID string, module pluginpackage.Module) (map[string]string, bool, error) {
	if storage == nil {
		return nil, false, nil
	}

	data, found, err := storage.ReadPluginValue(ctx, pluginID, StorageNamespaceSettings, settingGroupStorageKey(module.ID))
	if err != nil || !found {
		return nil, found, err
	}

	var values map[string]string
	if err := json.Unmarshal(data, &values); err != nil {
		return nil, true, fmt.Errorf("decode plugin settings %s.%s: %w", pluginID, module.ID, err)
	}
	return values, true, nil
}

// settingGroupDefaults fills absent typed fields from their manifest defaults.
func settingGroupDefaults(module pluginpackage.Module, stored map[string]string, found bool) map[string]string {
	values := make(map[string]string, len(module.Fields))
	for _, field := range module.Fields {
		values[field.ID] = field.Default
	}
	if found {
		for key, value := range stored {
			values[key] = value
		}
	}
	return values
}

// normalizeSettingGroup validates a complete typed settings submission against its manifest schema.
func normalizeSettingGroup(module pluginpackage.Module, values, previous map[string]string) (map[string]string, error) {
	declared := make(map[string]pluginpackage.ConfigurationField, len(module.Fields))
	for _, field := range module.Fields {
		declared[field.ID] = field
	}
	if len(values) != len(declared) {
		return nil, errors.New("plugin settings are incomplete")
	}
	for field := range values {
		if _, ok := declared[field]; !ok {
			return nil, fmt.Errorf("unknown plugin setting field %q", field)
		}
	}

	result := make(map[string]string, len(module.Fields))
	for _, field := range module.Fields {
		normalized, err := normalizeConfigurationValue(field, values[field.ID])
		if err != nil {
			return nil, err
		}
		if requiredConfigurationValueMissing(field, normalized, previous) {
			return nil, configurationFieldError(field, field.Name+" is required.")
		}
		result[field.ID] = normalized
	}
	return result, nil
}

// declaredSettingField resolves a logical <module>.<field> key to its manifest declaration.
func declaredSettingField(manifest pluginpackage.Manifest, key string) (pluginpackage.Module, pluginpackage.ConfigurationField, bool) {
	moduleID, fieldID, ok := strings.Cut(key, ".")
	if !validDeclaredSettingKey(moduleID, fieldID, ok) {
		return pluginpackage.Module{}, pluginpackage.ConfigurationField{}, false
	}
	for _, module := range manifest.Modules {
		if !matchingSettingsModule(module, moduleID) {
			continue
		}
		for _, field := range module.Fields {
			if field.ID == fieldID {
				return module, field, true
			}
		}
	}
	return pluginpackage.Module{}, pluginpackage.ConfigurationField{}, false
}

// validDeclaredSettingKey reports whether a logical setting key contains non-empty module and field identifiers.
func validDeclaredSettingKey(moduleID, fieldID string, separated bool) bool {
	return separated && moduleID != "" && fieldID != ""
}

// matchingSettingsModule reports whether module is the requested non-empty settings group.
func matchingSettingsModule(module pluginpackage.Module, moduleID string) bool {
	return ModuleType(module.Type) == ModuleTypeSettings && len(module.Fields) != 0 && module.ID == moduleID
}

// featureSettingStorageKey isolates host-managed feature toggles from plugin-owned settings and resources.
func featureSettingStorageKey(id string) string {
	return featureSettingKeyPrefix + id
}

// settingGroupStorageKey isolates host-managed typed settings groups from plugin-owned values.
func settingGroupStorageKey(moduleID string) string {
	return settingGroupKeyPrefix + moduleID
}
