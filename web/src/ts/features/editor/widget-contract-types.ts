// Types shared by widget contract validation, parsing, and editor views.
export type WidgetAttributeType =
  "string" | "identifier" | "enum" | "list" | "color-list";

export type WidgetSyntaxKind =
  "macro" | "substitution" | "callout" | "details" | "tabs";

export interface CatalogWidgetAttribute {
  name: string;
  type: WidgetAttributeType;
  default?: string;
  required?: boolean;
  max_bytes?: number;
  max_items?: number;
  values?: string[];
  separator?: string;
  fallback_separator?: string;
  unique?: boolean;
  repeat?: boolean;
  emit_empty?: boolean;
  aliases?: Record<string, string>;
}

export interface CatalogWidgetSettingColumn {
  label: string;
  type: "text" | "textarea" | "color";
}

export interface CatalogWidgetChoice {
  value: string;
  label: string;
  default?: boolean;
}

export interface CatalogWidgetChoiceSource {
  setting_module_id?: string;
  resource_module_id?: string;
  source_attribute: string;
  list_field: string;
  value_column: string;
  label_column: string;
  default_column?: string;
}

export interface CatalogWidgetTreeField {
  attribute: string;
  label: string;
  type: "text" | "textarea" | "mention" | "date" | "select";
  placeholder?: string;
  suggestions?: string[];
  choice_source?: CatalogWidgetChoiceSource;
  choices?: Record<string, CatalogWidgetChoice[]>;
}

export interface CatalogWidgetSetting {
  type:
    | "text"
    | "textarea"
    | "select"
    | "resource"
    | "mention"
    | "date"
    | "table"
    | "tree";
  label: string;
  attribute?: string;
  attributes?: string[];
  columns?: CatalogWidgetSettingColumn[];
  row_separator?: string;
  placeholder?: string;
  suggestions?: string[];
  completion_module_id?: string;
  fields?: CatalogWidgetTreeField[];
  id_attribute?: string;
  parent_attribute?: string;
  title_attribute?: string;
  description_attribute?: string;
  empty_value?: string;
  id_prefix?: string;
  max_depth?: number;
  choice_source?: CatalogWidgetChoiceSource;
  choices?: Record<string, CatalogWidgetChoice[]>;
}

export interface CatalogWidgetLineAnnotations {
  attribute: string;
  line_class: string;
  line_number_class: string;
}

export interface CatalogWidgetConstraint {
  kind: "exactly-one" | "same-length" | "member-of";
  attributes: string[];
  optional?: boolean;
}

export interface CatalogWidgetBadgePreview {
  class: string;
  solid_class?: string;
  outline_class?: string;
  prefix_class?: string;
  value_class?: string;
  prefix_attribute?: string;
  label_attribute?: string;
  labels_attribute?: string;
  fallback_attribute?: string;
  colors_attribute?: string;
  style_attribute?: string;
  default_label?: string;
  default_colors?: string[];
  tone_classes?: Record<string, string>;
}

export interface CatalogWidgetReferencePreview {
  class: string;
  prefix: string;
  value_attribute: string;
  default_value?: string;
}

export interface CatalogWidgetCardPreview {
  class: string;
  title: string;
  title_class?: string;
  subtitle_attribute?: string;
  subtitle_class?: string;
  metadata_attributes?: string[];
  metadata_class?: string;
  body_text?: string;
  rendered?: boolean;
  line_annotations?: CatalogWidgetLineAnnotations;
}

export type CatalogWidgetBodyFormat = "markdown";

export interface CatalogWidgetCalloutPreview {
  class: string;
  body_class?: string;
  kind_attribute: string;
  body_attribute: string;
  body_format?: CatalogWidgetBodyFormat;
}

export interface CatalogWidgetDetailsPreview {
  class: string;
  body_class?: string;
  title_attribute: string;
  open_attribute: string;
  body_attribute: string;
  body_format?: CatalogWidgetBodyFormat;
}

export interface CatalogWidgetTabsPreview {
  class: string;
  list_class: string;
  tab_class: string;
  active_class?: string;
  panels_class: string;
  panel_class: string;
  hidden_class?: string;
  titles_attribute: string;
  bodies_attribute: string;
  body_format?: CatalogWidgetBodyFormat;
}

export type CatalogWidgetPreview =
  | { kind: "badge"; badge: CatalogWidgetBadgePreview }
  | { kind: "reference"; reference: CatalogWidgetReferencePreview }
  | { kind: "card"; card: CatalogWidgetCardPreview }
  | { kind: "callout"; callout: CatalogWidgetCalloutPreview }
  | { kind: "details"; details: CatalogWidgetDetailsPreview }
  | { kind: "tabs"; tabs: CatalogWidgetTabsPreview };

export interface CatalogWidget {
  plugin_id: string;
  id: string;
  name: string;
  inline: boolean;
  syntax: { kind: WidgetSyntaxKind; name?: string; multiline?: boolean };
  attributes: CatalogWidgetAttribute[];
  settings: CatalogWidgetSetting[];
  constraints?: CatalogWidgetConstraint[];
  preview: CatalogWidgetPreview;
}

export interface CatalogWidgetProblem {
  plugin_id: string;
  message: string;
}

export interface ParsedMacroAttribute {
  name: string;
  value: string;
}

export interface ParsedMacro {
  name: string;
  raw: string;
  attributes: ParsedMacroAttribute[];
}

export interface MatchedWidgetSource {
  raw: string;
  widget: CatalogWidget;
}
