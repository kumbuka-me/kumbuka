package plugin

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"regexp"
	"strings"

	"github.com/kumbuka-me/sdk/pluginpackage"
)

const (
	editorWidgetAsset          = "visual-editor.json"
	maxEditorWidgetAssetBytes  = 64 << 10
	maxEditorWidgets           = 16
	maxEditorWidgetAttributes  = 32
	maxEditorWidgetSettings    = 32
	maxEditorWidgetConstraints = 16
)

var editorWidgetClass = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_-]{0,127}$`)

// EditorWidgetSyntax declares the Markdown construct recognized by one visual-editor widget.
type EditorWidgetSyntax struct {
	// Kind selects the supported Markdown mapping. Version 1 supports macro syntax.
	Kind string `json:"kind"`
	// Name is the macro name without surrounding braces.
	Name string `json:"name"`
	// Multiline allows a macro invocation to span multiple source lines.
	Multiline bool `json:"multiline,omitempty"`
}

// EditorWidgetAttribute declares one structured macro attribute and its validation rules.
type EditorWidgetAttribute struct {
	// Name identifies the Markdown attribute.
	Name string `json:"name"`
	// Type selects string, identifier, enum, list, or color-list validation.
	Type string `json:"type"`
	// Default is applied by the editor when the source omits this attribute.
	Default string `json:"default,omitempty"`
	// Required reports whether the attribute must contain a value.
	Required bool `json:"required,omitempty"`
	// MaxBytes bounds one scalar value or one list item when non-zero.
	MaxBytes int `json:"max_bytes,omitempty"`
	// MaxItems bounds list and color-list attributes when non-zero.
	MaxItems int `json:"max_items,omitempty"`
	// Values contains the allowed values for enum attributes.
	Values []string `json:"values,omitempty"`
	// Separator joins list values back into one Markdown attribute.
	Separator string `json:"separator,omitempty"`
	// FallbackSeparator is accepted while parsing legacy list values but is never emitted.
	FallbackSeparator string `json:"fallback_separator,omitempty"`
	// Unique reports whether list values must be distinct.
	Unique bool `json:"unique,omitempty"`
	// Aliases maps plugin-specific color names to canonical hexadecimal colors.
	Aliases map[string]string `json:"aliases,omitempty"`
}

// EditorWidgetSettingColumn declares one column in a generated table setting.
type EditorWidgetSettingColumn struct {
	// Label is the human-readable column heading.
	Label string `json:"label"`
	// Type selects a text or color input.
	Type string `json:"type"`
}

// EditorWidgetSetting declares one generic control rendered by the visual editor.
type EditorWidgetSetting struct {
	// Type selects text, select, or table behavior.
	Type string `json:"type"`
	// Label is the human-readable setting label.
	Label string `json:"label"`
	// Attribute identifies the scalar attribute edited by text and select controls.
	Attribute string `json:"attribute,omitempty"`
	// Attributes identifies parallel list attributes edited by a table control.
	Attributes []string `json:"attributes,omitempty"`
	// Columns describes the table columns corresponding to Attributes.
	Columns []EditorWidgetSettingColumn `json:"columns,omitempty"`
	// Placeholder is optional helper text for a scalar input.
	Placeholder string `json:"placeholder,omitempty"`
	// Suggestions supplies optional text-input choices without restricting custom values.
	Suggestions []string `json:"suggestions,omitempty"`
}

// EditorWidgetConstraint declares a cross-attribute validation rule.
type EditorWidgetConstraint struct {
	// Kind selects exactly-one or same-length validation.
	Kind string `json:"kind"`
	// Attributes contains the attributes participating in the rule.
	Attributes []string `json:"attributes"`
	// Optional allows the final attribute to be omitted for same-length validation.
	Optional bool `json:"optional,omitempty"`
}

// EditorWidgetBadgePreview describes a safe host-rendered badge preview.
type EditorWidgetBadgePreview struct {
	// Class is the base class used by the published widget appearance.
	Class string `json:"class"`
	// SolidClass and OutlineClass mirror the plugin's published style variants.
	SolidClass string `json:"solid_class,omitempty"`
	// OutlineClass stores the outline style class used by the preview.
	OutlineClass string `json:"outline_class,omitempty"`
	// PrefixClass styles optional prefix text.
	PrefixClass string `json:"prefix_class,omitempty"`
	// ValueClass styles the visible value.
	ValueClass string `json:"value_class,omitempty"`
	// PrefixAttribute supplies optional text shown before the value.
	PrefixAttribute string `json:"prefix_attribute,omitempty"`
	// LabelAttribute selects an explicit current value when present.
	LabelAttribute string `json:"label_attribute,omitempty"`
	// LabelsAttribute supplies fallback ordered labels.
	LabelsAttribute string `json:"labels_attribute,omitempty"`
	// FallbackAttribute supplies a scalar label when no ordered labels are available.
	FallbackAttribute string `json:"fallback_attribute,omitempty"`
	// ColorsAttribute supplies colors parallel to LabelsAttribute.
	ColorsAttribute string `json:"colors_attribute,omitempty"`
	// StyleAttribute selects solid or outline behavior.
	StyleAttribute string `json:"style_attribute,omitempty"`
	// DefaultLabel is shown when no configured value can be resolved.
	DefaultLabel string `json:"default_label,omitempty"`
	// DefaultColors supplies fallback colors for labels without explicit colors.
	DefaultColors []string `json:"default_colors,omitempty"`
	// ToneClasses maps canonical colors to plugin-owned presentation classes.
	ToneClasses map[string]string `json:"tone_classes,omitempty"`
}

// EditorWidgetPreview declares the host-rendered visual representation used while editing.
type EditorWidgetPreview struct {
	// Kind selects a bounded host-owned preview renderer.
	Kind string `json:"kind"`
	// Badge contains badge-specific presentation metadata.
	Badge *EditorWidgetBadgePreview `json:"badge,omitempty"`
}

// EditorWidgetContribution is one validated optional plugin visual-editor contract.
type EditorWidgetContribution struct {
	// PluginID identifies the enabled plugin that owns this widget contract.
	PluginID string `json:"plugin_id"`
	// ID identifies this widget within the plugin contract.
	ID string `json:"id"`
	// Name is the human-readable widget name.
	Name string `json:"name"`
	// Inline reports whether the widget occupies inline rather than block content.
	Inline bool `json:"inline"`
	// Syntax maps Markdown source into the structured widget node.
	Syntax EditorWidgetSyntax `json:"syntax"`
	// Attributes declares editable state, defaults, and validation.
	Attributes []EditorWidgetAttribute `json:"attributes"`
	// Settings declares the generic form controls shown when the widget is selected.
	Settings []EditorWidgetSetting `json:"settings"`
	// Constraints declares cross-attribute validation rules.
	Constraints []EditorWidgetConstraint `json:"constraints,omitempty"`
	// Preview describes the safe host-rendered editing representation.
	Preview EditorWidgetPreview `json:"preview"`
}

// EditorWidgetProblem reports one enabled plugin whose optional editor contract could not be loaded.
type EditorWidgetProblem struct {
	// PluginID identifies the plugin with the invalid visual-editor contract.
	PluginID string `json:"plugin_id"`
	// Message is a bounded diagnostic suitable for the authenticated editor UI.
	Message string `json:"message"`
}

// editorWidgetDocument is the versioned root stored in assets/visual-editor.json.
type editorWidgetDocument struct {
	// Version selects the host contract schema.
	Version int `json:"version"`
	// Widgets contains the plugin-owned visual editor contributions.
	Widgets []EditorWidgetContribution `json:"widgets"`
}

// EditorWidgets returns cached validated visual-editor contracts and non-fatal problems from enabled plugins.
func (m *Manager) EditorWidgets() ([]EditorWidgetContribution, []EditorWidgetProblem) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var widgets []EditorWidgetContribution
	var problems []EditorWidgetProblem
	owners := make(map[string]string)
	for _, id := range m.order {
		item, ok := m.loaded[id]
		if !ok || !item.metadata.Enabled {
			continue
		}

		if item.editorWidgetProblem != "" {
			problems = append(problems, editorWidgetProblem(id, item.editorWidgetProblem))
			continue
		}
		for _, widget := range item.editorWidgets {
			if owner, exists := owners[widget.Syntax.Name]; exists {
				problems = append(problems, editorWidgetProblem(id, fmt.Sprintf("macro %q is already handled by %s", widget.Syntax.Name, owner)))
				continue
			}
			owners[widget.Syntax.Name] = id
			widgets = append(widgets, cloneEditorWidget(widget))
		}
	}
	return widgets, problems
}

// editorWidgetsFromPackage reads and validates one package's optional visual-editor contract once.
func editorWidgetsFromPackage(pkg *pluginpackage.Package) ([]EditorWidgetContribution, string) {
	data, err := pkg.Asset(editorWidgetAsset)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, ""
	}
	if err != nil {
		return nil, "visual editor contract could not be read"
	}

	parsed, err := parseEditorWidgetDocument(data)
	if err != nil {
		return nil, err.Error()
	}
	id := pkg.Manifest().ID
	for index := range parsed {
		parsed[index].PluginID = id
	}
	return parsed, ""
}

// cloneEditorWidget copies mutable contract fields before they leave the manager.
func cloneEditorWidget(widget EditorWidgetContribution) EditorWidgetContribution {
	clone := widget
	clone.Attributes = append([]EditorWidgetAttribute(nil), widget.Attributes...)
	for index := range clone.Attributes {
		clone.Attributes[index].Values = append([]string(nil), widget.Attributes[index].Values...)
		clone.Attributes[index].Aliases = maps.Clone(widget.Attributes[index].Aliases)
	}
	clone.Settings = append([]EditorWidgetSetting(nil), widget.Settings...)
	for index := range clone.Settings {
		clone.Settings[index].Attributes = append([]string(nil), widget.Settings[index].Attributes...)
		clone.Settings[index].Columns = append([]EditorWidgetSettingColumn(nil), widget.Settings[index].Columns...)
		clone.Settings[index].Suggestions = append([]string(nil), widget.Settings[index].Suggestions...)
	}
	clone.Constraints = append([]EditorWidgetConstraint(nil), widget.Constraints...)
	for index := range clone.Constraints {
		clone.Constraints[index].Attributes = append([]string(nil), widget.Constraints[index].Attributes...)
	}
	if widget.Preview.Badge != nil {
		badge := *widget.Preview.Badge
		badge.DefaultColors = append([]string(nil), widget.Preview.Badge.DefaultColors...)
		badge.ToneClasses = maps.Clone(widget.Preview.Badge.ToneClasses)
		clone.Preview.Badge = &badge
	}
	return clone
}

// editorWidgetProblem bounds plugin-owned editor diagnostics before publishing them to clients.
func editorWidgetProblem(pluginID, message string) EditorWidgetProblem {
	message = strings.TrimSpace(message)
	if len(message) > 512 {
		message = message[:512]
	}
	return EditorWidgetProblem{PluginID: pluginID, Message: message}
}

// parseEditorWidgetDocument strictly decodes and validates one optional plugin editor contract.
func parseEditorWidgetDocument(data []byte) ([]EditorWidgetContribution, error) {
	if len(data) == 0 || len(data) > maxEditorWidgetAssetBytes {
		return nil, errors.New("visual editor contract is empty or too large")
	}

	var document editorWidgetDocument
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&document); err != nil {
		return nil, fmt.Errorf("invalid visual editor contract: %w", err)
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return nil, err
	}
	if document.Version != 1 {
		return nil, fmt.Errorf("unsupported visual editor contract version %d", document.Version)
	}
	if len(document.Widgets) == 0 || len(document.Widgets) > maxEditorWidgets {
		return nil, fmt.Errorf("visual editor contract must contain between 1 and %d widgets", maxEditorWidgets)
	}

	seen := make(map[string]bool, len(document.Widgets))
	macros := make(map[string]bool, len(document.Widgets))
	for index := range document.Widgets {
		widget := &document.Widgets[index]
		if !validID.MatchString(widget.ID) || seen[widget.ID] {
			return nil, fmt.Errorf("invalid or duplicate visual editor widget ID %q", widget.ID)
		}
		if macros[widget.Syntax.Name] {
			return nil, fmt.Errorf("duplicate visual editor macro %q", widget.Syntax.Name)
		}
		macros[widget.Syntax.Name] = true
		seen[widget.ID] = true
		if err := validateEditorWidget(widget); err != nil {
			return nil, fmt.Errorf("visual editor widget %q: %w", widget.ID, err)
		}
	}
	return document.Widgets, nil
}

// ensureJSONEOF rejects trailing JSON documents in a plugin editor contract.
func ensureJSONEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("visual editor contract must contain exactly one JSON document")
		}
		return fmt.Errorf("invalid visual editor contract: %w", err)
	}
	return nil
}

// validateEditorWidget validates one visual-editor contribution and all referenced attributes.
func validateEditorWidget(widget *EditorWidgetContribution) error {
	if strings.TrimSpace(widget.Name) == "" || len(widget.Name) > 128 {
		return errors.New("name is empty or too long")
	}
	if widget.Syntax.Kind != "macro" || !validID.MatchString(widget.Syntax.Name) {
		return errors.New("syntax must declare a valid macro name")
	}
	if len(widget.Attributes) == 0 || len(widget.Attributes) > maxEditorWidgetAttributes {
		return fmt.Errorf("must declare between 1 and %d attributes", maxEditorWidgetAttributes)
	}
	if len(widget.Settings) == 0 || len(widget.Settings) > maxEditorWidgetSettings {
		return fmt.Errorf("must declare between 1 and %d settings", maxEditorWidgetSettings)
	}
	if len(widget.Constraints) > maxEditorWidgetConstraints {
		return fmt.Errorf("declares more than %d constraints", maxEditorWidgetConstraints)
	}

	attributes := make(map[string]EditorWidgetAttribute, len(widget.Attributes))
	for _, attribute := range widget.Attributes {
		if err := validateEditorWidgetAttribute(attribute); err != nil {
			return fmt.Errorf("attribute %q: %w", attribute.Name, err)
		}
		if _, exists := attributes[attribute.Name]; exists {
			return fmt.Errorf("duplicate attribute %q", attribute.Name)
		}
		attributes[attribute.Name] = attribute
	}
	for _, setting := range widget.Settings {
		if err := validateEditorWidgetSetting(setting, attributes); err != nil {
			return err
		}
	}
	for _, constraint := range widget.Constraints {
		if err := validateEditorWidgetConstraint(constraint, attributes); err != nil {
			return err
		}
	}
	return validateEditorWidgetPreview(widget.Preview, attributes)
}

// validateEditorWidgetAttribute validates one structured source attribute declaration.
func validateEditorWidgetAttribute(attribute EditorWidgetAttribute) error {
	if !validID.MatchString(attribute.Name) {
		return errors.New("name is invalid")
	}
	if attribute.MaxBytes < 0 || attribute.MaxBytes > 4096 || attribute.MaxItems < 0 || attribute.MaxItems > 128 {
		return errors.New("limits are invalid")
	}
	if attribute.Separator != "" && attribute.Separator != ";" && attribute.Separator != "," {
		return errors.New("separator must be comma or semicolon")
	}
	if attribute.FallbackSeparator != "" &&
		(attribute.FallbackSeparator != ";" && attribute.FallbackSeparator != "," || attribute.FallbackSeparator == attribute.Separator) {
		return errors.New("fallback separator must be the other supported list separator")
	}

	switch attribute.Type {
	case "string", "identifier":
		if len(attribute.Values) != 0 || attribute.MaxItems != 0 || attribute.Separator != "" || attribute.FallbackSeparator != "" || attribute.Unique || len(attribute.Aliases) != 0 {
			return errors.New("scalar attribute declares list, enum, or color options")
		}
	case "enum":
		if len(attribute.Values) == 0 || len(attribute.Values) > 32 || attribute.MaxItems != 0 || attribute.Separator != "" || attribute.FallbackSeparator != "" || attribute.Unique || len(attribute.Aliases) != 0 {
			return errors.New("enum attribute has invalid values")
		}
		seen := make(map[string]bool, len(attribute.Values))
		for _, value := range attribute.Values {
			if value == "" || len(value) > 128 || seen[value] {
				return errors.New("enum values are empty, too long, or duplicated")
			}
			seen[value] = true
		}
		if attribute.Default != "" && !seen[attribute.Default] {
			return errors.New("default is not an allowed enum value")
		}
	case "list", "color-list":
		if len(attribute.Values) != 0 {
			return errors.New("list attribute cannot declare enum values")
		}
		if attribute.Separator == "" {
			return errors.New("list attribute requires a separator")
		}
		if attribute.Type == "list" && len(attribute.Aliases) != 0 {
			return errors.New("plain list attribute cannot declare color aliases")
		}
		if len(attribute.Aliases) > 32 {
			return errors.New("color attribute declares too many aliases")
		}
		for name, color := range attribute.Aliases {
			if !validID.MatchString(name) || !validEditorWidgetColor(color) {
				return errors.New("color attribute alias is invalid")
			}
		}
	default:
		return fmt.Errorf("unsupported type %q", attribute.Type)
	}
	return nil
}

// validateEditorWidgetSetting validates a generated control and its referenced attributes.
func validateEditorWidgetSetting(setting EditorWidgetSetting, attributes map[string]EditorWidgetAttribute) error {
	if strings.TrimSpace(setting.Label) == "" || len(setting.Label) > 128 || len(setting.Placeholder) > 256 || len(setting.Suggestions) > 16 {
		return errors.New("visual editor setting has an invalid label, placeholder, or suggestions")
	}
	for _, suggestion := range setting.Suggestions {
		if strings.TrimSpace(suggestion) == "" || len(suggestion) > 128 {
			return errors.New("visual editor setting has an invalid suggestion")
		}
	}
	switch setting.Type {
	case "text":
		attribute, ok := attributes[setting.Attribute]
		if !ok || (attribute.Type != "string" && attribute.Type != "identifier") || len(setting.Attributes) != 0 || len(setting.Columns) != 0 {
			return fmt.Errorf("text setting %q references an invalid attribute", setting.Label)
		}
	case "select":
		attribute, ok := attributes[setting.Attribute]
		if !ok || attribute.Type != "enum" || len(setting.Attributes) != 0 || len(setting.Columns) != 0 || len(setting.Suggestions) != 0 {
			return fmt.Errorf("select setting %q references an invalid attribute", setting.Label)
		}
	case "table":
		if setting.Attribute != "" || len(setting.Attributes) == 0 || len(setting.Attributes) != len(setting.Columns) || len(setting.Attributes) > 4 || len(setting.Suggestions) != 0 {
			return fmt.Errorf("table setting %q has invalid columns", setting.Label)
		}
		first := attributes[setting.Attributes[0]]
		if first.Type != "list" || setting.Columns[0].Type != "text" {
			return fmt.Errorf("table setting %q must start with a text list column", setting.Label)
		}
		seen := make(map[string]bool, len(setting.Attributes))
		for index, name := range setting.Attributes {
			attribute, ok := attributes[name]
			if !ok || (attribute.Type != "list" && attribute.Type != "color-list") || seen[name] {
				return fmt.Errorf("table setting %q references invalid list attribute %q", setting.Label, name)
			}
			seen[name] = true
			column := setting.Columns[index]
			if strings.TrimSpace(column.Label) == "" || len(column.Label) > 128 || (column.Type != "text" && column.Type != "color") {
				return fmt.Errorf("table setting %q has invalid column", setting.Label)
			}
			if column.Type == "color" && attribute.Type != "color-list" {
				return fmt.Errorf("table color column %q must target a color-list", column.Label)
			}
		}
	default:
		return fmt.Errorf("unsupported visual editor setting type %q", setting.Type)
	}
	return nil
}

// validateEditorWidgetConstraint validates a cross-attribute rule.
func validateEditorWidgetConstraint(constraint EditorWidgetConstraint, attributes map[string]EditorWidgetAttribute) error {
	if len(constraint.Attributes) < 2 || len(constraint.Attributes) > 8 {
		return errors.New("visual editor constraint must reference between 2 and 8 attributes")
	}
	seen := make(map[string]bool, len(constraint.Attributes))
	for _, name := range constraint.Attributes {
		if seen[name] {
			return fmt.Errorf("visual editor constraint repeats attribute %q", name)
		}
		seen[name] = true
		if _, ok := attributes[name]; !ok {
			return fmt.Errorf("visual editor constraint references unknown attribute %q", name)
		}
	}
	switch constraint.Kind {
	case "exactly-one":
		if constraint.Optional {
			return errors.New("exactly-one constraint cannot be optional")
		}
	case "same-length":
		seen := make(map[string]bool, len(constraint.Attributes))
		for _, name := range constraint.Attributes {
			if seen[name] {
				return fmt.Errorf("visual editor constraint repeats attribute %q", name)
			}
			seen[name] = true
			attribute := attributes[name]
			if attribute.Type != "list" && attribute.Type != "color-list" {
				return errors.New("same-length constraint requires list attributes")
			}
		}
	case "member-of":
		if len(constraint.Attributes) != 2 {
			return errors.New("member-of constraint requires a scalar and a list attribute")
		}
		value := attributes[constraint.Attributes[0]]
		set := attributes[constraint.Attributes[1]]
		if (value.Type != "string" && value.Type != "identifier") || set.Type != "list" {
			return errors.New("member-of constraint requires a scalar followed by a list attribute")
		}
	default:
		return fmt.Errorf("unsupported visual editor constraint %q", constraint.Kind)
	}
	return nil
}

// validateEditorWidgetPreview validates safe class names and attribute references for one preview.
func validateEditorWidgetPreview(preview EditorWidgetPreview, attributes map[string]EditorWidgetAttribute) error {
	if preview.Kind != "badge" || preview.Badge == nil {
		return errors.New("preview must declare a badge renderer")
	}
	badge := preview.Badge
	for _, className := range []string{badge.Class, badge.SolidClass, badge.OutlineClass, badge.PrefixClass, badge.ValueClass} {
		if className != "" && !editorWidgetClass.MatchString(className) {
			return fmt.Errorf("preview class %q is invalid", className)
		}
	}
	if badge.Class == "" || len(badge.DefaultLabel) > 128 || len(badge.DefaultColors) > 32 || len(badge.ToneClasses) > 32 {
		return errors.New("badge preview metadata is invalid")
	}
	for _, name := range []string{badge.PrefixAttribute, badge.LabelAttribute, badge.LabelsAttribute, badge.FallbackAttribute, badge.ColorsAttribute, badge.StyleAttribute} {
		if name != "" {
			if _, ok := attributes[name]; !ok {
				return fmt.Errorf("preview references unknown attribute %q", name)
			}
		}
	}
	for _, color := range badge.DefaultColors {
		if !validEditorWidgetColor(color) {
			return fmt.Errorf("preview color %q is invalid", color)
		}
	}
	for color, className := range badge.ToneClasses {
		if !validEditorWidgetColor(color) || !editorWidgetClass.MatchString(className) {
			return errors.New("preview tone class mapping is invalid")
		}
	}
	return nil
}

// validEditorWidgetColor validates one canonical six-digit hexadecimal color.
func validEditorWidgetColor(value string) bool {
	if len(value) != 7 || value[0] != '#' {
		return false
	}
	for _, char := range value[1:] {
		if char >= '0' && char <= '9' || char >= 'a' && char <= 'f' || char >= 'A' && char <= 'F' {
			continue
		}
		return false
	}
	return true
}
