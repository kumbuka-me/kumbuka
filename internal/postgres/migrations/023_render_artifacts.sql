-- Persist passive rendered artifacts only within their owning page revision.
CREATE TABLE IF NOT EXISTS page_render_artifacts (
    page_id bigint NOT NULL REFERENCES pages(id) ON DELETE CASCADE,
    page_updated_at timestamptz NOT NULL,
    digest char(64) NOT NULL,
    media_type text NOT NULL CHECK (media_type = 'image/svg+xml'),
    data bytea NOT NULL CHECK (octet_length(data) BETWEEN 16 AND 524288),
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (page_id, page_updated_at, digest)
);
CREATE INDEX IF NOT EXISTS page_render_artifacts_created_at_idx ON page_render_artifacts(created_at);
