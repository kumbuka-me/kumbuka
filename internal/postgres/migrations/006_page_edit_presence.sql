CREATE TABLE page_edit_presence (
  page_id bigint NOT NULL REFERENCES pages(id) ON DELETE CASCADE,
  user_id bigint NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  updated_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (page_id,user_id)
);

CREATE INDEX page_edit_presence_updated_at_idx
ON page_edit_presence(updated_at);
