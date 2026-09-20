ALTER TABLE application_settings
  ADD COLUMN brand_logo_content_type text NOT NULL DEFAULT '',
  ADD COLUMN brand_logo_data bytea,
  ADD CONSTRAINT application_settings_brand_logo_pair_check CHECK (
    (brand_logo_content_type = '' AND brand_logo_data IS NULL)
    OR
    (brand_logo_content_type <> '' AND brand_logo_data IS NOT NULL)
  );

COMMENT ON COLUMN application_settings.brand_logo_content_type IS 'Validated MIME type of the optional custom application logo.';
COMMENT ON COLUMN application_settings.brand_logo_data IS 'Binary contents of the optional custom application logo.';
