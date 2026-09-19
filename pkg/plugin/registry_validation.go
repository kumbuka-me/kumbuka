package plugin

import "fmt"

// registrationState summarizes active contribution names needed to validate a new plugin.
type registrationState struct {
	// activePlugins contains the IDs of all currently registered plugins.
	activePlugins map[string]struct{}
	// macroNames contains macro names already owned by active plugins.
	macroNames map[string]struct{}
	// codeHighlighterOwner identifies the active plugin providing the exclusive highlighter.
	codeHighlighterOwner string
}

// contributionIDValidator tracks IDs already seen within each contribution kind.
type contributionIDValidator struct {
	// seen contains kind-qualified contribution IDs already validated in this registration.
	seen map[string]struct{}
}

// recoverRegistrationPanic converts plugin callback panics during registration into ordinary errors.
func recoverRegistrationPanic(result *error, pluginID string) {
	if recovered := recover(); recovered != nil {
		*result = fmt.Errorf("plugin %s panicked during registration", pluginID)
	}
}

// validateRegistration checks a complete plugin contribution set against the active registry state.
func validateRegistration(descriptor Descriptor, modules Contributions, entries []Entry) error {
	if err := validateDescriptor(descriptor); err != nil {
		return err
	}

	state := collectRegistrationState(entries)
	if err := validateDependencies(descriptor, state); err != nil {
		return err
	}
	if err := validateMacros(descriptor.ID, modules.Macros, state.macroNames); err != nil {
		return err
	}
	if err := validateExecutableContributions(descriptor.ID, modules); err != nil {
		return err
	}
	if err := validateCodeHighlighter(descriptor.ID, modules.CodeHighlighters, state.codeHighlighterOwner); err != nil {
		return err
	}

	return validateContributionIDs(modules)
}

// validateDescriptor checks the required identity fields for one plugin descriptor.
func validateDescriptor(descriptor Descriptor) error {
	if !validID.MatchString(descriptor.ID) || descriptor.Name == "" {
		return fmt.Errorf("invalid plugin descriptor %q", descriptor.ID)
	}

	return nil
}

// collectRegistrationState builds the collision and dependency indexes for active plugins.
func collectRegistrationState(entries []Entry) registrationState {
	state := registrationState{
		activePlugins: make(map[string]struct{}, len(entries)),
		macroNames:    make(map[string]struct{}),
	}

	for _, entry := range entries {
		state.activePlugins[entry.Descriptor.ID] = struct{}{}
		if len(entry.Contributions.CodeHighlighters) != 0 {
			state.codeHighlighterOwner = entry.Descriptor.ID
		}
		for _, macro := range entry.Contributions.Macros {
			state.macroNames[macro.Name()] = struct{}{}
		}
	}

	return state
}

// validateDependencies checks plugin ownership and required active dependencies.
func validateDependencies(descriptor Descriptor, state registrationState) error {
	if _, exists := state.activePlugins[descriptor.ID]; exists {
		return fmt.Errorf("plugin %s is already registered", descriptor.ID)
	}

	for _, dependency := range descriptor.Requires {
		if _, active := state.activePlugins[dependency]; !active {
			return fmt.Errorf("plugin %s requires active plugin %s", descriptor.ID, dependency)
		}
	}

	return nil
}

// validateMacros checks macro callbacks, identifiers, and global name collisions.
func validateMacros(pluginID string, macros []Macro, activeNames map[string]struct{}) error {
	names := make(map[string]struct{}, len(activeNames)+len(macros))
	for name := range activeNames {
		names[name] = struct{}{}
	}

	for _, macro := range macros {
		if macro == nil || !validID.MatchString(macro.Name()) {
			return fmt.Errorf("invalid macro in plugin %s", pluginID)
		}

		name := macro.Name()
		if _, exists := names[name]; exists {
			return fmt.Errorf("macro %s is already registered", name)
		}

		names[name] = struct{}{}
	}

	return nil
}

