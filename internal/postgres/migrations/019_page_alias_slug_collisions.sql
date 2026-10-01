-- A page slug is authoritative over any historical alias using the same path.
-- Earlier move behavior could leave such rows behind when a page returned to an old slug.
DELETE FROM page_aliases AS alias
USING pages AS page
WHERE page.deleted_at IS NULL
  AND page.slug = alias.alias;
