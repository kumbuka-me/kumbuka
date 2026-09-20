ALTER TABLE page_comments
  ADD COLUMN parent_id bigint REFERENCES page_comments (id) ON DELETE SET NULL,
  ADD COLUMN quote text NOT NULL DEFAULT '';

CREATE INDEX page_comments_parent_idx ON page_comments (parent_id);

COMMENT ON COLUMN page_comments.parent_id IS 'Optional comment this discussion item replies to.';
COMMENT ON COLUMN page_comments.quote IS 'Optional quoted excerpt captured when the reply was created.';
