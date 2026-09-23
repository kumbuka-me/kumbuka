ALTER TABLE application_settings
ADD COLUMN editor_toolbar_overrides jsonb NOT NULL DEFAULT '[]'::jsonb;
