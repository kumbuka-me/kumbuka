CREATE TABLE page_review_comments (
  id bigserial PRIMARY KEY,
  request_id bigint NOT NULL REFERENCES page_review_requests (id) ON DELETE CASCADE,
  user_id bigint REFERENCES users (id) ON DELETE SET NULL,
  side text NOT NULL CHECK (side IN ('old', 'new')),
  start_line integer NOT NULL CHECK (start_line > 0),
  end_line integer NOT NULL CHECK (end_line >= start_line),
  body text NOT NULL DEFAULT '',
  is_suggestion boolean NOT NULL DEFAULT false,
  original_text text NOT NULL DEFAULT '',
  replacement_text text NOT NULL DEFAULT '',
  applied_by bigint REFERENCES users (id) ON DELETE SET NULL,
  applied_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  CHECK (body <> '' OR is_suggestion),
  CHECK (NOT is_suggestion OR side = 'new')
);

CREATE INDEX page_review_comments_request_idx
  ON page_review_comments (request_id, start_line, created_at, id);

CREATE INDEX page_review_comments_open_suggestion_idx
  ON page_review_comments (request_id, id)
  WHERE is_suggestion AND applied_at IS NULL;
