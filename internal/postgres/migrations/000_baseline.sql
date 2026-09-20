-- Kumbuka database baseline for fresh installations.
-- Continue with 001_<name>.sql, 002_<name>.sql, and so on.

CREATE TABLE users (
  id bigserial PRIMARY KEY,
  username text NOT NULL UNIQUE,
  email text NOT NULL DEFAULT '',
  display_name text NOT NULL DEFAULT '',
  role text NOT NULL DEFAULT 'viewer' CHECK (role IN ('admin', 'editor', 'viewer')),
  enabled boolean NOT NULL DEFAULT true,
  session_version bigint NOT NULL DEFAULT 1,
  oidc_admin_observed boolean NOT NULL DEFAULT false,
  oidc_external_admin boolean NOT NULL DEFAULT false,
  trusted_proxy_admin_observed boolean NOT NULL DEFAULT false,
  trusted_proxy_external_admin boolean NOT NULL DEFAULT false,
  created_at timestamptz NOT NULL DEFAULT now(),
  last_login timestamptz
);

CREATE TABLE wiki_groups (
  id bigserial PRIMARY KEY,
  name text NOT NULL UNIQUE,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX wiki_groups_name_ci_idx ON wiki_groups (lower(name));

CREATE TABLE user_groups (
  user_id bigint NOT NULL REFERENCES users (id) ON DELETE CASCADE,
  group_id bigint NOT NULL REFERENCES wiki_groups (id) ON DELETE CASCADE,
  PRIMARY KEY (user_id, group_id)
);

CREATE INDEX user_groups_group_idx ON user_groups (group_id, user_id);

CREATE TABLE application_settings (
  singleton boolean PRIMARY KEY DEFAULT true CHECK (singleton),
  allow_user_registration boolean NOT NULL DEFAULT true,
  content_language text NOT NULL DEFAULT 'en',
  discussions_enabled boolean NOT NULL DEFAULT true,
  auth_mode text NOT NULL DEFAULT 'none' CHECK (auth_mode IN ('none', 'local', 'trusted-proxy', 'oidc')),
  oidc_issuer text NOT NULL DEFAULT '',
  oidc_client_id text NOT NULL DEFAULT '',
  oidc_group_claim text NOT NULL DEFAULT 'groups',
  oidc_group_sync boolean NOT NULL DEFAULT false,
  oidc_groups_authoritative boolean NOT NULL DEFAULT true,
  oidc_admin_group text NOT NULL DEFAULT '',
  trusted_username_headers text[] NOT NULL DEFAULT ARRAY['X-Forwarded-User', 'X-Auth-Request-User', 'Remote-User']::text[],
  trusted_email_headers text[] NOT NULL DEFAULT ARRAY['X-Forwarded-Email', 'X-Auth-Request-Email', 'X-Authentik-Email']::text[],
  trusted_display_name_headers text[] NOT NULL DEFAULT ARRAY['X-Forwarded-Name', 'X-Auth-Request-Preferred-Username', 'X-Authentik-Name']::text[],
  trusted_group_headers text[] NOT NULL DEFAULT ARRAY['X-Forwarded-Groups', 'X-Auth-Request-Groups']::text[],
  trusted_admin_group text NOT NULL DEFAULT '',
  pdf_url text NOT NULL DEFAULT '',
  external_links jsonb NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(external_links) = 'array'),
  default_typography_size text NOT NULL DEFAULT 'compact' CHECK (default_typography_size IN ('compact', 'standard', 'large')),
  robots_policy text NOT NULL DEFAULT 'disallow' CHECK (robots_policy IN ('allow', 'disallow', 'none')),
  updated_at timestamptz NOT NULL DEFAULT now()
);

INSERT INTO application_settings (singleton) VALUES (true);

