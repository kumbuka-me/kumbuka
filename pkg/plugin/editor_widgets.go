package plugin

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
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
	// Kind selects macro, substitution, callout, details, or tabs source mapping.
	Kind string `json:"kind"`
	// Name identifies a macro name or substitution prefix when the syntax kind uses one.
	Name string `json:"name,omitempty"`
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
	// Repeat serializes list values as repeated macro attributes instead of one joined value.
	Repeat bool `json:"repeat,omitempty"`
	// EmitEmpty preserves an explicit empty scalar value instead of omitting its source attribute.
	EmitEmpty bool `json:"emit_empty,omitempty"`
	// Aliases maps plugin-specific color names to canonical hexadecimal colors.
	Aliases map[string]string `json:"aliases,omitempty"`
}

// EditorWidgetSettingColumn declares one column in a generated table setting.
type EditorWidgetSettingColumn struct {
	// Label is the human-readable column heading.
	Label string `json:"label"`
	// Type selects a text, textarea, or color input.
	Type string `json:"type"`
}

// EditorWidgetSetting declares one generic control rendered by the visual editor.
type EditorWidgetSetting struct {
	// Type selects text, textarea, select, or table behavior.
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

// EditorWidgetReferencePreview describes an inline reference chip for substitutions and resource-backed content.
type EditorWidgetReferencePreview struct {
	// Class is the plugin-owned presentation class applied to the reference.
	Class string `json:"class"`
	// Prefix is the short visible kind label, such as Variable or Include.
	Prefix string `json:"prefix"`
	// ValueAttribute selects the source attribute displayed after Prefix.
	ValueAttribute string `json:"value_attribute"`
	// DefaultValue is used when the selected attribute is empty.
	DefaultValue string `json:"default_value,omitempty"`
}

// EditorWidgetCardPreview describes a bounded block card for dynamic plugin content.
type EditorWidgetCardPreview struct {
	// Class is the base class used by the published component when available.
	Class string `json:"class"`
	// Title is the fixed heading shown by the card.
	Title string `json:"title"`
	// TitleClass optionally applies a published title class.
	TitleClass string `json:"title_class,omitempty"`
	// SubtitleAttribute selects the main source value shown below the title.
	SubtitleAttribute string `json:"subtitle_attribute,omitempty"`
	// SubtitleClass optionally applies a published subtitle class.
	SubtitleClass string `json:"subtitle_class,omitempty"`
	// MetadataAttributes selects additional non-empty source values shown compactly.
	MetadataAttributes []string `json:"metadata_attributes,omitempty"`
	// MetadataClass optionally applies a published metadata class.
	MetadataClass string `json:"metadata_class,omitempty"`
	// BodyText is optional static helper text for dynamic content unavailable in the editor.
	BodyText string `json:"body_text,omitempty"`
}

// EditorWidgetCalloutPreview describes the published callout panel shape.
type EditorWidgetCalloutPreview struct {
	// Class is the base callout class.
	Class string `json:"class"`
	// BodyClass is the class applied to callout body text.
	BodyClass string `json:"body_class,omitempty"`
	// KindAttribute selects the callout kind and additional tone class.
	KindAttribute string `json:"kind_attribute"`
	// BodyAttribute selects callout body text.
	BodyAttribute string `json:"body_attribute"`
}

// EditorWidgetDetailsPreview describes a native details preview.
type EditorWidgetDetailsPreview struct {
	// Class is the published details class.
	Class string `json:"class"`
	// BodyClass is the published details-body class.
	BodyClass string `json:"body_class,omitempty"`
	// TitleAttribute selects summary text.
	TitleAttribute string `json:"title_attribute"`
	// OpenAttribute selects the true/false initial-open value.
	OpenAttribute string `json:"open_attribute"`
	// BodyAttribute selects details body text.
	BodyAttribute string `json:"body_attribute"`
}

// EditorWidgetTabsPreview describes the published tabs component shape.
type EditorWidgetTabsPreview struct {
	// Class is the outer tabs class.
	Class string `json:"class"`
	// ListClass is the tab-list class.
	ListClass string `json:"list_class"`
	// TabClass is the tab-button class.
	TabClass string `json:"tab_class"`
	// ActiveClass marks the first visible tab.
	ActiveClass string `json:"active_class,omitempty"`
	// PanelsClass is the tab-panels wrapper class.
	PanelsClass string `json:"panels_class"`
	// PanelClass is the visible panel class.
	PanelClass string `json:"panel_class"`
	// HiddenClass marks inactive panels.
	HiddenClass string `json:"hidden_class,omitempty"`
	// TitlesAttribute selects the list of tab titles.
	TitlesAttribute string `json:"titles_attribute"`
	// BodiesAttribute selects the parallel list of tab bodies.
	BodiesAttribute string `json:"bodies_attribute"`
}

// EditorWidgetPreview declares the host-rendered visual representation used while editing.
type EditorWidgetPreview struct {
	// Kind selects a bounded host-owned preview renderer.
	Kind string `json:"kind"`
	// Badge contains badge-specific presentation metadata.
	Badge *EditorWidgetBadgePreview `json:"badge,omitempty"`
	// Reference contains resource-reference chip metadata.
	Reference *EditorWidgetReferencePreview `json:"reference,omitempty"`
	// Card contains dynamic block-card metadata.
	Card *EditorWidgetCardPreview `json:"card,omitempty"`
	// Callout contains callout-panel metadata.
	Callout *EditorWidgetCalloutPreview `json:"callout,omitempty"`
	// Details contains collapsible-details metadata.
	Details *EditorWidgetDetailsPreview `json:"details,omitempty"`
	// Tabs contains tab-group metadata.
	Tabs *EditorWidgetTabsPreview `json:"tabs,omitempty"`
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
		clone.Attributes[index].Aliases = cloneStringMap(widget.Attributes[index].Aliases)
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
		badge.ToneClasses = cloneStringMap(widget.Preview.Badge.ToneClasses)
		clone.Preview.Badge = &badge
	}
	if widget.Preview.Reference != nil {
		reference := *widget.Preview.Reference
		clone.Preview.Reference = &reference
	}
	if widget.Preview.Card != nil {
		card := *widget.Preview.Card
		card.MetadataAttributes = append([]string(nil), widget.Preview.Card.MetadataAttributes...)
		clone.Preview.Card = &card
	}
	if widget.Preview.Callout != nil {
		callout := *widget.Preview.Callout
		clone.Preview.Callout = &callout
	}
	if widget.Preview.Details != nil {
		details := *widget.Preview.Details
		clone.Preview.Details = &details
	}
	if widget.Preview.Tabs != nil {
		tabs := *widget.Preview.Tabs
		clone.Preview.Tabs = &tabs
	}
	return clone
}

