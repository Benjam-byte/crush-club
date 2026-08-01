-- +goose Up
ALTER TABLE questions
  ADD COLUMN owner_identity_id uuid REFERENCES host_identities(id) ON DELETE CASCADE,
  ADD COLUMN is_system boolean NOT NULL DEFAULT true;

ALTER TABLE questions
  ADD CONSTRAINT questions_owner_consistency CHECK (
    (is_system AND owner_identity_id IS NULL)
    OR
    (NOT is_system AND owner_identity_id IS NOT NULL)
  );

CREATE INDEX questions_owner_idx ON questions(owner_identity_id);

-- +goose Down
DROP INDEX IF EXISTS questions_owner_idx;
ALTER TABLE questions DROP CONSTRAINT IF EXISTS questions_owner_consistency;
ALTER TABLE questions
  DROP COLUMN IF EXISTS is_system,
  DROP COLUMN IF EXISTS owner_identity_id;
