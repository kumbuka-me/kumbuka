CREATE TABLE plugin_content_changes (
    id bigserial PRIMARY KEY,
    plugin_id text NOT NULL,
    actor_id bigint NOT NULL,
    page_snapshot jsonb NOT NULL,
    previous_markdown text NOT NULL,
    markdown text NOT NULL,
    attempts integer NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    available_at timestamptz NOT NULL DEFAULT now(),
    claimed_until timestamptz,
    last_error text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX plugin_content_changes_ready
ON plugin_content_changes(available_at, id);
