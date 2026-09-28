-- Add a per-user interface locale. Empty keeps browser language negotiation enabled.
ALTER TABLE user_preferences
  ADD COLUMN locale text DEFAULT ''::text NOT NULL;
