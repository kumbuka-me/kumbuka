CREATE TABLE plugin_update_announcements (
  plugin_id text NOT NULL,
  version text NOT NULL,
  announced_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (plugin_id, version)
);

COMMENT ON TABLE plugin_update_announcements IS 'Plugin release identities already announced to administrators.';
COMMENT ON COLUMN plugin_update_announcements.plugin_id IS 'Stable plugin identifier from the first-party catalog.';
COMMENT ON COLUMN plugin_update_announcements.version IS 'Compatible catalog version already announced to administrators.';
