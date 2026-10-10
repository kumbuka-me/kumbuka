package plugin

// validateSettings validates generated controls against declared attributes.
func (v editorWidgetValidator) validateSettings(settings []EditorWidgetSetting) error {
	for _, setting := range settings {
		if err := validateEditorWidgetSetting(setting, v.attributes); err != nil {
			return err
		}
	}
	return nil
}

// validateConstraints validates cross-attribute constraints.
func (v editorWidgetValidator) validateConstraints(constraints []EditorWidgetConstraint) error {
	for _, constraint := range constraints {
		if err := validateEditorWidgetConstraint(constraint, v.attributes); err != nil {
			return err
		}
	}
	return nil
}

// validatePreview validates preview metadata against declared attributes.
func (v editorWidgetValidator) validatePreview(preview EditorWidgetPreview) error {
	return validateEditorWidgetPreview(preview, v.attributes)
}
