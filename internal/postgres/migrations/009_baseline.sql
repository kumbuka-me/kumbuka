--
-- PostgreSQL database dump
--


-- Dumped from database version 18.6
-- Dumped by pg_dump version 18.6

SET statement_timeout = 0;
SET lock_timeout = 0;
SET idle_in_transaction_session_timeout = 0;
SET transaction_timeout = 0;
SET client_encoding = 'UTF8';
SET standard_conforming_strings = on;
SELECT pg_catalog.set_config('search_path', '', false);
SET check_function_bodies = false;
SET xmloption = content;
SET client_min_messages = warning;
SET row_security = off;

SET default_table_access_method = heap;

--
-- Name: api_tokens; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.api_tokens (
    id bigint NOT NULL,
    name text NOT NULL,
    token_hash text NOT NULL,
    created_by bigint,
    user_id bigint,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    last_used timestamp with time zone,
    expires_at timestamp with time zone
);


--
-- Name: api_tokens_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.api_tokens_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: api_tokens_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.api_tokens_id_seq OWNED BY public.api_tokens.id;


--
-- Name: application_settings; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.application_settings (
    singleton boolean DEFAULT true NOT NULL,
    allow_user_registration boolean DEFAULT true NOT NULL,
    content_language text DEFAULT 'en'::text NOT NULL,
    discussions_enabled boolean DEFAULT true NOT NULL,
    auth_mode text DEFAULT 'none'::text NOT NULL,
    oidc_issuer text DEFAULT ''::text NOT NULL,
    oidc_client_id text DEFAULT ''::text NOT NULL,
    oidc_group_claim text DEFAULT 'groups'::text NOT NULL,
    oidc_group_sync boolean DEFAULT false NOT NULL,
    oidc_groups_authoritative boolean DEFAULT true NOT NULL,
    oidc_admin_group text DEFAULT ''::text NOT NULL,
    trusted_username_headers text[] DEFAULT ARRAY['X-Forwarded-User'::text, 'X-Auth-Request-User'::text, 'Remote-User'::text] NOT NULL,
    trusted_email_headers text[] DEFAULT ARRAY['X-Forwarded-Email'::text, 'X-Auth-Request-Email'::text, 'X-Authentik-Email'::text] NOT NULL,
    trusted_display_name_headers text[] DEFAULT ARRAY['X-Forwarded-Name'::text, 'X-Auth-Request-Preferred-Username'::text, 'X-Authentik-Name'::text] NOT NULL,
    trusted_group_headers text[] DEFAULT ARRAY['X-Forwarded-Groups'::text, 'X-Auth-Request-Groups'::text] NOT NULL,
    trusted_admin_group text DEFAULT ''::text NOT NULL,
    pdf_url text DEFAULT ''::text NOT NULL,
    external_links jsonb DEFAULT '[]'::jsonb NOT NULL,
    default_typography_size text DEFAULT 'compact'::text NOT NULL,
    robots_policy text DEFAULT 'disallow'::text NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    brand_logo_content_type text DEFAULT ''::text NOT NULL,
    brand_logo_data bytea,
    editor_toolbar_overrides jsonb DEFAULT '[]'::jsonb NOT NULL,
    CONSTRAINT application_settings_auth_mode_check CHECK ((auth_mode = ANY (ARRAY['none'::text, 'local'::text, 'trusted-proxy'::text, 'oidc'::text]))),
    CONSTRAINT application_settings_brand_logo_pair_check CHECK ((((brand_logo_content_type = ''::text) AND (brand_logo_data IS NULL)) OR ((brand_logo_content_type <> ''::text) AND (brand_logo_data IS NOT NULL)))),
    CONSTRAINT application_settings_default_typography_size_check CHECK ((default_typography_size = ANY (ARRAY['compact'::text, 'standard'::text, 'large'::text]))),
    CONSTRAINT application_settings_external_links_check CHECK ((jsonb_typeof(external_links) = 'array'::text)),
    CONSTRAINT application_settings_robots_policy_check CHECK ((robots_policy = ANY (ARRAY['allow'::text, 'disallow'::text, 'none'::text]))),
    CONSTRAINT application_settings_singleton_check CHECK (singleton)
);


--
-- Name: COLUMN application_settings.brand_logo_content_type; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.application_settings.brand_logo_content_type IS 'Validated MIME type of the optional custom application logo.';


--
-- Name: COLUMN application_settings.brand_logo_data; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.application_settings.brand_logo_data IS 'Binary contents of the optional custom application logo.';


--
-- Name: attachments; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.attachments (
    id bigint NOT NULL,
    filename text NOT NULL,
    content_type text NOT NULL,
    data bytea NOT NULL,
    size_bytes bigint NOT NULL,
    uploaded_by bigint,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT attachments_size_bytes_check CHECK ((size_bytes > 0))
);


--
-- Name: attachments_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.attachments_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: attachments_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.attachments_id_seq OWNED BY public.attachments.id;


