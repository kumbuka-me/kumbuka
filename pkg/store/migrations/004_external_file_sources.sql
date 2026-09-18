-- Approved sources are core-owned records. Guest storage dispatch accepts only
-- settings/data and never lets a plugin choose this namespace.
ALTER TABLE plugin_values DROP CONSTRAINT plugin_values_namespace_check;
ALTER TABLE plugin_values ADD CONSTRAINT plugin_values_namespace_check
  CHECK (namespace IN ('settings', 'data') OR
         (namespace = 'approved-sources' AND plugin_id = 'core.external-files'));
