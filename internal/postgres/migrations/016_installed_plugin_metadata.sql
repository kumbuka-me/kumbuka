-- The application backfills archive metadata before applying this DDL, in the
-- same migration transaction. Empty databases can apply this file directly.
ALTER TABLE plugin_installations
    DROP CONSTRAINT IF EXISTS plugin_installations_check,
    DROP CONSTRAINT plugin_installations_source_check,
    ADD COLUMN IF NOT EXISTS manifest jsonb,
    ADD COLUMN IF NOT EXISTS digest bytea,
    ADD COLUMN IF NOT EXISTS readme text;
ALTER TABLE plugin_installations
    DROP COLUMN source,
    ALTER COLUMN manifest SET NOT NULL,
    ALTER COLUMN digest SET NOT NULL,
    ALTER COLUMN readme SET NOT NULL,
    ALTER COLUMN package SET NOT NULL,
    ADD CONSTRAINT plugin_installations_digest_check CHECK (octet_length(digest)=32),
    ADD CONSTRAINT plugin_installations_package_check CHECK (octet_length(package) BETWEEN 1 AND 16777216),
    ADD CONSTRAINT plugin_installations_manifest_check CHECK (
        coalesce(manifest->>'ID','')=plugin_id AND coalesce(manifest->>'Version','')<>'');