// cloneStringMap copies a string map while preserving nil.
func cloneStringMap(source map[string]string) map[string]string {
	if source == nil {
		return nil
	}
	clone := make(map[string]string, len(source))
	for key, value := range source {
		clone[key] = value
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
	for index := range document.Widgets {
		widget := &document.Widgets[index]
		if !validID.MatchString(widget.ID) || seen[widget.ID] {
			return nil, fmt.Errorf("invalid or duplicate visual editor widget ID %q", widget.ID)
		}
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
	if err := validateEditorWidgetDeclaration(widget); err != nil {
		return err
	}

	attributes, err := validateEditorWidgetAttributes(widget.Attributes)
	if err != nil {
		return err
	}
	if err := validateEditorWidgetSettings(widget.Settings, attributes); err != nil {
		return err
	}
	if err := validateEditorWidgetConstraints(widget.Constraints, attributes); err != nil {
		return err
	}
	return validateEditorWidgetPreview(widget.Preview, attributes)
}

// validateEditorWidgetDeclaration validates widget metadata, syntax, and collection bounds.
func validateEditorWidgetDeclaration(widget *EditorWidgetContribution) error {
	if strings.TrimSpace(widget.Name) == "" || len(widget.Name) > 128 {
		return errors.New("name is empty or too long")
	}
	if err := validateEditorWidgetSyntax(widget.Syntax, widget.Inline); err != nil {
		return err
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
	return nil
}

// validateEditorWidgetSyntax validates the source syntax declaration for one widget.
func validateEditorWidgetSyntax(syntax EditorWidgetSyntax, inline bool) error {
	switch syntax.Kind {
	case "macro", "substitution":
		if !validID.MatchString(syntax.Name) {
			return errors.New("syntax must declare a valid macro name or substitution prefix")
		}
	case "callout", "details", "tabs":
		if syntax.Name != "" || inline {
			return errors.New("block syntax cannot declare a name or be inline")
		}
	default:
		return fmt.Errorf("unsupported visual editor syntax kind %q", syntax.Kind)
	}
	return nil
}

// validateEditorWidgetAttributes validates attributes and returns them indexed by name.
func validateEditorWidgetAttributes(declarations []EditorWidgetAttribute) (map[string]EditorWidgetAttribute, error) {
	attributes := make(map[string]EditorWidgetAttribute, len(declarations))
	for _, attribute := range declarations {
		if err := validateEditorWidgetAttribute(attribute); err != nil {
			return nil, fmt.Errorf("attribute %q: %w", attribute.Name, err)
		}
		if _, exists := attributes[attribute.Name]; exists {
			return nil, fmt.Errorf("duplicate attribute %q", attribute.Name)
		}
		attributes[attribute.Name] = attribute
	}
	return attributes, nil
}

// validateEditorWidgetSettings validates all generated controls against declared attributes.
func validateEditorWidgetSettings(settings []EditorWidgetSetting, attributes map[string]EditorWidgetAttribute) error {
	for _, setting := range settings {
		if err := validateEditorWidgetSetting(setting, attributes); err != nil {
			return err
		}
	}
	return nil
}

// validateEditorWidgetConstraints validates all cross-attribute constraints.
func validateEditorWidgetConstraints(constraints []EditorWidgetConstraint, attributes map[string]EditorWidgetAttribute) error {
	for _, constraint := range constraints {
		if err := validateEditorWidgetConstraint(constraint, attributes); err != nil {
			return err
		}
	}
	return nil
}

// validateEditorWidgetAttribute validates one structured source attribute declaration.
func validateEditorWidgetAttribute(attribute EditorWidgetAttribute) error {
	if !validID.MatchString(attribute.Name) {
		return errors.New("name is invalid")
	}
	if !validEditorWidgetAttributeLimits(attribute) {
		return errors.New("limits are invalid")
	}
	if err := validateEditorWidgetAttributeSeparators(attribute); err != nil {
		return err
	}

	switch attribute.Type {
	case "string", "identifier":
		return validateEditorWidgetScalarAttributeDeclaration(attribute)
	case "enum":
		return validateEditorWidgetEnumAttributeDeclaration(attribute)
	case "list", "color-list":
		return validateEditorWidgetListAttributeDeclaration(attribute)
	default:
		return fmt.Errorf("unsupported type %q", attribute.Type)
	}
}

// validateEditorWidgetAttributeSeparators validates primary and fallback separator declarations.
func validateEditorWidgetAttributeSeparators(attribute EditorWidgetAttribute) error {
	validSeparator := func(value string) bool { return value == "" || value == ";" || value == "," || value == "\x1f" }
	if !validSeparator(attribute.Separator) {
		return errors.New("separator must be comma, semicolon, or unit separator")
	}
	if !validEditorWidgetFallbackSeparator(attribute.FallbackSeparator, attribute.Separator, validSeparator) {
		return errors.New("fallback separator is invalid")
	}
	return nil
}

// validateEditorWidgetScalarAttributeDeclaration rejects list, enum, and color options on scalar attributes.
func validateEditorWidgetScalarAttributeDeclaration(attribute EditorWidgetAttribute) error {
	if !validEditorWidgetScalarAttribute(attribute) {
		return errors.New("scalar attribute declares list, enum, or color options")
	}
	return nil
}

// validateEditorWidgetEnumAttributeDeclaration validates enum shape, values, uniqueness, and default membership.
func validateEditorWidgetEnumAttributeDeclaration(attribute EditorWidgetAttribute) error {
	if !validEditorWidgetEnumAttribute(attribute) {
		return errors.New("enum attribute has invalid values")
	}
	seen := make(map[string]bool, len(attribute.Values))
	for _, value := range attribute.Values {
		if !validEditorWidgetEnumValue(value, seen) {
			return errors.New("enum values are empty, too long, or duplicated")
		}
		seen[value] = true
	}
	if attribute.Default != "" && !seen[attribute.Default] {
		return errors.New("default is not an allowed enum value")
	}
	return nil
}

// validateEditorWidgetListAttributeDeclaration validates list separators and color-list aliases.
func validateEditorWidgetListAttributeDeclaration(attribute EditorWidgetAttribute) error {
	if len(attribute.Values) != 0 || attribute.EmitEmpty {
		return errors.New("list attribute cannot declare enum values or emit-empty behavior")
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
	return nil
}

// validEditorWidgetFallbackSeparator reports whether a fallback separator is supported and differs from the primary separator when set.
func validEditorWidgetFallbackSeparator(fallback, primary string, valid func(string) bool) bool {
	return valid(fallback) && (fallback == "" || fallback != primary)
}

// validEditorWidgetEnumValue reports whether an enum value is non-empty, bounded, and unique.
func validEditorWidgetEnumValue(value string, seen map[string]bool) bool {
	return value != "" && len(value) <= 128 && !seen[value]
}

// validEditorWidgetMemberOfConstraint reports whether a member-of rule maps a scalar attribute to a list attribute.
func validEditorWidgetMemberOfConstraint(value, set EditorWidgetAttribute) bool {
	return (value.Type == "string" || value.Type == "identifier") && set.Type == "list"
}

// validateEditorWidgetSetting validates a generated control and its referenced attributes.
func validateEditorWidgetSetting(setting EditorWidgetSetting, attributes map[string]EditorWidgetAttribute) error {
	if err := validateEditorWidgetSettingMetadata(setting); err != nil {
		return err
	}

	switch setting.Type {
	case "text", "textarea":
		return validateEditorWidgetTextSettingDeclaration(setting, attributes)
	case "select":
		return validateEditorWidgetSelectSettingDeclaration(setting, attributes)
	case "table":
		return validateEditorWidgetTableSettingDeclaration(setting, attributes)
	default:
		return fmt.Errorf("unsupported visual editor setting type %q", setting.Type)
	}
}

// validateEditorWidgetSettingMetadata validates common control labels, placeholders, and suggestions.
func validateEditorWidgetSettingMetadata(setting EditorWidgetSetting) error {
	if !validEditorWidgetSettingMetadata(setting) {
		return errors.New("visual editor setting has an invalid label, placeholder, or suggestions")
	}
	for _, suggestion := range setting.Suggestions {
		if strings.TrimSpace(suggestion) == "" || len(suggestion) > 128 {
			return errors.New("visual editor setting has an invalid suggestion")
		}
	}
	return nil
}

// validateEditorWidgetTextSettingDeclaration validates text and textarea controls.
func validateEditorWidgetTextSettingDeclaration(setting EditorWidgetSetting, attributes map[string]EditorWidgetAttribute) error {
	attribute, ok := attributes[setting.Attribute]
	if !validEditorWidgetTextSetting(setting, attribute, ok) {
		return fmt.Errorf("%s setting %q references an invalid attribute", setting.Type, setting.Label)
	}
	return nil
}

// validateEditorWidgetSelectSettingDeclaration validates select controls.
func validateEditorWidgetSelectSettingDeclaration(setting EditorWidgetSetting, attributes map[string]EditorWidgetAttribute) error {
	attribute, ok := attributes[setting.Attribute]
	if !validEditorWidgetSelectSetting(setting, attribute, ok) {
		return fmt.Errorf("select setting %q references an invalid attribute", setting.Label)
	}
	return nil
}

// validateEditorWidgetTableSettingDeclaration validates table shape and each bound list column.
func validateEditorWidgetTableSettingDeclaration(setting EditorWidgetSetting, attributes map[string]EditorWidgetAttribute) error {
	if !validEditorWidgetTableShape(setting) {
		return fmt.Errorf("table setting %q has invalid columns", setting.Label)
	}
	first := attributes[setting.Attributes[0]]
	if !validEditorWidgetFirstTableColumn(first, setting.Columns[0]) {
		return fmt.Errorf("table setting %q must start with a text or textarea list column", setting.Label)
	}

	seen := make(map[string]bool, len(setting.Attributes))
	for index, name := range setting.Attributes {
		if err := validateEditorWidgetTableColumnBinding(setting, index, name, attributes, seen[name]); err != nil {
			return err
		}
		seen[name] = true
	}
	return nil
}

// validateEditorWidgetTableColumnBinding validates one table column and its target attribute.
func validateEditorWidgetTableColumnBinding(setting EditorWidgetSetting, index int, name string, attributes map[string]EditorWidgetAttribute, duplicate bool) error {
	attribute, ok := attributes[name]
	if !validEditorWidgetTableAttribute(attribute, ok, duplicate) {
		return fmt.Errorf("table setting %q references invalid list attribute %q", setting.Label, name)
	}
	column := setting.Columns[index]
	if !validEditorWidgetTableColumn(column) {
		return fmt.Errorf("table setting %q has invalid column", setting.Label)
	}
	if column.Type == "color" && attribute.Type != "color-list" {
		return fmt.Errorf("table color column %q must target a color-list", column.Label)
	}
	return nil
}

// validateEditorWidgetConstraint validates a cross-attribute rule.
func validateEditorWidgetConstraint(constraint EditorWidgetConstraint, attributes map[string]EditorWidgetAttribute) error {
	if err := validateEditorWidgetConstraintReferences(constraint, attributes); err != nil {
		return err
	}

	switch constraint.Kind {
	case "exactly-one":
		if constraint.Optional {
			return errors.New("exactly-one constraint cannot be optional")
		}
	case "same-length":
		return validateEditorWidgetSameLengthConstraint(constraint, attributes)
	case "member-of":
		return validateEditorWidgetMemberOfConstraintDeclaration(constraint, attributes)
	default:
		return fmt.Errorf("unsupported visual editor constraint %q", constraint.Kind)
	}
	return nil
}

// validateEditorWidgetConstraintReferences validates the number and existence of referenced attributes.
func validateEditorWidgetConstraintReferences(constraint EditorWidgetConstraint, attributes map[string]EditorWidgetAttribute) error {
	if len(constraint.Attributes) < 2 || len(constraint.Attributes) > 8 {
		return errors.New("visual editor constraint must reference between 2 and 8 attributes")
	}
	for _, name := range constraint.Attributes {
		if _, ok := attributes[name]; !ok {
			return fmt.Errorf("visual editor constraint references unknown attribute %q", name)
		}
	}
	return nil
}

// validateEditorWidgetSameLengthConstraint requires all referenced attributes to be list-shaped.
func validateEditorWidgetSameLengthConstraint(constraint EditorWidgetConstraint, attributes map[string]EditorWidgetAttribute) error {
	for _, name := range constraint.Attributes {
		attribute := attributes[name]
		if attribute.Type != "list" && attribute.Type != "color-list" {
			return errors.New("same-length constraint requires list attributes")
		}
	}
	return nil
}

// validateEditorWidgetMemberOfConstraintDeclaration validates the scalar-to-list member relationship.
func validateEditorWidgetMemberOfConstraintDeclaration(constraint EditorWidgetConstraint, attributes map[string]EditorWidgetAttribute) error {
	if len(constraint.Attributes) != 2 {
		return errors.New("member-of constraint requires a scalar and a list attribute")
	}
	value := attributes[constraint.Attributes[0]]
	set := attributes[constraint.Attributes[1]]
	if !validEditorWidgetMemberOfConstraint(value, set) {
		return errors.New("member-of constraint requires a scalar followed by a list attribute")
	}
	return nil
}

// validateEditorWidgetPreview validates safe class names and attribute references for one preview.
func validateEditorWidgetPreview(preview EditorWidgetPreview, attributes map[string]EditorWidgetAttribute) error {
	switch preview.Kind {
	case "badge":
		return validateEditorWidgetBadgePreview(preview, attributes)
	case "reference":
		return validateEditorWidgetReferencePreview(preview, attributes)
	case "card":
		return validateEditorWidgetCardPreview(preview, attributes)
	case "callout":
		return validateEditorWidgetCalloutPreview(preview, attributes)
	case "details":
		return validateEditorWidgetDetailsPreview(preview, attributes)
	case "tabs":
		return validateEditorWidgetTabsPreview(preview, attributes)
	default:
		return fmt.Errorf("unsupported visual editor preview kind %q", preview.Kind)
	}
}

// validateEditorWidgetBadgePreview validates badge-specific preview metadata.
func validateEditorWidgetBadgePreview(preview EditorWidgetPreview, attributes map[string]EditorWidgetAttribute) error {
	if preview.Badge == nil || previewHasOtherKind(preview, "badge") {
		return errors.New("badge preview metadata is invalid")
	}

	badge := preview.Badge
	if err := validateEditorWidgetClasses(badge.Class, badge.SolidClass, badge.OutlineClass, badge.PrefixClass, badge.ValueClass); err != nil {
		return err
	}
	if !validEditorWidgetBadgeMetadata(badge) {
		return errors.New("badge preview metadata is invalid")
	}
	if err := validateEditorWidgetAttributeReferences(attributes, badge.PrefixAttribute, badge.LabelAttribute, badge.LabelsAttribute, badge.FallbackAttribute, badge.ColorsAttribute, badge.StyleAttribute); err != nil {
		return err
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

// validateEditorWidgetReferencePreview validates inline-reference preview metadata.
func validateEditorWidgetReferencePreview(preview EditorWidgetPreview, attributes map[string]EditorWidgetAttribute) error {
	if preview.Reference == nil || previewHasOtherKind(preview, "reference") {
		return errors.New("reference preview metadata is invalid")
	}

	reference := preview.Reference
	if !validEditorWidgetReferenceMetadata(reference) {
		return errors.New("reference preview metadata is invalid")
	}
	if err := validateEditorWidgetClasses(reference.Class); err != nil {
		return err
	}
	return validateEditorWidgetAttributeReferences(attributes, reference.ValueAttribute)
}

// validateEditorWidgetCardPreview validates card-specific preview metadata.
func validateEditorWidgetCardPreview(preview EditorWidgetPreview, attributes map[string]EditorWidgetAttribute) error {
	if preview.Card == nil || previewHasOtherKind(preview, "card") {
		return errors.New("card preview metadata is invalid")
	}

	card := preview.Card
	if !validEditorWidgetCardMetadata(card) {
		return errors.New("card preview metadata is invalid")
	}
	if err := validateEditorWidgetClasses(card.Class, card.TitleClass, card.SubtitleClass, card.MetadataClass); err != nil {
		return err
	}
	return validateEditorWidgetAttributeReferences(attributes, append([]string{card.SubtitleAttribute}, card.MetadataAttributes...)...)
}

// validateEditorWidgetCalloutPreview validates callout-specific preview metadata.
func validateEditorWidgetCalloutPreview(preview EditorWidgetPreview, attributes map[string]EditorWidgetAttribute) error {
	if preview.Callout == nil || previewHasOtherKind(preview, "callout") {
		return errors.New("callout preview metadata is invalid")
	}

	callout := preview.Callout
	if callout.Class == "" {
		return errors.New("callout preview class is required")
	}
	if err := validateEditorWidgetClasses(callout.Class, callout.BodyClass); err != nil {
		return err
	}
	return validateEditorWidgetAttributeReferences(attributes, callout.KindAttribute, callout.BodyAttribute)
}

// validateEditorWidgetDetailsPreview validates details-specific preview metadata.
func validateEditorWidgetDetailsPreview(preview EditorWidgetPreview, attributes map[string]EditorWidgetAttribute) error {
	if preview.Details == nil || previewHasOtherKind(preview, "details") {
		return errors.New("details preview metadata is invalid")
	}

	details := preview.Details
	if details.Class == "" {
		return errors.New("details preview class is required")
	}
	if err := validateEditorWidgetClasses(details.Class, details.BodyClass); err != nil {
		return err
	}
	return validateEditorWidgetAttributeReferences(attributes, details.TitleAttribute, details.OpenAttribute, details.BodyAttribute)
}

// validateEditorWidgetTabsPreview validates tabs-specific preview metadata.
func validateEditorWidgetTabsPreview(preview EditorWidgetPreview, attributes map[string]EditorWidgetAttribute) error {
	if preview.Tabs == nil || previewHasOtherKind(preview, "tabs") {
		return errors.New("tabs preview metadata is invalid")
	}

	tabs := preview.Tabs
	if err := validateEditorWidgetClasses(tabs.Class, tabs.ListClass, tabs.TabClass, tabs.ActiveClass, tabs.PanelsClass, tabs.PanelClass, tabs.HiddenClass); err != nil {
		return err
	}
	if !validEditorWidgetTabsClasses(tabs) {
		return errors.New("tabs preview classes are required")
	}
	for _, name := range []string{tabs.TitlesAttribute, tabs.BodiesAttribute} {
		if err := validateEditorWidgetAttributeReferences(attributes, name); err != nil {
			return err
		}
		if attributes[name].Type != "list" {
			return errors.New("tabs preview attributes must be lists")
		}
	}
	return nil
}

// previewHasOtherKind reports whether preview contains metadata for a different preview renderer.
func previewHasOtherKind(preview EditorWidgetPreview, allowed string) bool {
	kinds := []struct {
		name    string
		present bool
	}{
		{name: "badge", present: preview.Badge != nil},
		{name: "reference", present: preview.Reference != nil},
		{name: "card", present: preview.Card != nil},
		{name: "callout", present: preview.Callout != nil},
		{name: "details", present: preview.Details != nil},
		{name: "tabs", present: preview.Tabs != nil},
	}
	for _, kind := range kinds {
		if kind.name != allowed && kind.present {
			return true
		}
	}
	return false
}

// validateEditorWidgetClasses rejects unsafe non-empty presentation class names.
func validateEditorWidgetClasses(classes ...string) error {
	for _, className := range classes {
		if className != "" && !editorWidgetClass.MatchString(className) {
			return fmt.Errorf("preview class %q is invalid", className)
		}
	}
	return nil
}

// validateEditorWidgetAttributeReferences rejects preview references to undeclared widget attributes.
func validateEditorWidgetAttributeReferences(attributes map[string]EditorWidgetAttribute, names ...string) error {
	for _, name := range names {
		if name == "" {
			continue
		}
		if _, ok := attributes[name]; !ok {
			return fmt.Errorf("preview references unknown attribute %q", name)
		}
	}
	return nil
}

// validEditorWidgetColor validates one canonical six-digit hexadecimal color.
func validEditorWidgetColor(value string) bool {
	if !hasHexColorShape(value) {
		return false
	}
	for _, char := range value[1:] {
		if isHexadecimalDigit(char) {
			continue
		}
		return false
	}
	return true
}

// validEditorWidgetAttributeLimits reports whether list and byte limits are within contract bounds.
func validEditorWidgetAttributeLimits(attribute EditorWidgetAttribute) bool {
	return attribute.MaxBytes >= 0 && attribute.MaxBytes <= 65536 && attribute.MaxItems >= 0 && attribute.MaxItems <= 128
}

// validEditorWidgetScalarAttribute reports whether a scalar attribute avoids list, enum, and color-only options.
func validEditorWidgetScalarAttribute(attribute EditorWidgetAttribute) bool {
	return len(attribute.Values) == 0 && attribute.MaxItems == 0 && attribute.Separator == "" &&
		attribute.FallbackSeparator == "" && !attribute.Unique && !attribute.Repeat && len(attribute.Aliases) == 0
}

// validEditorWidgetEnumAttribute reports whether an enum attribute uses only enum-compatible options.
func validEditorWidgetEnumAttribute(attribute EditorWidgetAttribute) bool {
	return len(attribute.Values) > 0 && len(attribute.Values) <= 32 && attribute.MaxItems == 0 &&
		attribute.Separator == "" && attribute.FallbackSeparator == "" && !attribute.Unique && !attribute.Repeat &&
		!attribute.EmitEmpty && len(attribute.Aliases) == 0
}

// validEditorWidgetSettingMetadata reports whether common setting presentation fields stay within contract limits.
func validEditorWidgetSettingMetadata(setting EditorWidgetSetting) bool {
	return strings.TrimSpace(setting.Label) != "" && len(setting.Label) <= 128 && len(setting.Placeholder) <= 256 && len(setting.Suggestions) <= 16
}

// validEditorWidgetTextSetting reports whether a text control targets one scalar attribute.
func validEditorWidgetTextSetting(setting EditorWidgetSetting, attribute EditorWidgetAttribute, found bool) bool {
	return found && (attribute.Type == "string" || attribute.Type == "identifier") && len(setting.Attributes) == 0 && len(setting.Columns) == 0
}

// validEditorWidgetSelectSetting reports whether a select control targets one enum attribute without extra control data.
func validEditorWidgetSelectSetting(setting EditorWidgetSetting, attribute EditorWidgetAttribute, found bool) bool {
	return found && attribute.Type == "enum" && len(setting.Attributes) == 0 && len(setting.Columns) == 0 && len(setting.Suggestions) == 0
}

// validEditorWidgetTableShape reports whether a table control has a bounded one-to-one attribute and column layout.
func validEditorWidgetTableShape(setting EditorWidgetSetting) bool {
	return setting.Attribute == "" && len(setting.Attributes) > 0 && len(setting.Attributes) == len(setting.Columns) &&
		len(setting.Attributes) <= 4 && len(setting.Suggestions) == 0
}

// validEditorWidgetFirstTableColumn reports whether the first table column is textual and backed by a list.
func validEditorWidgetFirstTableColumn(attribute EditorWidgetAttribute, column EditorWidgetSettingColumn) bool {
	return attribute.Type == "list" && (column.Type == "text" || column.Type == "textarea")
}

// validEditorWidgetTableAttribute reports whether a table attribute exists, is list-like, and is not duplicated.
func validEditorWidgetTableAttribute(attribute EditorWidgetAttribute, found, duplicate bool) bool {
	return found && (attribute.Type == "list" || attribute.Type == "color-list") && !duplicate
}

// validEditorWidgetTableColumn reports whether a table column has a bounded label and supported control type.
func validEditorWidgetTableColumn(column EditorWidgetSettingColumn) bool {
	return strings.TrimSpace(column.Label) != "" && len(column.Label) <= 128 &&
		(column.Type == "text" || column.Type == "textarea" || column.Type == "color")
}

// validEditorWidgetBadgeMetadata reports whether badge preview metadata stays within contract limits.
func validEditorWidgetBadgeMetadata(badge *EditorWidgetBadgePreview) bool {
	return badge.Class != "" && len(badge.DefaultLabel) <= 128 && len(badge.DefaultColors) <= 32 && len(badge.ToneClasses) <= 32
}

// validEditorWidgetReferenceMetadata reports whether reference preview metadata has a usable prefix and bounded values.
func validEditorWidgetReferenceMetadata(reference *EditorWidgetReferencePreview) bool {
	return reference.Class != "" && strings.TrimSpace(reference.Prefix) != "" && len(reference.Prefix) <= 64 && len(reference.DefaultValue) <= 128
}

// validEditorWidgetCardMetadata reports whether card preview metadata has required text and bounded optional content.
func validEditorWidgetCardMetadata(card *EditorWidgetCardPreview) bool {
	return card.Class != "" && strings.TrimSpace(card.Title) != "" && len(card.Title) <= 128 && len(card.BodyText) <= 512 && len(card.MetadataAttributes) <= 8
}

// validEditorWidgetTabsClasses reports whether every structural tabs preview class is configured.
func validEditorWidgetTabsClasses(tabs *EditorWidgetTabsPreview) bool {
	return tabs.Class != "" && tabs.ListClass != "" && tabs.TabClass != "" && tabs.PanelsClass != "" && tabs.PanelClass != ""
}

// hasHexColorShape reports whether value uses the hash-prefixed six-digit hexadecimal color shape.
func hasHexColorShape(value string) bool {
	return len(value) == 7 && value[0] == '#'
}

// isHexadecimalDigit reports whether character is an ASCII hexadecimal digit.
func isHexadecimalDigit(character rune) bool {
	return character >= '0' && character <= '9' || character >= 'a' && character <= 'f' || character >= 'A' && character <= 'F'
}