CREATE TABLE user_preferences (
  user_id bigint PRIMARY KEY REFERENCES users (id) ON DELETE CASCADE,
  theme text NOT NULL DEFAULT '',
  show_page_contents boolean NOT NULL DEFAULT true,
  navigation_style text NOT NULL DEFAULT 'sidebar' CHECK (navigation_style IN ('sidebar', 'topbar', 'tree')),
  navigation_density text NOT NULL DEFAULT 'comfortable' CHECK (navigation_density IN ('comfortable', 'compact')),
  typography_size text NOT NULL DEFAULT '' CHECK (typography_size IN ('', 'compact', 'standard', 'large')),
  sidebar_width integer NOT NULL DEFAULT 280 CHECK (sidebar_width BETWEEN 220 AND 420),
  show_navigation_guides boolean NOT NULL DEFAULT true,
  remember_navigation_state boolean NOT NULL DEFAULT true,
  show_navigation_page_counts boolean NOT NULL DEFAULT false,
  expanded_navigation text[] NOT NULL DEFAULT '{}',
  hidden_plugin_widgets text[] NOT NULL DEFAULT '{}',
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE pages (
  id bigserial PRIMARY KEY,
  slug text NOT NULL UNIQUE,
  title text NOT NULL,
  markdown_content text NOT NULL DEFAULT '',
  content_language text NOT NULL DEFAULT '',
  plugin_usage jsonb,
  rendered_html text NOT NULL DEFAULT '',
  rendered_contents jsonb NOT NULL DEFAULT '[]'::jsonb,
  render_fingerprint text NOT NULL DEFAULT '',
  rendered_at timestamptz,
  created_by bigint REFERENCES users (id),
  updated_by bigint REFERENCES users (id),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  view_count bigint NOT NULL DEFAULT 0,
  deleted_at timestamptz,
  deleted_by bigint REFERENCES users (id),
  status text NOT NULL DEFAULT 'verified' CHECK (status IN ('draft', 'verified', 'deprecated', 'archived')),
  owner_group_id bigint REFERENCES wiki_groups (id) ON DELETE SET NULL,
  last_reviewed_at timestamptz,
  review_interval_days integer NOT NULL DEFAULT 0 CHECK (review_interval_days >= 0),
  deprecated_target text NOT NULL DEFAULT '',
  search_vector tsvector GENERATED ALWAYS AS (
    setweight(to_tsvector('english', coalesce(title, '')), 'A')
    || setweight(to_tsvector('english', coalesce(markdown_content, '')), 'B')
  ) STORED
);

CREATE INDEX pages_search_idx ON pages USING gin (search_vector);
CREATE INDEX pages_updated_idx ON pages (updated_at DESC);
CREATE INDEX pages_deleted_at_idx ON pages (deleted_at) WHERE deleted_at IS NOT NULL;
CREATE INDEX pages_status_idx ON pages (status) WHERE deleted_at IS NULL;
CREATE INDEX pages_review_due_idx ON pages (last_reviewed_at, review_interval_days)
  WHERE deleted_at IS NULL AND review_interval_days > 0;

CREATE TABLE page_revisions (
  id bigserial PRIMARY KEY,
  page_id bigint NOT NULL REFERENCES pages (id) ON DELETE CASCADE,
  revision_number integer NOT NULL,
  markdown_content text NOT NULL,
  created_by bigint REFERENCES users (id),
  created_at timestamptz NOT NULL DEFAULT now(),
  message text NOT NULL DEFAULT '',
  UNIQUE (page_id, revision_number)
);

CREATE TABLE tags (
  id bigserial PRIMARY KEY,
  name text NOT NULL UNIQUE
);

CREATE TABLE page_tags (
  page_id bigint NOT NULL REFERENCES pages (id) ON DELETE CASCADE,
  tag_id bigint NOT NULL REFERENCES tags (id) ON DELETE CASCADE,
  PRIMARY KEY (page_id, tag_id)
);

CREATE TABLE page_groups (
  page_id bigint NOT NULL REFERENCES pages (id) ON DELETE CASCADE,
  group_id bigint NOT NULL REFERENCES wiki_groups (id) ON DELETE CASCADE,
  PRIMARY KEY (page_id, group_id)
);

CREATE INDEX page_groups_group_idx ON page_groups (group_id, page_id);

CREATE TABLE api_tokens (
  id bigserial PRIMARY KEY,
  name text NOT NULL,
  token_hash text NOT NULL UNIQUE,
  created_by bigint REFERENCES users (id),
  user_id bigint REFERENCES users (id) ON DELETE CASCADE,
  created_at timestamptz NOT NULL DEFAULT now(),
  last_used timestamptz,
  expires_at timestamptz
);

CREATE INDEX api_tokens_user_idx ON api_tokens (user_id, created_at DESC);

CREATE TABLE favorites (
  user_id bigint NOT NULL REFERENCES users (id) ON DELETE CASCADE,
  page_id bigint NOT NULL REFERENCES pages (id) ON DELETE CASCADE,
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (user_id, page_id)
);

CREATE TABLE page_views (
  user_id bigint NOT NULL REFERENCES users (id) ON DELETE CASCADE,
  page_id bigint NOT NULL REFERENCES pages (id) ON DELETE CASCADE,
  viewed_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (user_id, page_id)
);

CREATE TABLE page_links (
  source_page_id bigint NOT NULL REFERENCES pages (id) ON DELETE CASCADE,
  target_slug text NOT NULL,
  PRIMARY KEY (source_page_id, target_slug)
);

CREATE INDEX page_links_target_idx ON page_links (target_slug);

CREATE TABLE page_properties (
  page_id bigint NOT NULL REFERENCES pages (id) ON DELETE CASCADE,
  key text NOT NULL,
  value text NOT NULL DEFAULT '',
  PRIMARY KEY (page_id, key)
);

CREATE UNIQUE INDEX page_properties_key_ci_idx ON page_properties (page_id, lower(key));

CREATE TABLE images (
  id bigserial PRIMARY KEY,
  filename text NOT NULL,
  content_type text NOT NULL,
  data bytea NOT NULL,
  size_bytes bigint NOT NULL CHECK (size_bytes > 0),
  uploaded_by bigint REFERENCES users (id) ON DELETE SET NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX images_created_idx ON images (created_at DESC);

CREATE TABLE attachments (
  id bigserial PRIMARY KEY,
  filename text NOT NULL,
  content_type text NOT NULL,
  data bytea NOT NULL,
  size_bytes bigint NOT NULL CHECK (size_bytes > 0),
  uploaded_by bigint REFERENCES users (id) ON DELETE SET NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX attachments_created_idx ON attachments (created_at DESC);

CREATE TABLE navigation_icons (
  path text PRIMARY KEY,
  icon text NOT NULL DEFAULT ''
);

CREATE TABLE page_templates (
  id bigserial PRIMARY KEY,
  name text NOT NULL UNIQUE,
  description text NOT NULL DEFAULT '',
  markdown_content text NOT NULL DEFAULT '',
  path_prefix text NOT NULL DEFAULT '',
  icon text NOT NULL DEFAULT '',
  tags text[] NOT NULL DEFAULT '{}',
  status text NOT NULL DEFAULT 'verified' CHECK (status IN ('draft', 'verified', 'deprecated', 'archived')),
  owner_group_id bigint REFERENCES wiki_groups (id) ON DELETE SET NULL,
  review_interval_days integer NOT NULL DEFAULT 0 CHECK (review_interval_days >= 0),
  properties jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(properties) = 'object'),
  fields jsonb NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(fields) = 'array'),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX page_templates_name_ci_idx ON page_templates (lower(name));

CREATE TABLE page_aliases (
  alias text PRIMARY KEY,
  page_id bigint NOT NULL REFERENCES pages (id) ON DELETE CASCADE,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX page_aliases_page_idx ON page_aliases (page_id);

CREATE TABLE audit_events (
  id bigserial PRIMARY KEY,
  user_id bigint REFERENCES users (id) ON DELETE SET NULL,
  action text NOT NULL,
  object_type text NOT NULL DEFAULT '',
  object_key text NOT NULL DEFAULT '',
  detail text NOT NULL DEFAULT '',
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX audit_events_created_idx ON audit_events (created_at DESC, id DESC);

CREATE TABLE saved_searches (
  id bigserial PRIMARY KEY,
  user_id bigint NOT NULL REFERENCES users (id) ON DELETE CASCADE,
  name text NOT NULL,
  query text NOT NULL,
  pinned boolean NOT NULL DEFAULT true,
  created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (user_id, name)
);

CREATE UNIQUE INDEX saved_searches_user_name_ci_idx ON saved_searches (user_id, lower(name));

CREATE TABLE notifications (
  id bigserial PRIMARY KEY,
  user_id bigint NOT NULL REFERENCES users (id) ON DELETE CASCADE,
  kind text NOT NULL DEFAULT 'info',
  title text NOT NULL,
  body text NOT NULL DEFAULT '',
  url text NOT NULL DEFAULT '',
  read_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX notifications_user_idx ON notifications (user_id, read_at, created_at DESC);

CREATE TABLE page_comments (
  id bigserial PRIMARY KEY,
  page_id bigint NOT NULL REFERENCES pages (id) ON DELETE CASCADE,
  user_id bigint REFERENCES users (id) ON DELETE SET NULL,
  anchor text NOT NULL DEFAULT '',
  body text NOT NULL,
  resolved_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX page_comments_page_idx ON page_comments (page_id, resolved_at, created_at);

CREATE TABLE oidc_identities (
  issuer text NOT NULL,
  subject text NOT NULL,
  user_id bigint NOT NULL REFERENCES users (id) ON DELETE CASCADE,
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (issuer, subject),
  UNIQUE (user_id, issuer)
);

CREATE INDEX oidc_identities_user_idx ON oidc_identities (user_id);

CREATE TABLE pending_oidc_identities (
  id bigserial PRIMARY KEY,
  issuer text NOT NULL,
  subject text NOT NULL,
  username text NOT NULL DEFAULT '',
  email text NOT NULL DEFAULT '',
  display_name text NOT NULL DEFAULT '',
  status text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'rejected')),
  first_seen_at timestamptz NOT NULL DEFAULT now(),
  last_seen_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (issuer, subject)
);

CREATE INDEX pending_oidc_identities_status_idx
  ON pending_oidc_identities (status, last_seen_at DESC);

CREATE TABLE oidc_group_mappings (
  oidc_group text PRIMARY KEY,
  group_id bigint NOT NULL REFERENCES wiki_groups (id) ON DELETE CASCADE
);

CREATE INDEX oidc_group_mappings_group_idx ON oidc_group_mappings (group_id);

CREATE TABLE local_credentials (
  user_id bigint PRIMARY KEY REFERENCES users (id) ON DELETE CASCADE,
  password_hash text NOT NULL,
  enabled boolean NOT NULL DEFAULT true,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE local_sessions (
  token_hash text PRIMARY KEY,
  user_id bigint NOT NULL REFERENCES users (id) ON DELETE CASCADE,
  expires_at timestamptz NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX local_sessions_user_idx ON local_sessions (user_id);
CREATE INDEX local_sessions_expiry_idx ON local_sessions (expires_at);

CREATE TABLE page_share_links (
  id bigserial PRIMARY KEY,
  page_id bigint NOT NULL REFERENCES pages (id) ON DELETE CASCADE,
  token_hash text NOT NULL UNIQUE,
  created_by bigint REFERENCES users (id) ON DELETE SET NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  revoked_at timestamptz
);

CREATE INDEX page_share_links_page_idx
  ON page_share_links (page_id, created_at DESC);

CREATE TABLE page_drafts (
  id bigserial PRIMARY KEY,
  user_id bigint NOT NULL REFERENCES users (id) ON DELETE CASCADE,
  draft_key text NOT NULL,
  page_id bigint REFERENCES pages (id) ON DELETE CASCADE,
  base_revision integer NOT NULL DEFAULT 0,
  title text NOT NULL DEFAULT '',
  slug text NOT NULL DEFAULT '',
  form_values jsonb NOT NULL DEFAULT '{}'::jsonb,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (user_id, draft_key)
);

CREATE INDEX page_drafts_user_updated_idx
  ON page_drafts (user_id, updated_at DESC);

CREATE INDEX page_drafts_page_idx
  ON page_drafts (page_id)
  WHERE page_id IS NOT NULL;

CREATE TABLE pdf_headers (
  id bigserial PRIMARY KEY,
  name text NOT NULL,
  value text NOT NULL,
  sensitive boolean NOT NULL DEFAULT false
);

CREATE UNIQUE INDEX pdf_headers_name_ci_idx ON pdf_headers (lower(name));

CREATE TABLE page_watches (
  user_id bigint NOT NULL REFERENCES users (id) ON DELETE CASCADE,
  path text NOT NULL,
  scope text NOT NULL CHECK (scope IN ('page', 'subtree')),
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (user_id, path)
);

CREATE INDEX page_watches_path_idx ON page_watches (path, scope, user_id);

CREATE TABLE page_access_rules (
  id bigserial PRIMARY KEY,
  path text NOT NULL,
  group_id bigint NOT NULL REFERENCES wiki_groups (id) ON DELETE CASCADE,
  access text NOT NULL CHECK (access IN ('view', 'edit')),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (path, group_id)
);

CREATE INDEX page_access_rules_path_idx ON page_access_rules (path);

CREATE TABLE page_review_requests (
  id bigserial PRIMARY KEY,
  page_id bigint NOT NULL REFERENCES pages (id) ON DELETE CASCADE,
  revision_number integer NOT NULL,
  requested_by bigint NOT NULL REFERENCES users (id) ON DELETE CASCADE,
  reviewed_by bigint REFERENCES users (id) ON DELETE SET NULL,
  reviewer_group_id bigint REFERENCES wiki_groups (id) ON DELETE SET NULL,
  status text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'changes_requested', 'approved', 'canceled', 'superseded')),
  note text NOT NULL DEFAULT '',
  decision_note text NOT NULL DEFAULT '',
  previous_status text NOT NULL DEFAULT 'draft',
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX page_review_requests_pending_idx
  ON page_review_requests (page_id)
  WHERE status = 'pending';

CREATE INDEX page_review_requests_page_idx
  ON page_review_requests (page_id, created_at DESC, id DESC);

CREATE TABLE page_review_request_reviewers (
  request_id bigint NOT NULL REFERENCES page_review_requests (id) ON DELETE CASCADE,
  user_id bigint NOT NULL REFERENCES users (id) ON DELETE CASCADE,
  PRIMARY KEY (request_id, user_id)
);

CREATE INDEX page_review_request_reviewers_user_idx
  ON page_review_request_reviewers (user_id, request_id);

CREATE TABLE webhooks (
  id bigserial PRIMARY KEY,
  name text NOT NULL UNIQUE,
  url text NOT NULL,
  events text[] NOT NULL DEFAULT '{}',
  enabled boolean NOT NULL DEFAULT true,
  body_template text NOT NULL DEFAULT '',
  retry_enabled boolean NOT NULL DEFAULT false,
  retry_count integer NOT NULL DEFAULT 2 CHECK (retry_count >= 0),
  retry_backoff_ms bigint NOT NULL DEFAULT 1000 CHECK (retry_backoff_ms >= 0),
  retry_max_backoff_ms bigint NOT NULL DEFAULT 30000 CHECK (retry_max_backoff_ms >= 0),
  retry_jitter boolean NOT NULL DEFAULT true,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX webhooks_name_ci_idx ON webhooks (lower(name));

CREATE TABLE webhook_headers (
  id bigserial PRIMARY KEY,
  webhook_id bigint NOT NULL REFERENCES webhooks (id) ON DELETE CASCADE,
  name text NOT NULL,
  value text NOT NULL,
  sensitive boolean NOT NULL DEFAULT false,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX webhook_headers_name_ci_idx
  ON webhook_headers (webhook_id, lower(name));

CREATE TABLE webhook_deliveries (
  id bigserial PRIMARY KEY,
  webhook_id bigint NOT NULL REFERENCES webhooks (id) ON DELETE CASCADE,
  event text NOT NULL,
  status_code integer NOT NULL DEFAULT 0,
  error text NOT NULL DEFAULT '',
  attempts integer NOT NULL DEFAULT 1 CHECK (attempts >= 0),
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX webhook_deliveries_recent_idx
  ON webhook_deliveries (created_at DESC, id DESC);

CREATE TABLE plugin_values (
  plugin_id text NOT NULL,
  namespace text NOT NULL CHECK (namespace IN ('settings', 'data')),
  key text NOT NULL CHECK (octet_length(key) BETWEEN 1 AND 256),
  value bytea NOT NULL CHECK (octet_length(value) <= 65536),
  PRIMARY KEY (plugin_id, namespace, key)
);

CREATE TABLE plugin_installations (
  plugin_id text PRIMARY KEY CHECK (octet_length(plugin_id) BETWEEN 1 AND 128),
  source text NOT NULL CHECK (source IN ('bundled', 'installed')),
  enabled boolean NOT NULL,
  package bytea,
  CHECK (
    (source = 'bundled' AND package IS NULL)
    OR (source = 'installed' AND package IS NOT NULL AND octet_length(package) BETWEEN 1 AND 16777216)
  )
);
