ALTER TABLE public.users
  ADD COLUMN profile_source text DEFAULT 'local'::text NOT NULL,
  ADD COLUMN username_overridden boolean DEFAULT false NOT NULL,
  ADD COLUMN email_overridden boolean DEFAULT false NOT NULL,
  ADD COLUMN display_name_overridden boolean DEFAULT false NOT NULL,
  ADD CONSTRAINT users_profile_source_check
    CHECK (profile_source = ANY (ARRAY['local'::text, 'oidc'::text, 'trusted-proxy'::text]));

ALTER TABLE public.oidc_identities
  ADD COLUMN username text DEFAULT ''::text NOT NULL,
  ADD COLUMN email text DEFAULT ''::text NOT NULL,
  ADD COLUMN display_name text DEFAULT ''::text NOT NULL,
  ADD COLUMN last_seen_at timestamp with time zone DEFAULT now() NOT NULL;

UPDATE public.oidc_identities AS identity
SET username = users.username,
    email = users.email,
    display_name = users.display_name,
    last_seen_at = identity.created_at
FROM public.users AS users
WHERE users.id = identity.user_id;

UPDATE public.users AS users
SET profile_source = 'oidc'
WHERE EXISTS (SELECT 1 FROM public.oidc_identities identity WHERE identity.user_id = users.id);

CREATE TABLE public.trusted_proxy_identities (
  username text PRIMARY KEY,
  user_id bigint UNIQUE NOT NULL REFERENCES public.users(id) ON DELETE CASCADE,
  email text DEFAULT ''::text NOT NULL,
  display_name text DEFAULT ''::text NOT NULL,
  created_at timestamp with time zone DEFAULT now() NOT NULL,
  last_seen_at timestamp with time zone DEFAULT now() NOT NULL
);

CREATE INDEX trusted_proxy_identities_user_idx ON public.trusted_proxy_identities(user_id);

CREATE TABLE public.retired_trusted_proxy_identities (
  username text PRIMARY KEY,
  user_id bigint REFERENCES public.users(id) ON DELETE SET NULL,
  retired_at timestamp with time zone DEFAULT now() NOT NULL
);
