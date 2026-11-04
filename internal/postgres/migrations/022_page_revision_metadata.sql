-- Capture full editable page metadata alongside Markdown and link page audits to exact revisions.
ALTER TABLE page_revisions
    ADD COLUMN metadata_snapshot jsonb,
    ADD COLUMN metadata_changes text[] NOT NULL DEFAULT '{}';

ALTER TABLE audit_events
    ADD COLUMN revision_number integer;
