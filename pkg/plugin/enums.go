package plugin

// ModuleType identifies one supported plugin manifest contribution kind.
type ModuleType string

const (
	ModuleTypeAdminAction         ModuleType = "admin-action"
	ModuleTypeAdminResource       ModuleType = "admin-resource"
	ModuleTypeBrowserModule       ModuleType = "browser-module"
	ModuleTypeCodeHighlighter     ModuleType = "code-highlighter"
	ModuleTypeContentStyle        ModuleType = "content-style"
	ModuleTypeContentSubstitution ModuleType = "content-substitution"
	ModuleTypeEditorCompletion    ModuleType = "editor-completion"
	ModuleTypeEditorInsert        ModuleType = "editor-insert"
	ModuleTypeEditorMenu          ModuleType = "editor-menu"
	ModuleTypeExporter            ModuleType = "exporter"
	ModuleTypeIconResource        ModuleType = "icon-resource"
	ModuleTypeMacro               ModuleType = "macro"
	ModuleTypeMarkdownSyntax      ModuleType = "markdown-syntax"
	ModuleTypePageAction          ModuleType = "page-action"
	ModuleTypeRenderPolicy        ModuleType = "render-policy"
	ModuleTypeRendererExtension   ModuleType = "renderer-extension"
	ModuleTypeSettings            ModuleType = "settings"
	ModuleTypeWidget              ModuleType = "widget"
)

// StorageNamespace identifies one persisted plugin-value namespace owned by Kumbuka.
type StorageNamespace string

const (
	// StorageNamespaceSettings stores administrator-managed plugin configuration.
	StorageNamespaceSettings StorageNamespace = "settings"
	// StorageNamespaceData stores plugin-owned runtime data.
	StorageNamespaceData StorageNamespace = "data"
)

// RenderStage identifies one host invocation stage exposed to sandboxed plugins.
type RenderStage string

const (
	RenderStageAdminAction       RenderStage = "admin-action"
	RenderStageContentPreprocess RenderStage = "content-preprocess"
	RenderStageExport            RenderStage = "export"
	RenderStageHighlight         RenderStage = "highlight"
	RenderStagePostprocess       RenderStage = "postprocess"
	RenderStagePreprocess        RenderStage = "preprocess"
	RenderStageWidget            RenderStage = "widget"
	RenderStageWidgetCommand     RenderStage = "widget-command"
)

// WidgetActionKind identifies one host-rendered widget action behavior.
type WidgetActionKind string

const (
	WidgetActionLink    WidgetActionKind = "link"
	WidgetActionDialog  WidgetActionKind = "dialog"
	WidgetActionCommand WidgetActionKind = "command"
)

// PageActionKind identifies one host-rendered page action presentation.
type PageActionKind string

const (
	PageActionLink   PageActionKind = "link"
	PageActionDialog PageActionKind = "dialog"
)

// EditorInsertMode identifies how an editor insertion contribution changes source text.
type EditorInsertMode string

const (
	EditorInsertModeInsert EditorInsertMode = "insert"
	EditorInsertModeWrap   EditorInsertMode = "wrap"
)

// ConfigurationFieldType identifies one host-rendered plugin configuration control.
type ConfigurationFieldType string

const (
	ConfigurationFieldBoolean  ConfigurationFieldType = "boolean"
	ConfigurationFieldColor    ConfigurationFieldType = "color"
	ConfigurationFieldList     ConfigurationFieldType = "list"
	ConfigurationFieldSecret   ConfigurationFieldType = "secret"
	ConfigurationFieldSelect   ConfigurationFieldType = "select"
	ConfigurationFieldText     ConfigurationFieldType = "text"
	ConfigurationFieldTextarea ConfigurationFieldType = "textarea"
	ConfigurationFieldURL      ConfigurationFieldType = "url"
)

// EditorWidgetSyntaxKind identifies the Markdown construct represented by a visual-editor widget.
type EditorWidgetSyntaxKind string

const (
	EditorWidgetSyntaxMacro        EditorWidgetSyntaxKind = "macro"
	EditorWidgetSyntaxSubstitution EditorWidgetSyntaxKind = "substitution"
	EditorWidgetSyntaxCallout      EditorWidgetSyntaxKind = "callout"
	EditorWidgetSyntaxDetails      EditorWidgetSyntaxKind = "details"
	EditorWidgetSyntaxTabs         EditorWidgetSyntaxKind = "tabs"
)

// EditorWidgetAttributeType identifies one visual-editor source attribute shape.
type EditorWidgetAttributeType string

const (
	EditorWidgetAttributeString     EditorWidgetAttributeType = "string"
	EditorWidgetAttributeIdentifier EditorWidgetAttributeType = "identifier"
	EditorWidgetAttributeEnum       EditorWidgetAttributeType = "enum"
	EditorWidgetAttributeList       EditorWidgetAttributeType = "list"
	EditorWidgetAttributeColorList  EditorWidgetAttributeType = "color-list"
)

// EditorWidgetSettingType identifies one generic visual-editor setting control.
type EditorWidgetSettingType string

const (
	EditorWidgetSettingText     EditorWidgetSettingType = "text"
	EditorWidgetSettingTextarea EditorWidgetSettingType = "textarea"
	EditorWidgetSettingSelect   EditorWidgetSettingType = "select"
	EditorWidgetSettingResource EditorWidgetSettingType = "resource"
	EditorWidgetSettingTable    EditorWidgetSettingType = "table"
	EditorWidgetSettingMention  EditorWidgetSettingType = "mention"
	EditorWidgetSettingDate     EditorWidgetSettingType = "date"
)

// EditorWidgetColumnType identifies one table-setting column control.
type EditorWidgetColumnType string

const (
	EditorWidgetColumnText     EditorWidgetColumnType = "text"
	EditorWidgetColumnTextarea EditorWidgetColumnType = "textarea"
	EditorWidgetColumnColor    EditorWidgetColumnType = "color"
)

// EditorWidgetConstraintKind identifies one cross-attribute validation rule.
type EditorWidgetConstraintKind string

const (
	EditorWidgetConstraintExactlyOne EditorWidgetConstraintKind = "exactly-one"
	EditorWidgetConstraintSameLength EditorWidgetConstraintKind = "same-length"
	EditorWidgetConstraintMemberOf   EditorWidgetConstraintKind = "member-of"
)

// EditorWidgetPreviewKind identifies one bounded host-owned preview renderer.
type EditorWidgetPreviewKind string

const (
	EditorWidgetPreviewBadge     EditorWidgetPreviewKind = "badge"
	EditorWidgetPreviewReference EditorWidgetPreviewKind = "reference"
	EditorWidgetPreviewCard      EditorWidgetPreviewKind = "card"
	EditorWidgetPreviewCallout   EditorWidgetPreviewKind = "callout"
	EditorWidgetPreviewDetails   EditorWidgetPreviewKind = "details"
	EditorWidgetPreviewTabs      EditorWidgetPreviewKind = "tabs"
)