--
-- Name: audit_events; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.audit_events (
    id bigint NOT NULL,
    user_id bigint,
    action text NOT NULL,
    object_type text DEFAULT ''::text NOT NULL,
    object_key text DEFAULT ''::text NOT NULL,
    detail text DEFAULT ''::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: audit_events_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.audit_events_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: audit_events_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.audit_events_id_seq OWNED BY public.audit_events.id;


--
-- Name: favorites; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.favorites (
    user_id bigint NOT NULL,
    page_id bigint NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: images; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.images (
    id bigint NOT NULL,
    filename text NOT NULL,
    content_type text NOT NULL,
    data bytea NOT NULL,
    size_bytes bigint NOT NULL,
    uploaded_by bigint,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT images_size_bytes_check CHECK ((size_bytes > 0))
);


--
-- Name: images_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.images_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: images_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.images_id_seq OWNED BY public.images.id;


--
-- Name: local_credentials; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.local_credentials (
    user_id bigint NOT NULL,
    password_hash text NOT NULL,
    enabled boolean DEFAULT true NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: local_sessions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.local_sessions (
    token_hash text NOT NULL,
    user_id bigint NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: navigation_icons; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.navigation_icons (
    path text NOT NULL,
    icon text DEFAULT ''::text NOT NULL
);


--
-- Name: notifications; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.notifications (
    id bigint NOT NULL,
    user_id bigint NOT NULL,
    kind text DEFAULT 'info'::text NOT NULL,
    title text NOT NULL,
    body text DEFAULT ''::text NOT NULL,
    url text DEFAULT ''::text NOT NULL,
    read_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: notifications_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.notifications_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: notifications_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.notifications_id_seq OWNED BY public.notifications.id;


--
-- Name: oidc_group_mappings; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.oidc_group_mappings (
    oidc_group text NOT NULL,
    group_id bigint NOT NULL
);


--
-- Name: oidc_identities; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.oidc_identities (
    issuer text NOT NULL,
    subject text NOT NULL,
    user_id bigint NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: page_access_rules; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.page_access_rules (
    id bigint NOT NULL,
    path text NOT NULL,
    group_id bigint NOT NULL,
    access text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT page_access_rules_access_check CHECK ((access = ANY (ARRAY['view'::text, 'edit'::text])))
);


--
-- Name: page_access_rules_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.page_access_rules_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: page_access_rules_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.page_access_rules_id_seq OWNED BY public.page_access_rules.id;


--
-- Name: page_aliases; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.page_aliases (
    alias text NOT NULL,
    page_id bigint NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: page_comments; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.page_comments (
    id bigint NOT NULL,
    page_id bigint NOT NULL,
    user_id bigint,
    anchor text DEFAULT ''::text NOT NULL,
    body text NOT NULL,
    resolved_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    parent_id bigint,
    quote text DEFAULT ''::text NOT NULL,
    suggestion_revision integer,
    suggestion_start_byte integer,
    suggestion_end_byte integer,
    suggestion_original text DEFAULT ''::text NOT NULL,
    suggestion_replacement text DEFAULT ''::text NOT NULL,
    suggestion_applied_by bigint,
    suggestion_applied_at timestamp with time zone,
    CONSTRAINT page_comments_suggestion_shape CHECK ((((suggestion_revision IS NULL) AND (suggestion_start_byte IS NULL) AND (suggestion_end_byte IS NULL) AND (suggestion_original = ''::text) AND (suggestion_replacement = ''::text) AND (suggestion_applied_by IS NULL) AND (suggestion_applied_at IS NULL)) OR ((parent_id IS NULL) AND (anchor <> ''::text) AND (suggestion_revision > 0) AND (suggestion_start_byte IS NOT NULL) AND (suggestion_end_byte IS NOT NULL) AND (suggestion_start_byte >= 0) AND (suggestion_end_byte > suggestion_start_byte) AND (suggestion_original <> ''::text))))
);


--
-- Name: COLUMN page_comments.parent_id; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.page_comments.parent_id IS 'Optional comment this discussion item replies to.';


--
-- Name: COLUMN page_comments.quote; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.page_comments.quote IS 'Optional quoted excerpt captured when the reply was created.';


--
-- Name: COLUMN page_comments.suggestion_revision; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.page_comments.suggestion_revision IS 'Revision number from which an inline Markdown suggestion was created.';


--
-- Name: COLUMN page_comments.suggestion_start_byte; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.page_comments.suggestion_start_byte IS 'Zero-based inclusive byte offset of the suggested source range.';


--
-- Name: COLUMN page_comments.suggestion_end_byte; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.page_comments.suggestion_end_byte IS 'Zero-based exclusive byte offset of the suggested source range.';


--
-- Name: COLUMN page_comments.suggestion_original; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.page_comments.suggestion_original IS 'Exact original Markdown source used for stale-suggestion detection.';


--
-- Name: COLUMN page_comments.suggestion_replacement; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.page_comments.suggestion_replacement IS 'Markdown replacement proposed by the inline suggestion.';


--
-- Name: COLUMN page_comments.suggestion_applied_by; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.page_comments.suggestion_applied_by IS 'User who applied the inline suggestion.';


--
-- Name: COLUMN page_comments.suggestion_applied_at; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.page_comments.suggestion_applied_at IS 'Timestamp when the inline suggestion was applied.';


--
-- Name: page_comments_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.page_comments_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: page_comments_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.page_comments_id_seq OWNED BY public.page_comments.id;


--
-- Name: page_drafts; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.page_drafts (
    id bigint NOT NULL,
    user_id bigint NOT NULL,
    draft_key text NOT NULL,
    page_id bigint,
    base_revision integer DEFAULT 0 NOT NULL,
    title text DEFAULT ''::text NOT NULL,
    slug text DEFAULT ''::text NOT NULL,
    form_values jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: page_drafts_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.page_drafts_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: page_drafts_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.page_drafts_id_seq OWNED BY public.page_drafts.id;


--
-- Name: page_edit_presence; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.page_edit_presence (
    page_id bigint NOT NULL,
    user_id bigint NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: page_groups; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.page_groups (
    page_id bigint NOT NULL,
    group_id bigint NOT NULL
);


--
-- Name: page_links; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.page_links (
    source_page_id bigint NOT NULL,
    target_slug text NOT NULL
);


--
-- Name: page_properties; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.page_properties (
    page_id bigint NOT NULL,
    key text NOT NULL,
    value text DEFAULT ''::text NOT NULL
);


--
-- Name: page_review_comments; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.page_review_comments (
    id bigint NOT NULL,
    request_id bigint NOT NULL,
    user_id bigint,
    side text NOT NULL,
    start_line integer NOT NULL,
    end_line integer NOT NULL,
    body text DEFAULT ''::text NOT NULL,
    is_suggestion boolean DEFAULT false NOT NULL,
    original_text text DEFAULT ''::text NOT NULL,
    replacement_text text DEFAULT ''::text NOT NULL,
    applied_by bigint,
    applied_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT page_review_comments_check CHECK ((end_line >= start_line)),
    CONSTRAINT page_review_comments_check1 CHECK (((body <> ''::text) OR is_suggestion)),
    CONSTRAINT page_review_comments_check2 CHECK (((NOT is_suggestion) OR (side = 'new'::text))),
    CONSTRAINT page_review_comments_side_check CHECK ((side = ANY (ARRAY['old'::text, 'new'::text]))),
    CONSTRAINT page_review_comments_start_line_check CHECK ((start_line > 0))
);


--
-- Name: page_review_comments_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.page_review_comments_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: page_review_comments_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.page_review_comments_id_seq OWNED BY public.page_review_comments.id;


--
-- Name: page_review_request_reviewers; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.page_review_request_reviewers (
    request_id bigint NOT NULL,
    user_id bigint NOT NULL
);


--
-- Name: page_review_requests; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.page_review_requests (
    id bigint NOT NULL,
    page_id bigint NOT NULL,
    revision_number integer NOT NULL,
    requested_by bigint NOT NULL,
    reviewed_by bigint,
    reviewer_group_id bigint,
    status text DEFAULT 'pending'::text NOT NULL,
    note text DEFAULT ''::text NOT NULL,
    decision_note text DEFAULT ''::text NOT NULL,
    previous_status text DEFAULT 'draft'::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT page_review_requests_status_check CHECK ((status = ANY (ARRAY['pending'::text, 'changes_requested'::text, 'approved'::text, 'canceled'::text, 'superseded'::text])))
);


--
-- Name: page_review_requests_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.page_review_requests_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: page_review_requests_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.page_review_requests_id_seq OWNED BY public.page_review_requests.id;


--
-- Name: page_revisions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.page_revisions (
    id bigint NOT NULL,
    page_id bigint NOT NULL,
    revision_number integer NOT NULL,
    markdown_content text NOT NULL,
    created_by bigint,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    message text DEFAULT ''::text NOT NULL
);


--
-- Name: page_revisions_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.page_revisions_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: page_revisions_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.page_revisions_id_seq OWNED BY public.page_revisions.id;


--
-- Name: page_tags; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.page_tags (
    page_id bigint NOT NULL,
    tag_id bigint NOT NULL
);


--
-- Name: page_templates; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.page_templates (
    id bigint NOT NULL,
    name text NOT NULL,
    description text DEFAULT ''::text NOT NULL,
    markdown_content text DEFAULT ''::text NOT NULL,
    path_prefix text DEFAULT ''::text NOT NULL,
    icon text DEFAULT ''::text NOT NULL,
    tags text[] DEFAULT '{}'::text[] NOT NULL,
    status text DEFAULT 'verified'::text NOT NULL,
    owner_group_id bigint,
    review_interval_days integer DEFAULT 0 NOT NULL,
    properties jsonb DEFAULT '{}'::jsonb NOT NULL,
    fields jsonb DEFAULT '[]'::jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT page_templates_fields_check CHECK ((jsonb_typeof(fields) = 'array'::text)),
    CONSTRAINT page_templates_properties_check CHECK ((jsonb_typeof(properties) = 'object'::text)),
    CONSTRAINT page_templates_review_interval_days_check CHECK ((review_interval_days >= 0)),
    CONSTRAINT page_templates_status_check CHECK ((status = ANY (ARRAY['draft'::text, 'verified'::text, 'deprecated'::text, 'archived'::text])))
);


--
-- Name: page_templates_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.page_templates_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: page_templates_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.page_templates_id_seq OWNED BY public.page_templates.id;


--
-- Name: page_views; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.page_views (
    user_id bigint NOT NULL,
    page_id bigint NOT NULL,
    viewed_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: page_watches; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.page_watches (
    user_id bigint NOT NULL,
    path text NOT NULL,
    scope text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT page_watches_scope_check CHECK ((scope = ANY (ARRAY['page'::text, 'subtree'::text])))
);


--
-- Name: pages; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.pages (
    id bigint NOT NULL,
    slug text NOT NULL,
    title text NOT NULL,
    markdown_content text DEFAULT ''::text NOT NULL,
    content_language text DEFAULT ''::text NOT NULL,
    plugin_usage jsonb,
    rendered_html text DEFAULT ''::text NOT NULL,
    rendered_contents jsonb DEFAULT '[]'::jsonb NOT NULL,
    render_fingerprint text DEFAULT ''::text NOT NULL,
    rendered_at timestamp with time zone,
    created_by bigint,
    updated_by bigint,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    view_count bigint DEFAULT 0 NOT NULL,
    deleted_at timestamp with time zone,
    deleted_by bigint,
    status text DEFAULT 'verified'::text NOT NULL,
    owner_group_id bigint,
    last_reviewed_at timestamp with time zone,
    review_interval_days integer DEFAULT 0 NOT NULL,
    deprecated_target text DEFAULT ''::text NOT NULL,
    search_vector tsvector GENERATED ALWAYS AS ((setweight(to_tsvector('english'::regconfig, COALESCE(title, ''::text)), 'A'::"char") || setweight(to_tsvector('english'::regconfig, COALESCE(markdown_content, ''::text)), 'B'::"char"))) STORED,
    CONSTRAINT pages_review_interval_days_check CHECK ((review_interval_days >= 0)),
    CONSTRAINT pages_status_check CHECK ((status = ANY (ARRAY['draft'::text, 'verified'::text, 'deprecated'::text, 'archived'::text])))
);


--
-- Name: pages_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.pages_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: pages_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.pages_id_seq OWNED BY public.pages.id;


--
-- Name: pdf_headers; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.pdf_headers (
    id bigint NOT NULL,
    name text NOT NULL,
    value text NOT NULL,
    sensitive boolean DEFAULT false NOT NULL
);


--
-- Name: pdf_headers_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.pdf_headers_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: pdf_headers_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.pdf_headers_id_seq OWNED BY public.pdf_headers.id;


--
-- Name: pending_oidc_identities; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.pending_oidc_identities (
    id bigint NOT NULL,
    issuer text NOT NULL,
    subject text NOT NULL,
    username text DEFAULT ''::text NOT NULL,
    email text DEFAULT ''::text NOT NULL,
    display_name text DEFAULT ''::text NOT NULL,
    status text DEFAULT 'pending'::text NOT NULL,
    first_seen_at timestamp with time zone DEFAULT now() NOT NULL,
    last_seen_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT pending_oidc_identities_status_check CHECK ((status = ANY (ARRAY['pending'::text, 'rejected'::text])))
);


--
-- Name: pending_oidc_identities_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.pending_oidc_identities_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: pending_oidc_identities_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.pending_oidc_identities_id_seq OWNED BY public.pending_oidc_identities.id;


--
-- Name: plugin_installations; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.plugin_installations (
    plugin_id text NOT NULL,
    source text NOT NULL,
    enabled boolean NOT NULL,
    package bytea,
    CONSTRAINT plugin_installations_check CHECK ((((source = 'bundled'::text) AND (package IS NULL)) OR ((source = 'installed'::text) AND (package IS NOT NULL) AND ((octet_length(package) >= 1) AND (octet_length(package) <= 16777216))))),
    CONSTRAINT plugin_installations_plugin_id_check CHECK (((octet_length(plugin_id) >= 1) AND (octet_length(plugin_id) <= 128))),
    CONSTRAINT plugin_installations_source_check CHECK ((source = ANY (ARRAY['bundled'::text, 'installed'::text])))
);


--
-- Name: plugin_update_announcements; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.plugin_update_announcements (
    plugin_id text NOT NULL,
    version text NOT NULL,
    announced_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: TABLE plugin_update_announcements; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE public.plugin_update_announcements IS 'Plugin release identities already announced to administrators.';


--
-- Name: COLUMN plugin_update_announcements.plugin_id; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.plugin_update_announcements.plugin_id IS 'Stable plugin identifier from the first-party catalog.';


--
-- Name: COLUMN plugin_update_announcements.version; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.plugin_update_announcements.version IS 'Compatible catalog version already announced to administrators.';


--
-- Name: plugin_values; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.plugin_values (
    plugin_id text NOT NULL,
    namespace text NOT NULL,
    key text NOT NULL,
    value bytea NOT NULL,
    CONSTRAINT plugin_values_key_check CHECK (((octet_length(key) >= 1) AND (octet_length(key) <= 256))),
    CONSTRAINT plugin_values_namespace_check CHECK ((namespace = ANY (ARRAY['settings'::text, 'data'::text]))),
    CONSTRAINT plugin_values_value_check CHECK ((octet_length(value) <= 65536))
);


--
-- Name: saved_searches; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.saved_searches (
    id bigint NOT NULL,
    user_id bigint NOT NULL,
    name text NOT NULL,
    query text NOT NULL,
    pinned boolean DEFAULT true NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: saved_searches_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.saved_searches_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: saved_searches_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.saved_searches_id_seq OWNED BY public.saved_searches.id;


--
-- Name: tags; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.tags (
    id bigint NOT NULL,
    name text NOT NULL
);


--
-- Name: tags_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.tags_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: tags_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.tags_id_seq OWNED BY public.tags.id;


--
-- Name: user_groups; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.user_groups (
    user_id bigint NOT NULL,
    group_id bigint NOT NULL
);


--
-- Name: user_preferences; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.user_preferences (
    user_id bigint NOT NULL,
    theme text DEFAULT ''::text NOT NULL,
    show_page_contents boolean DEFAULT true NOT NULL,
    navigation_style text DEFAULT 'sidebar'::text NOT NULL,
    navigation_density text DEFAULT 'comfortable'::text NOT NULL,
    typography_size text DEFAULT ''::text NOT NULL,
    sidebar_width integer DEFAULT 280 NOT NULL,
    show_navigation_guides boolean DEFAULT true NOT NULL,
    remember_navigation_state boolean DEFAULT true NOT NULL,
    show_navigation_page_counts boolean DEFAULT false NOT NULL,
    expanded_navigation text[] DEFAULT '{}'::text[] NOT NULL,
    hidden_plugin_widgets text[] DEFAULT '{}'::text[] NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT user_preferences_navigation_density_check CHECK ((navigation_density = ANY (ARRAY['comfortable'::text, 'compact'::text]))),
    CONSTRAINT user_preferences_navigation_style_check CHECK ((navigation_style = ANY (ARRAY['sidebar'::text, 'topbar'::text, 'tree'::text]))),
    CONSTRAINT user_preferences_sidebar_width_check CHECK (((sidebar_width >= 220) AND (sidebar_width <= 420))),
    CONSTRAINT user_preferences_typography_size_check CHECK ((typography_size = ANY (ARRAY[''::text, 'compact'::text, 'standard'::text, 'large'::text])))
);


--
-- Name: users; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.users (
    id bigint NOT NULL,
    username text NOT NULL,
    email text DEFAULT ''::text NOT NULL,
    display_name text DEFAULT ''::text NOT NULL,
    role text DEFAULT 'viewer'::text NOT NULL,
    enabled boolean DEFAULT true NOT NULL,
    session_version bigint DEFAULT 1 NOT NULL,
    oidc_admin_observed boolean DEFAULT false NOT NULL,
    oidc_external_admin boolean DEFAULT false NOT NULL,
    trusted_proxy_admin_observed boolean DEFAULT false NOT NULL,
    trusted_proxy_external_admin boolean DEFAULT false NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    last_login timestamp with time zone,
    CONSTRAINT users_role_check CHECK ((role = ANY (ARRAY['admin'::text, 'editor'::text, 'viewer'::text])))
);


--
-- Name: users_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.users_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: users_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.users_id_seq OWNED BY public.users.id;


--
-- Name: webhook_deliveries; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.webhook_deliveries (
    id bigint NOT NULL,
    webhook_id bigint NOT NULL,
    event text NOT NULL,
    status_code integer DEFAULT 0 NOT NULL,
    error text DEFAULT ''::text NOT NULL,
    attempts integer DEFAULT 1 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT webhook_deliveries_attempts_check CHECK ((attempts >= 0))
);


--
-- Name: webhook_deliveries_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.webhook_deliveries_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: webhook_deliveries_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.webhook_deliveries_id_seq OWNED BY public.webhook_deliveries.id;


--
-- Name: webhook_headers; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.webhook_headers (
    id bigint NOT NULL,
    webhook_id bigint NOT NULL,
    name text NOT NULL,
    value text NOT NULL,
    sensitive boolean DEFAULT false NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: webhook_headers_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.webhook_headers_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: webhook_headers_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.webhook_headers_id_seq OWNED BY public.webhook_headers.id;


--
-- Name: webhooks; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.webhooks (
    id bigint NOT NULL,
    name text NOT NULL,
    url text NOT NULL,
    events text[] DEFAULT '{}'::text[] NOT NULL,
    enabled boolean DEFAULT true NOT NULL,
    body_template text DEFAULT ''::text NOT NULL,
    retry_enabled boolean DEFAULT false NOT NULL,
    retry_count integer DEFAULT 2 NOT NULL,
    retry_backoff_ms bigint DEFAULT 1000 NOT NULL,
    retry_max_backoff_ms bigint DEFAULT 30000 NOT NULL,
    retry_jitter boolean DEFAULT true NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT webhooks_retry_backoff_ms_check CHECK ((retry_backoff_ms >= 0)),
    CONSTRAINT webhooks_retry_count_check CHECK ((retry_count >= 0)),
    CONSTRAINT webhooks_retry_max_backoff_ms_check CHECK ((retry_max_backoff_ms >= 0))
);


--
-- Name: webhooks_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.webhooks_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: webhooks_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.webhooks_id_seq OWNED BY public.webhooks.id;


--
-- Name: wiki_groups; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.wiki_groups (
    id bigint NOT NULL,
    name text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: wiki_groups_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.wiki_groups_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: wiki_groups_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.wiki_groups_id_seq OWNED BY public.wiki_groups.id;


--
-- Name: api_tokens id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.api_tokens ALTER COLUMN id SET DEFAULT nextval('public.api_tokens_id_seq'::regclass);


--
-- Name: attachments id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.attachments ALTER COLUMN id SET DEFAULT nextval('public.attachments_id_seq'::regclass);


--
-- Name: audit_events id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.audit_events ALTER COLUMN id SET DEFAULT nextval('public.audit_events_id_seq'::regclass);


--
-- Name: images id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.images ALTER COLUMN id SET DEFAULT nextval('public.images_id_seq'::regclass);


--
-- Name: notifications id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.notifications ALTER COLUMN id SET DEFAULT nextval('public.notifications_id_seq'::regclass);


--
-- Name: page_access_rules id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.page_access_rules ALTER COLUMN id SET DEFAULT nextval('public.page_access_rules_id_seq'::regclass);


--
-- Name: page_comments id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.page_comments ALTER COLUMN id SET DEFAULT nextval('public.page_comments_id_seq'::regclass);


--
-- Name: page_drafts id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.page_drafts ALTER COLUMN id SET DEFAULT nextval('public.page_drafts_id_seq'::regclass);


--
-- Name: page_review_comments id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.page_review_comments ALTER COLUMN id SET DEFAULT nextval('public.page_review_comments_id_seq'::regclass);


--
-- Name: page_review_requests id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.page_review_requests ALTER COLUMN id SET DEFAULT nextval('public.page_review_requests_id_seq'::regclass);


--
-- Name: page_revisions id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.page_revisions ALTER COLUMN id SET DEFAULT nextval('public.page_revisions_id_seq'::regclass);


--
-- Name: page_templates id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.page_templates ALTER COLUMN id SET DEFAULT nextval('public.page_templates_id_seq'::regclass);


--
-- Name: pages id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.pages ALTER COLUMN id SET DEFAULT nextval('public.pages_id_seq'::regclass);


--
-- Name: pdf_headers id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.pdf_headers ALTER COLUMN id SET DEFAULT nextval('public.pdf_headers_id_seq'::regclass);


--
-- Name: pending_oidc_identities id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.pending_oidc_identities ALTER COLUMN id SET DEFAULT nextval('public.pending_oidc_identities_id_seq'::regclass);


--
-- Name: saved_searches id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.saved_searches ALTER COLUMN id SET DEFAULT nextval('public.saved_searches_id_seq'::regclass);


--
-- Name: tags id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.tags ALTER COLUMN id SET DEFAULT nextval('public.tags_id_seq'::regclass);


--
-- Name: users id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.users ALTER COLUMN id SET DEFAULT nextval('public.users_id_seq'::regclass);


--
-- Name: webhook_deliveries id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.webhook_deliveries ALTER COLUMN id SET DEFAULT nextval('public.webhook_deliveries_id_seq'::regclass);


--
-- Name: webhook_headers id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.webhook_headers ALTER COLUMN id SET DEFAULT nextval('public.webhook_headers_id_seq'::regclass);


--
-- Name: webhooks id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.webhooks ALTER COLUMN id SET DEFAULT nextval('public.webhooks_id_seq'::regclass);


--
-- Name: wiki_groups id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.wiki_groups ALTER COLUMN id SET DEFAULT nextval('public.wiki_groups_id_seq'::regclass);


--
-- Data for Name: api_tokens; Type: TABLE DATA; Schema: public; Owner: -
--



--
-- Data for Name: application_settings; Type: TABLE DATA; Schema: public; Owner: -
--

INSERT INTO public.application_settings VALUES (true, true, 'en', true, 'none', '', '', 'groups', false, true, '', '{X-Forwarded-User,X-Auth-Request-User,Remote-User}', '{X-Forwarded-Email,X-Auth-Request-Email,X-Authentik-Email}', '{X-Forwarded-Name,X-Auth-Request-Preferred-Username,X-Authentik-Name}', '{X-Forwarded-Groups,X-Auth-Request-Groups}', '', '', '[]', 'compact', 'disallow', '2026-09-24 15:12:29.758812+00', '', NULL, '[]');


--
-- Data for Name: attachments; Type: TABLE DATA; Schema: public; Owner: -
--



--
-- Data for Name: audit_events; Type: TABLE DATA; Schema: public; Owner: -
--



--
-- Data for Name: favorites; Type: TABLE DATA; Schema: public; Owner: -
--



--
-- Data for Name: images; Type: TABLE DATA; Schema: public; Owner: -
--



--
-- Data for Name: local_credentials; Type: TABLE DATA; Schema: public; Owner: -
--



--
-- Data for Name: local_sessions; Type: TABLE DATA; Schema: public; Owner: -
--



--
-- Data for Name: navigation_icons; Type: TABLE DATA; Schema: public; Owner: -
--



--
-- Data for Name: notifications; Type: TABLE DATA; Schema: public; Owner: -
--



--
-- Data for Name: oidc_group_mappings; Type: TABLE DATA; Schema: public; Owner: -
--



--
-- Data for Name: oidc_identities; Type: TABLE DATA; Schema: public; Owner: -
--



--
-- Data for Name: page_access_rules; Type: TABLE DATA; Schema: public; Owner: -
--



--
-- Data for Name: page_aliases; Type: TABLE DATA; Schema: public; Owner: -
--



--
-- Data for Name: page_comments; Type: TABLE DATA; Schema: public; Owner: -
--



--
-- Data for Name: page_drafts; Type: TABLE DATA; Schema: public; Owner: -
--



--
-- Data for Name: page_edit_presence; Type: TABLE DATA; Schema: public; Owner: -
--



--
-- Data for Name: page_groups; Type: TABLE DATA; Schema: public; Owner: -
--



--
-- Data for Name: page_links; Type: TABLE DATA; Schema: public; Owner: -
--



--
-- Data for Name: page_properties; Type: TABLE DATA; Schema: public; Owner: -
--



--
-- Data for Name: page_review_comments; Type: TABLE DATA; Schema: public; Owner: -
--



--
-- Data for Name: page_review_request_reviewers; Type: TABLE DATA; Schema: public; Owner: -
--



--
-- Data for Name: page_review_requests; Type: TABLE DATA; Schema: public; Owner: -
--



--
-- Data for Name: page_revisions; Type: TABLE DATA; Schema: public; Owner: -
--



--
-- Data for Name: page_tags; Type: TABLE DATA; Schema: public; Owner: -
--



--
-- Data for Name: page_templates; Type: TABLE DATA; Schema: public; Owner: -
--



--
-- Data for Name: page_views; Type: TABLE DATA; Schema: public; Owner: -
--



--
-- Data for Name: page_watches; Type: TABLE DATA; Schema: public; Owner: -
--



--
-- Data for Name: pages; Type: TABLE DATA; Schema: public; Owner: -
--



--
-- Data for Name: pdf_headers; Type: TABLE DATA; Schema: public; Owner: -
--



--
-- Data for Name: pending_oidc_identities; Type: TABLE DATA; Schema: public; Owner: -
--



--
-- Data for Name: plugin_installations; Type: TABLE DATA; Schema: public; Owner: -
--



--
-- Data for Name: plugin_update_announcements; Type: TABLE DATA; Schema: public; Owner: -
--



--
-- Data for Name: plugin_values; Type: TABLE DATA; Schema: public; Owner: -
--



--
-- Data for Name: saved_searches; Type: TABLE DATA; Schema: public; Owner: -
--



--
-- Data for Name: tags; Type: TABLE DATA; Schema: public; Owner: -
--



--
-- Data for Name: user_groups; Type: TABLE DATA; Schema: public; Owner: -
--



--
-- Data for Name: user_preferences; Type: TABLE DATA; Schema: public; Owner: -
--



--
-- Data for Name: users; Type: TABLE DATA; Schema: public; Owner: -
--



--
-- Data for Name: webhook_deliveries; Type: TABLE DATA; Schema: public; Owner: -
--



--
-- Data for Name: webhook_headers; Type: TABLE DATA; Schema: public; Owner: -
--



--
-- Data for Name: webhooks; Type: TABLE DATA; Schema: public; Owner: -
--



--
-- Data for Name: wiki_groups; Type: TABLE DATA; Schema: public; Owner: -
--



--
-- Name: api_tokens_id_seq; Type: SEQUENCE SET; Schema: public; Owner: -
--

SELECT pg_catalog.setval('public.api_tokens_id_seq', 1, false);


--
-- Name: attachments_id_seq; Type: SEQUENCE SET; Schema: public; Owner: -
--

SELECT pg_catalog.setval('public.attachments_id_seq', 1, false);


--
-- Name: audit_events_id_seq; Type: SEQUENCE SET; Schema: public; Owner: -
--

SELECT pg_catalog.setval('public.audit_events_id_seq', 1, false);


--
-- Name: images_id_seq; Type: SEQUENCE SET; Schema: public; Owner: -
--

SELECT pg_catalog.setval('public.images_id_seq', 1, false);


--
-- Name: notifications_id_seq; Type: SEQUENCE SET; Schema: public; Owner: -
--

SELECT pg_catalog.setval('public.notifications_id_seq', 1, false);


--
-- Name: page_access_rules_id_seq; Type: SEQUENCE SET; Schema: public; Owner: -
--

SELECT pg_catalog.setval('public.page_access_rules_id_seq', 1, false);


--
-- Name: page_comments_id_seq; Type: SEQUENCE SET; Schema: public; Owner: -
--

SELECT pg_catalog.setval('public.page_comments_id_seq', 1, false);


--
-- Name: page_drafts_id_seq; Type: SEQUENCE SET; Schema: public; Owner: -
--

SELECT pg_catalog.setval('public.page_drafts_id_seq', 1, false);


--
-- Name: page_review_comments_id_seq; Type: SEQUENCE SET; Schema: public; Owner: -
--

SELECT pg_catalog.setval('public.page_review_comments_id_seq', 1, false);


--
-- Name: page_review_requests_id_seq; Type: SEQUENCE SET; Schema: public; Owner: -
--

SELECT pg_catalog.setval('public.page_review_requests_id_seq', 1, false);


--
-- Name: page_revisions_id_seq; Type: SEQUENCE SET; Schema: public; Owner: -
--

SELECT pg_catalog.setval('public.page_revisions_id_seq', 1, false);


--
-- Name: page_templates_id_seq; Type: SEQUENCE SET; Schema: public; Owner: -
--

SELECT pg_catalog.setval('public.page_templates_id_seq', 1, false);


--
-- Name: pages_id_seq; Type: SEQUENCE SET; Schema: public; Owner: -
--

SELECT pg_catalog.setval('public.pages_id_seq', 1, false);


--
-- Name: pdf_headers_id_seq; Type: SEQUENCE SET; Schema: public; Owner: -
--

SELECT pg_catalog.setval('public.pdf_headers_id_seq', 1, false);


--
-- Name: pending_oidc_identities_id_seq; Type: SEQUENCE SET; Schema: public; Owner: -
--

SELECT pg_catalog.setval('public.pending_oidc_identities_id_seq', 1, false);


--
-- Name: saved_searches_id_seq; Type: SEQUENCE SET; Schema: public; Owner: -
--

SELECT pg_catalog.setval('public.saved_searches_id_seq', 1, false);


--
-- Name: tags_id_seq; Type: SEQUENCE SET; Schema: public; Owner: -
--

SELECT pg_catalog.setval('public.tags_id_seq', 1, false);


--
-- Name: users_id_seq; Type: SEQUENCE SET; Schema: public; Owner: -
--

SELECT pg_catalog.setval('public.users_id_seq', 1, false);


--
-- Name: webhook_deliveries_id_seq; Type: SEQUENCE SET; Schema: public; Owner: -
--

SELECT pg_catalog.setval('public.webhook_deliveries_id_seq', 1, false);


--
-- Name: webhook_headers_id_seq; Type: SEQUENCE SET; Schema: public; Owner: -
--

SELECT pg_catalog.setval('public.webhook_headers_id_seq', 1, false);


--
-- Name: webhooks_id_seq; Type: SEQUENCE SET; Schema: public; Owner: -
--

SELECT pg_catalog.setval('public.webhooks_id_seq', 1, false);


--
-- Name: wiki_groups_id_seq; Type: SEQUENCE SET; Schema: public; Owner: -
--

SELECT pg_catalog.setval('public.wiki_groups_id_seq', 1, false);


--
-- Name: api_tokens api_tokens_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.api_tokens
    ADD CONSTRAINT api_tokens_pkey PRIMARY KEY (id);


--
-- Name: api_tokens api_tokens_token_hash_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.api_tokens
    ADD CONSTRAINT api_tokens_token_hash_key UNIQUE (token_hash);


--
-- Name: application_settings application_settings_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.application_settings
    ADD CONSTRAINT application_settings_pkey PRIMARY KEY (singleton);


--
-- Name: attachments attachments_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.attachments
    ADD CONSTRAINT attachments_pkey PRIMARY KEY (id);


--
-- Name: audit_events audit_events_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.audit_events
    ADD CONSTRAINT audit_events_pkey PRIMARY KEY (id);


--
-- Name: favorites favorites_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.favorites
    ADD CONSTRAINT favorites_pkey PRIMARY KEY (user_id, page_id);


--
-- Name: images images_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.images
    ADD CONSTRAINT images_pkey PRIMARY KEY (id);


--
-- Name: local_credentials local_credentials_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.local_credentials
    ADD CONSTRAINT local_credentials_pkey PRIMARY KEY (user_id);


--
-- Name: local_sessions local_sessions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.local_sessions
    ADD CONSTRAINT local_sessions_pkey PRIMARY KEY (token_hash);


--
-- Name: navigation_icons navigation_icons_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.navigation_icons
    ADD CONSTRAINT navigation_icons_pkey PRIMARY KEY (path);


--
-- Name: notifications notifications_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.notifications
    ADD CONSTRAINT notifications_pkey PRIMARY KEY (id);


--
-- Name: oidc_group_mappings oidc_group_mappings_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.oidc_group_mappings
    ADD CONSTRAINT oidc_group_mappings_pkey PRIMARY KEY (oidc_group);


--
-- Name: oidc_identities oidc_identities_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.oidc_identities
    ADD CONSTRAINT oidc_identities_pkey PRIMARY KEY (issuer, subject);


--
-- Name: oidc_identities oidc_identities_user_id_issuer_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.oidc_identities
    ADD CONSTRAINT oidc_identities_user_id_issuer_key UNIQUE (user_id, issuer);


--
-- Name: page_access_rules page_access_rules_path_group_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.page_access_rules
    ADD CONSTRAINT page_access_rules_path_group_id_key UNIQUE (path, group_id);


--
-- Name: page_access_rules page_access_rules_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.page_access_rules
    ADD CONSTRAINT page_access_rules_pkey PRIMARY KEY (id);


--
-- Name: page_aliases page_aliases_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.page_aliases
    ADD CONSTRAINT page_aliases_pkey PRIMARY KEY (alias);


--
-- Name: page_comments page_comments_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.page_comments
    ADD CONSTRAINT page_comments_pkey PRIMARY KEY (id);


--
-- Name: page_drafts page_drafts_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.page_drafts
    ADD CONSTRAINT page_drafts_pkey PRIMARY KEY (id);


--
-- Name: page_drafts page_drafts_user_id_draft_key_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.page_drafts
    ADD CONSTRAINT page_drafts_user_id_draft_key_key UNIQUE (user_id, draft_key);


--
-- Name: page_edit_presence page_edit_presence_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.page_edit_presence
    ADD CONSTRAINT page_edit_presence_pkey PRIMARY KEY (page_id, user_id);


--
-- Name: page_groups page_groups_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.page_groups
    ADD CONSTRAINT page_groups_pkey PRIMARY KEY (page_id, group_id);


--
-- Name: page_links page_links_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.page_links
    ADD CONSTRAINT page_links_pkey PRIMARY KEY (source_page_id, target_slug);


--
-- Name: page_properties page_properties_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.page_properties
    ADD CONSTRAINT page_properties_pkey PRIMARY KEY (page_id, key);


--
-- Name: page_review_comments page_review_comments_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.page_review_comments
    ADD CONSTRAINT page_review_comments_pkey PRIMARY KEY (id);


--
-- Name: page_review_request_reviewers page_review_request_reviewers_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.page_review_request_reviewers
    ADD CONSTRAINT page_review_request_reviewers_pkey PRIMARY KEY (request_id, user_id);


--
-- Name: page_review_requests page_review_requests_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.page_review_requests
    ADD CONSTRAINT page_review_requests_pkey PRIMARY KEY (id);


--
-- Name: page_revisions page_revisions_page_id_revision_number_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.page_revisions
    ADD CONSTRAINT page_revisions_page_id_revision_number_key UNIQUE (page_id, revision_number);


--
-- Name: page_revisions page_revisions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.page_revisions
    ADD CONSTRAINT page_revisions_pkey PRIMARY KEY (id);


--
-- Name: page_tags page_tags_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.page_tags
    ADD CONSTRAINT page_tags_pkey PRIMARY KEY (page_id, tag_id);


--
-- Name: page_templates page_templates_name_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.page_templates
    ADD CONSTRAINT page_templates_name_key UNIQUE (name);


--
-- Name: page_templates page_templates_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.page_templates
    ADD CONSTRAINT page_templates_pkey PRIMARY KEY (id);


--
-- Name: page_views page_views_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.page_views
    ADD CONSTRAINT page_views_pkey PRIMARY KEY (user_id, page_id);


--
-- Name: page_watches page_watches_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.page_watches
    ADD CONSTRAINT page_watches_pkey PRIMARY KEY (user_id, path);


--
-- Name: pages pages_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.pages
    ADD CONSTRAINT pages_pkey PRIMARY KEY (id);


--
-- Name: pages pages_slug_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.pages
    ADD CONSTRAINT pages_slug_key UNIQUE (slug);


--
-- Name: pdf_headers pdf_headers_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.pdf_headers
    ADD CONSTRAINT pdf_headers_pkey PRIMARY KEY (id);


--
-- Name: pending_oidc_identities pending_oidc_identities_issuer_subject_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.pending_oidc_identities
    ADD CONSTRAINT pending_oidc_identities_issuer_subject_key UNIQUE (issuer, subject);


--
-- Name: pending_oidc_identities pending_oidc_identities_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.pending_oidc_identities
    ADD CONSTRAINT pending_oidc_identities_pkey PRIMARY KEY (id);


--
-- Name: plugin_installations plugin_installations_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.plugin_installations
    ADD CONSTRAINT plugin_installations_pkey PRIMARY KEY (plugin_id);


--
-- Name: plugin_update_announcements plugin_update_announcements_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.plugin_update_announcements
    ADD CONSTRAINT plugin_update_announcements_pkey PRIMARY KEY (plugin_id, version);


--
-- Name: plugin_values plugin_values_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.plugin_values
    ADD CONSTRAINT plugin_values_pkey PRIMARY KEY (plugin_id, namespace, key);


--
-- Name: saved_searches saved_searches_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.saved_searches
    ADD CONSTRAINT saved_searches_pkey PRIMARY KEY (id);


--
-- Name: saved_searches saved_searches_user_id_name_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.saved_searches
    ADD CONSTRAINT saved_searches_user_id_name_key UNIQUE (user_id, name);


--
-- Name: tags tags_name_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.tags
    ADD CONSTRAINT tags_name_key UNIQUE (name);


--
-- Name: tags tags_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.tags
    ADD CONSTRAINT tags_pkey PRIMARY KEY (id);


--
-- Name: user_groups user_groups_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_groups
    ADD CONSTRAINT user_groups_pkey PRIMARY KEY (user_id, group_id);


--
-- Name: user_preferences user_preferences_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_preferences
    ADD CONSTRAINT user_preferences_pkey PRIMARY KEY (user_id);


--
-- Name: users users_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.users
    ADD CONSTRAINT users_pkey PRIMARY KEY (id);


--
-- Name: users users_username_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.users
    ADD CONSTRAINT users_username_key UNIQUE (username);


--
-- Name: webhook_deliveries webhook_deliveries_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.webhook_deliveries
    ADD CONSTRAINT webhook_deliveries_pkey PRIMARY KEY (id);


--
-- Name: webhook_headers webhook_headers_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.webhook_headers
    ADD CONSTRAINT webhook_headers_pkey PRIMARY KEY (id);


--
-- Name: webhooks webhooks_name_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.webhooks
    ADD CONSTRAINT webhooks_name_key UNIQUE (name);


--
-- Name: webhooks webhooks_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.webhooks
    ADD CONSTRAINT webhooks_pkey PRIMARY KEY (id);


--
-- Name: wiki_groups wiki_groups_name_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.wiki_groups
    ADD CONSTRAINT wiki_groups_name_key UNIQUE (name);


--
-- Name: wiki_groups wiki_groups_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.wiki_groups
    ADD CONSTRAINT wiki_groups_pkey PRIMARY KEY (id);


--
-- Name: api_tokens_user_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX api_tokens_user_idx ON public.api_tokens USING btree (user_id, created_at DESC);


--
-- Name: attachments_created_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX attachments_created_idx ON public.attachments USING btree (created_at DESC);


--
-- Name: audit_events_created_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX audit_events_created_idx ON public.audit_events USING btree (created_at DESC, id DESC);


--
-- Name: images_created_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX images_created_idx ON public.images USING btree (created_at DESC);


--
-- Name: local_sessions_expiry_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX local_sessions_expiry_idx ON public.local_sessions USING btree (expires_at);


--
-- Name: local_sessions_user_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX local_sessions_user_idx ON public.local_sessions USING btree (user_id);


--
-- Name: notifications_user_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX notifications_user_idx ON public.notifications USING btree (user_id, read_at, created_at DESC);


--
-- Name: oidc_group_mappings_group_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX oidc_group_mappings_group_idx ON public.oidc_group_mappings USING btree (group_id);


--
-- Name: oidc_identities_user_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX oidc_identities_user_idx ON public.oidc_identities USING btree (user_id);


--
-- Name: page_access_rules_path_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX page_access_rules_path_idx ON public.page_access_rules USING btree (path);


--
-- Name: page_aliases_page_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX page_aliases_page_idx ON public.page_aliases USING btree (page_id);


--
-- Name: page_comments_open_suggestion_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX page_comments_open_suggestion_idx ON public.page_comments USING btree (page_id, suggestion_applied_at) WHERE (suggestion_revision IS NOT NULL);


--
-- Name: page_comments_page_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX page_comments_page_idx ON public.page_comments USING btree (page_id, resolved_at, created_at);


--
-- Name: page_comments_parent_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX page_comments_parent_idx ON public.page_comments USING btree (parent_id);


--
-- Name: page_drafts_page_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX page_drafts_page_idx ON public.page_drafts USING btree (page_id) WHERE (page_id IS NOT NULL);


--
-- Name: page_drafts_user_updated_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX page_drafts_user_updated_idx ON public.page_drafts USING btree (user_id, updated_at DESC);


--
-- Name: page_edit_presence_updated_at_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX page_edit_presence_updated_at_idx ON public.page_edit_presence USING btree (updated_at);


--
-- Name: page_groups_group_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX page_groups_group_idx ON public.page_groups USING btree (group_id, page_id);


--
-- Name: page_links_target_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX page_links_target_idx ON public.page_links USING btree (target_slug);


--
-- Name: page_properties_key_ci_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX page_properties_key_ci_idx ON public.page_properties USING btree (page_id, lower(key));


--
-- Name: page_review_comments_open_suggestion_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX page_review_comments_open_suggestion_idx ON public.page_review_comments USING btree (request_id, id) WHERE (is_suggestion AND (applied_at IS NULL));


--
-- Name: page_review_comments_request_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX page_review_comments_request_idx ON public.page_review_comments USING btree (request_id, start_line, created_at, id);


--
-- Name: page_review_request_reviewers_user_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX page_review_request_reviewers_user_idx ON public.page_review_request_reviewers USING btree (user_id, request_id);


--
-- Name: page_review_requests_page_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX page_review_requests_page_idx ON public.page_review_requests USING btree (page_id, created_at DESC, id DESC);


--
-- Name: page_review_requests_pending_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX page_review_requests_pending_idx ON public.page_review_requests USING btree (page_id) WHERE (status = 'pending'::text);


--
-- Name: page_templates_name_ci_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX page_templates_name_ci_idx ON public.page_templates USING btree (lower(name));


--
-- Name: page_watches_path_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX page_watches_path_idx ON public.page_watches USING btree (path, scope, user_id);


--
-- Name: pages_deleted_at_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX pages_deleted_at_idx ON public.pages USING btree (deleted_at) WHERE (deleted_at IS NOT NULL);


--
-- Name: pages_review_due_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX pages_review_due_idx ON public.pages USING btree (last_reviewed_at, review_interval_days) WHERE ((deleted_at IS NULL) AND (review_interval_days > 0));


--
-- Name: pages_search_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX pages_search_idx ON public.pages USING gin (search_vector);


--
-- Name: pages_status_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX pages_status_idx ON public.pages USING btree (status) WHERE (deleted_at IS NULL);


--
-- Name: pages_updated_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX pages_updated_idx ON public.pages USING btree (updated_at DESC);


--
-- Name: pdf_headers_name_ci_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX pdf_headers_name_ci_idx ON public.pdf_headers USING btree (lower(name));


--
-- Name: pending_oidc_identities_status_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX pending_oidc_identities_status_idx ON public.pending_oidc_identities USING btree (status, last_seen_at DESC);


--
-- Name: saved_searches_user_name_ci_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX saved_searches_user_name_ci_idx ON public.saved_searches USING btree (user_id, lower(name));


--
-- Name: user_groups_group_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX user_groups_group_idx ON public.user_groups USING btree (group_id, user_id);


--
-- Name: webhook_deliveries_recent_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX webhook_deliveries_recent_idx ON public.webhook_deliveries USING btree (created_at DESC, id DESC);


--
-- Name: webhook_headers_name_ci_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX webhook_headers_name_ci_idx ON public.webhook_headers USING btree (webhook_id, lower(name));


--
-- Name: webhooks_name_ci_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX webhooks_name_ci_idx ON public.webhooks USING btree (lower(name));


--
-- Name: wiki_groups_name_ci_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX wiki_groups_name_ci_idx ON public.wiki_groups USING btree (lower(name));


--
-- Name: api_tokens api_tokens_created_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.api_tokens
    ADD CONSTRAINT api_tokens_created_by_fkey FOREIGN KEY (created_by) REFERENCES public.users(id);


--
-- Name: api_tokens api_tokens_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.api_tokens
    ADD CONSTRAINT api_tokens_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: attachments attachments_uploaded_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.attachments
    ADD CONSTRAINT attachments_uploaded_by_fkey FOREIGN KEY (uploaded_by) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: audit_events audit_events_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.audit_events
    ADD CONSTRAINT audit_events_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: favorites favorites_page_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.favorites
    ADD CONSTRAINT favorites_page_id_fkey FOREIGN KEY (page_id) REFERENCES public.pages(id) ON DELETE CASCADE;


--
-- Name: favorites favorites_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.favorites
    ADD CONSTRAINT favorites_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: images images_uploaded_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.images
    ADD CONSTRAINT images_uploaded_by_fkey FOREIGN KEY (uploaded_by) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: local_credentials local_credentials_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.local_credentials
    ADD CONSTRAINT local_credentials_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: local_sessions local_sessions_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.local_sessions
    ADD CONSTRAINT local_sessions_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: notifications notifications_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.notifications
    ADD CONSTRAINT notifications_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: oidc_group_mappings oidc_group_mappings_group_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.oidc_group_mappings
    ADD CONSTRAINT oidc_group_mappings_group_id_fkey FOREIGN KEY (group_id) REFERENCES public.wiki_groups(id) ON DELETE CASCADE;


--
-- Name: oidc_identities oidc_identities_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.oidc_identities
    ADD CONSTRAINT oidc_identities_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: page_access_rules page_access_rules_group_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.page_access_rules
    ADD CONSTRAINT page_access_rules_group_id_fkey FOREIGN KEY (group_id) REFERENCES public.wiki_groups(id) ON DELETE CASCADE;


--
-- Name: page_aliases page_aliases_page_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.page_aliases
    ADD CONSTRAINT page_aliases_page_id_fkey FOREIGN KEY (page_id) REFERENCES public.pages(id) ON DELETE CASCADE;


--
-- Name: page_comments page_comments_page_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.page_comments
    ADD CONSTRAINT page_comments_page_id_fkey FOREIGN KEY (page_id) REFERENCES public.pages(id) ON DELETE CASCADE;


--
-- Name: page_comments page_comments_parent_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.page_comments
    ADD CONSTRAINT page_comments_parent_id_fkey FOREIGN KEY (parent_id) REFERENCES public.page_comments(id) ON DELETE SET NULL;


--
-- Name: page_comments page_comments_suggestion_applied_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.page_comments
    ADD CONSTRAINT page_comments_suggestion_applied_by_fkey FOREIGN KEY (suggestion_applied_by) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: page_comments page_comments_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.page_comments
    ADD CONSTRAINT page_comments_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: page_drafts page_drafts_page_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.page_drafts
    ADD CONSTRAINT page_drafts_page_id_fkey FOREIGN KEY (page_id) REFERENCES public.pages(id) ON DELETE CASCADE;


--
-- Name: page_drafts page_drafts_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.page_drafts
    ADD CONSTRAINT page_drafts_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: page_edit_presence page_edit_presence_page_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.page_edit_presence
    ADD CONSTRAINT page_edit_presence_page_id_fkey FOREIGN KEY (page_id) REFERENCES public.pages(id) ON DELETE CASCADE;


--
-- Name: page_edit_presence page_edit_presence_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.page_edit_presence
    ADD CONSTRAINT page_edit_presence_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: page_groups page_groups_group_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.page_groups
    ADD CONSTRAINT page_groups_group_id_fkey FOREIGN KEY (group_id) REFERENCES public.wiki_groups(id) ON DELETE CASCADE;


--
-- Name: page_groups page_groups_page_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.page_groups
    ADD CONSTRAINT page_groups_page_id_fkey FOREIGN KEY (page_id) REFERENCES public.pages(id) ON DELETE CASCADE;


--
-- Name: page_links page_links_source_page_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.page_links
    ADD CONSTRAINT page_links_source_page_id_fkey FOREIGN KEY (source_page_id) REFERENCES public.pages(id) ON DELETE CASCADE;


--
-- Name: page_properties page_properties_page_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.page_properties
    ADD CONSTRAINT page_properties_page_id_fkey FOREIGN KEY (page_id) REFERENCES public.pages(id) ON DELETE CASCADE;


--
-- Name: page_review_comments page_review_comments_applied_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.page_review_comments
    ADD CONSTRAINT page_review_comments_applied_by_fkey FOREIGN KEY (applied_by) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: page_review_comments page_review_comments_request_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.page_review_comments
    ADD CONSTRAINT page_review_comments_request_id_fkey FOREIGN KEY (request_id) REFERENCES public.page_review_requests(id) ON DELETE CASCADE;


--
-- Name: page_review_comments page_review_comments_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.page_review_comments
    ADD CONSTRAINT page_review_comments_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: page_review_request_reviewers page_review_request_reviewers_request_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.page_review_request_reviewers
    ADD CONSTRAINT page_review_request_reviewers_request_id_fkey FOREIGN KEY (request_id) REFERENCES public.page_review_requests(id) ON DELETE CASCADE;


--
-- Name: page_review_request_reviewers page_review_request_reviewers_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.page_review_request_reviewers
    ADD CONSTRAINT page_review_request_reviewers_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: page_review_requests page_review_requests_page_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.page_review_requests
    ADD CONSTRAINT page_review_requests_page_id_fkey FOREIGN KEY (page_id) REFERENCES public.pages(id) ON DELETE CASCADE;


--
-- Name: page_review_requests page_review_requests_requested_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.page_review_requests
    ADD CONSTRAINT page_review_requests_requested_by_fkey FOREIGN KEY (requested_by) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: page_review_requests page_review_requests_reviewed_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.page_review_requests
    ADD CONSTRAINT page_review_requests_reviewed_by_fkey FOREIGN KEY (reviewed_by) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: page_review_requests page_review_requests_reviewer_group_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.page_review_requests
    ADD CONSTRAINT page_review_requests_reviewer_group_id_fkey FOREIGN KEY (reviewer_group_id) REFERENCES public.wiki_groups(id) ON DELETE SET NULL;


--
-- Name: page_revisions page_revisions_created_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.page_revisions
    ADD CONSTRAINT page_revisions_created_by_fkey FOREIGN KEY (created_by) REFERENCES public.users(id);


--
-- Name: page_revisions page_revisions_page_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.page_revisions
    ADD CONSTRAINT page_revisions_page_id_fkey FOREIGN KEY (page_id) REFERENCES public.pages(id) ON DELETE CASCADE;


--
-- Name: page_tags page_tags_page_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.page_tags
    ADD CONSTRAINT page_tags_page_id_fkey FOREIGN KEY (page_id) REFERENCES public.pages(id) ON DELETE CASCADE;


--
-- Name: page_tags page_tags_tag_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.page_tags
    ADD CONSTRAINT page_tags_tag_id_fkey FOREIGN KEY (tag_id) REFERENCES public.tags(id) ON DELETE CASCADE;


--
-- Name: page_templates page_templates_owner_group_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.page_templates
    ADD CONSTRAINT page_templates_owner_group_id_fkey FOREIGN KEY (owner_group_id) REFERENCES public.wiki_groups(id) ON DELETE SET NULL;


--
-- Name: page_views page_views_page_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.page_views
    ADD CONSTRAINT page_views_page_id_fkey FOREIGN KEY (page_id) REFERENCES public.pages(id) ON DELETE CASCADE;


--
-- Name: page_views page_views_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.page_views
    ADD CONSTRAINT page_views_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: page_watches page_watches_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.page_watches
    ADD CONSTRAINT page_watches_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: pages pages_created_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.pages
    ADD CONSTRAINT pages_created_by_fkey FOREIGN KEY (created_by) REFERENCES public.users(id);


--
-- Name: pages pages_deleted_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.pages
    ADD CONSTRAINT pages_deleted_by_fkey FOREIGN KEY (deleted_by) REFERENCES public.users(id);


--
-- Name: pages pages_owner_group_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.pages
    ADD CONSTRAINT pages_owner_group_id_fkey FOREIGN KEY (owner_group_id) REFERENCES public.wiki_groups(id) ON DELETE SET NULL;


--
-- Name: pages pages_updated_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.pages
    ADD CONSTRAINT pages_updated_by_fkey FOREIGN KEY (updated_by) REFERENCES public.users(id);


--
-- Name: saved_searches saved_searches_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.saved_searches
    ADD CONSTRAINT saved_searches_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: user_groups user_groups_group_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_groups
    ADD CONSTRAINT user_groups_group_id_fkey FOREIGN KEY (group_id) REFERENCES public.wiki_groups(id) ON DELETE CASCADE;


--
-- Name: user_groups user_groups_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_groups
    ADD CONSTRAINT user_groups_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: user_preferences user_preferences_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_preferences
    ADD CONSTRAINT user_preferences_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: webhook_deliveries webhook_deliveries_webhook_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.webhook_deliveries
    ADD CONSTRAINT webhook_deliveries_webhook_id_fkey FOREIGN KEY (webhook_id) REFERENCES public.webhooks(id) ON DELETE CASCADE;


--
-- Name: webhook_headers webhook_headers_webhook_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.webhook_headers
    ADD CONSTRAINT webhook_headers_webhook_id_fkey FOREIGN KEY (webhook_id) REFERENCES public.webhooks(id) ON DELETE CASCADE;


--
-- PostgreSQL database dump complete
--


