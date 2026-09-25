ALTER TABLE notifications
    ADD COLUMN actor_id bigint REFERENCES users(id) ON DELETE SET NULL,
    ADD COLUMN source_type text NOT NULL DEFAULT 'core',
    ADD COLUMN source_id text NOT NULL DEFAULT '',
    ADD COLUMN source_name text NOT NULL DEFAULT 'Kumbuka',
    ADD COLUMN idempotency_key text NOT NULL DEFAULT '',
    ADD CONSTRAINT notifications_source_type_check CHECK (source_type IN ('core','plugin'));

CREATE UNIQUE INDEX notifications_plugin_idempotency
ON notifications(source_id,user_id,idempotency_key)
WHERE source_type='plugin' AND idempotency_key<>'';

ALTER TABLE application_settings
    ADD COLUMN integration_user_directory_enabled boolean NOT NULL DEFAULT false;
