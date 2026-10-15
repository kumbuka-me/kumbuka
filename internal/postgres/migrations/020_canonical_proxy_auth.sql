-- Normalize stored authentication and profile-source modes without changing
-- trusted_proxy_identities or identity bindings.
ALTER TABLE public.application_settings
    DROP CONSTRAINT IF EXISTS application_settings_auth_mode_check;

UPDATE public.application_settings
SET auth_mode = 'proxy'
WHERE auth_mode = 'trusted-proxy';

ALTER TABLE public.application_settings
    ADD CONSTRAINT application_settings_auth_mode_check
    CHECK (auth_mode IN ('none', 'local', 'oidc', 'proxy'));

ALTER TABLE public.users
    DROP CONSTRAINT IF EXISTS users_profile_source_check;

UPDATE public.users
SET profile_source = 'proxy'
WHERE profile_source = 'trusted-proxy';

ALTER TABLE public.users
    ADD CONSTRAINT users_profile_source_check
    CHECK (profile_source IN ('local', 'oidc', 'proxy'));
