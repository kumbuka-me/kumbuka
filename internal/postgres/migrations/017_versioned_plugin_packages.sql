-- Package identity outlives an installation pointer: another running server may
-- still serve the previous version. Keep immutable archives in PostgreSQL rather
-- than retaining them in every manager's heap.
CREATE TABLE plugin_packages (
    plugin_id text NOT NULL,
    digest bytea NOT NULL CHECK (octet_length(digest)=32),
    package bytea NOT NULL CHECK (octet_length(package) BETWEEN 1 AND 16777216),
    PRIMARY KEY (plugin_id,digest)
);
INSERT INTO plugin_packages(plugin_id,digest,package)
SELECT plugin_id,digest,package FROM plugin_installations;
ALTER TABLE plugin_installations
    DROP COLUMN package,
    ADD CONSTRAINT plugin_installations_package_fk
        FOREIGN KEY (plugin_id,digest) REFERENCES plugin_packages(plugin_id,digest);
