ALTER TABLE page_comments
  ADD COLUMN suggestion_revision integer,
  ADD COLUMN suggestion_start_byte integer,
  ADD COLUMN suggestion_end_byte integer,
  ADD COLUMN suggestion_original text NOT NULL DEFAULT '',
  ADD COLUMN suggestion_replacement text NOT NULL DEFAULT '',
  ADD COLUMN suggestion_applied_by bigint REFERENCES users (id) ON DELETE SET NULL,
  ADD COLUMN suggestion_applied_at timestamptz;

ALTER TABLE page_comments
  ADD CONSTRAINT page_comments_suggestion_shape CHECK (
    (
      suggestion_revision IS NULL
      AND suggestion_start_byte IS NULL
      AND suggestion_end_byte IS NULL
      AND suggestion_original = ''
      AND suggestion_replacement = ''
      AND suggestion_applied_by IS NULL
      AND suggestion_applied_at IS NULL
    ) OR (
      parent_id IS NULL
      AND anchor <> ''
      AND suggestion_revision > 0
      AND suggestion_start_byte IS NOT NULL
      AND suggestion_end_byte IS NOT NULL
      AND suggestion_start_byte >= 0
      AND suggestion_end_byte > suggestion_start_byte
      AND suggestion_original <> ''
    )
  );

CREATE INDEX page_comments_open_suggestion_idx
  ON page_comments (page_id, suggestion_applied_at)
  WHERE suggestion_revision IS NOT NULL;

COMMENT ON COLUMN page_comments.suggestion_revision IS 'Revision number from which an inline Markdown suggestion was created.';
COMMENT ON COLUMN page_comments.suggestion_start_byte IS 'Zero-based inclusive byte offset of the suggested source range.';
COMMENT ON COLUMN page_comments.suggestion_end_byte IS 'Zero-based exclusive byte offset of the suggested source range.';
COMMENT ON COLUMN page_comments.suggestion_original IS 'Exact original Markdown source used for stale-suggestion detection.';
COMMENT ON COLUMN page_comments.suggestion_replacement IS 'Markdown replacement proposed by the inline suggestion.';
COMMENT ON COLUMN page_comments.suggestion_applied_by IS 'User who applied the inline suggestion.';
COMMENT ON COLUMN page_comments.suggestion_applied_at IS 'Timestamp when the inline suggestion was applied.';
