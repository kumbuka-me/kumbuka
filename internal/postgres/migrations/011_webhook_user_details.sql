ALTER TABLE webhooks
    ADD COLUMN include_user_details boolean NOT NULL DEFAULT false;

ALTER TABLE application_settings
    DROP COLUMN integration_user_directory_enabled;