// validateExecutableContributions rejects nil callbacks before metadata IDs are published.
func validateExecutableContributions(pluginID string, modules Contributions) error {
	for _, module := range modules.ContentPreprocessors {
		if module == nil {
			return fmt.Errorf("nil content preprocessor in %s", pluginID)
		}
	}
	for _, module := range modules.Preprocessors {
		if module == nil {
			return fmt.Errorf("nil preprocessor in %s", pluginID)
		}
	}
	for _, module := range modules.MarkdownExtensions {
		if module == nil {
			return fmt.Errorf("nil Markdown extension in %s", pluginID)
		}
	}
	for _, module := range modules.Postprocessors {
		if module == nil {
			return fmt.Errorf("nil postprocessor in %s", pluginID)
		}
	}

	return nil
}

// validateCodeHighlighter enforces the single-provider highlighter contract.
func validateCodeHighlighter(pluginID string, highlighters []CodeHighlighterModule, activeOwner string) error {
	if len(highlighters) > 1 {
		return fmt.Errorf("plugin %s contributes more than one code highlighter", pluginID)
	}
	if len(highlighters) == 0 {
		return nil
	}
	if highlighters[0].Highlighter == nil {
		return fmt.Errorf("nil code highlighter in %s", pluginID)
	}
	if activeOwner != "" {
		return fmt.Errorf("code highlighter is already provided by plugin %s", activeOwner)
	}

	return nil
}

// validateContributionIDs validates unique IDs and required callbacks for metadata contributions.
func validateContributionIDs(contributions Contributions) error {
	validator := contributionIDValidator{seen: make(map[string]struct{})}

	for _, module := range contributions.CodeHighlighters {
		if err := validator.check("code-highlighter", module.ID); err != nil {
			return err
		}
	}
	for _, module := range contributions.Widgets {
		if err := validator.check("widget", module.ID); err != nil {
			return err
		}
	}
	for _, module := range contributions.Exporters {
		if module.Exporter == nil {
			return fmt.Errorf("nil exporter %q", module.ID)
		}
		if err := validator.check("exporter", module.ID); err != nil {
			return err
		}
	}
	for _, module := range contributions.BrowserModules {
		if err := validator.check("browser", module.ID); err != nil {
			return err
		}
	}
	for _, module := range contributions.EditorExtensions {
		if err := validator.check("editor", module.ID); err != nil {
			return err
		}
	}
	for _, module := range contributions.AdminActions {
		if module.Action == nil {
			return fmt.Errorf("nil admin action %q", module.ID)
		}
		if err := validator.check("admin-action", module.ID); err != nil {
			return err
		}
	}
	for _, module := range contributions.AdminResources {
		if err := validator.check("admin-resource", module.ID); err != nil {
			return err
		}
	}
	for _, module := range contributions.EditorCompletions {
		if err := validator.check("editor-completion", module.ID); err != nil {
			return err
		}
	}
	for _, module := range contributions.EditorInserts {
		if err := validator.check("editor-insert", module.ID); err != nil {
			return err
		}
	}
	for _, module := range contributions.SettingsModules {
		if err := validator.check("settings", module.ID); err != nil {
			return err
		}
	}
	for _, module := range contributions.ContentStyles {
		if err := validator.check("content-style", module.ID); err != nil {
			return err
		}
	}
	for _, module := range contributions.RenderPolicies {
		if err := validator.check("render-policy", module.ID); err != nil {
			return err
		}
	}

	return nil
}

// check validates one kind-qualified contribution ID and records it as seen.
func (v *contributionIDValidator) check(kind, id string) error {
	key := kind + ":" + id
	if !validID.MatchString(id) {
		return fmt.Errorf("invalid or duplicate %s ID %q", kind, id)
	}
	if _, duplicate := v.seen[key]; duplicate {
		return fmt.Errorf("invalid or duplicate %s ID %q", kind, id)
	}

	v.seen[key] = struct{}{}
	return nil
}
