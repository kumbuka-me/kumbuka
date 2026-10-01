-- Rendered page artifacts created before canonical stable-ID URLs may contain
-- obsolete slug routes or application URLs without the deployment prefix.
UPDATE pages
SET rendered_html = '',
    rendered_contents = '[]'::jsonb,
    render_fingerprint = '',
    rendered_at = NULL;
