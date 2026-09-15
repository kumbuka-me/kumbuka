ALTER TABLE user_preferences
  DROP COLUMN IF EXISTS show_pinned_pages,
  DROP COLUMN IF EXISTS show_recently_viewed,
  ADD COLUMN IF NOT EXISTS hidden_plugin_widgets text[] NOT NULL DEFAULT '{}';
