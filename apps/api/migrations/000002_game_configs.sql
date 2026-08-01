-- +goose Up
CREATE TABLE host_identities (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  session_token_hash bytea NOT NULL UNIQUE,
  created_at timestamptz NOT NULL DEFAULT now(),
  last_seen_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE questions (
  id text PRIMARY KEY,
  type text NOT NULL CHECK (type IN ('single_choice', 'binary_choice', 'integer_range')),
  label text NOT NULL CHECK (char_length(label) BETWEEN 1 AND 160),
  description text,
  maximum_score integer NOT NULL CHECK (maximum_score > 0),
  lover_eligible boolean NOT NULL DEFAULT true,
  options jsonb,
  minimum integer,
  maximum integer,
  minimum_label text,
  maximum_label text,
  is_active boolean NOT NULL DEFAULT true,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  CHECK (
    (type = 'integer_range' AND minimum IS NOT NULL AND maximum IS NOT NULL AND minimum < maximum)
    OR
    (type <> 'integer_range' AND minimum IS NULL AND maximum IS NULL)
  ),
  CHECK (
    (type IN ('single_choice', 'binary_choice') AND jsonb_typeof(options) = 'array' AND jsonb_array_length(options) >= 2)
    OR
    (type = 'integer_range' AND options IS NULL)
  )
);

CREATE TABLE game_configs (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  owner_identity_id uuid REFERENCES host_identities(id) ON DELETE CASCADE,
  name text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 80),
  is_system boolean NOT NULL DEFAULT false,
  version integer NOT NULL DEFAULT 1 CHECK (version > 0),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  CHECK (
    (is_system AND owner_identity_id IS NULL)
    OR
    (NOT is_system AND owner_identity_id IS NOT NULL)
  )
);

CREATE TABLE game_config_questions (
  game_config_id uuid NOT NULL REFERENCES game_configs(id) ON DELETE CASCADE,
  question_id text NOT NULL REFERENCES questions(id) ON DELETE RESTRICT,
  position integer NOT NULL CHECK (position >= 0),
  PRIMARY KEY (game_config_id, question_id),
  UNIQUE (game_config_id, position)
);

ALTER TABLE lobbies
  ADD COLUMN owner_identity_id uuid REFERENCES host_identities(id) ON DELETE SET NULL,
  ADD COLUMN game_config_id uuid REFERENCES game_configs(id) ON DELETE SET NULL,
  ADD COLUMN game_config_version integer,
  ADD COLUMN game_config_snapshot jsonb;

CREATE INDEX game_configs_owner_idx ON game_configs(owner_identity_id);
CREATE INDEX lobbies_owner_identity_idx ON lobbies(owner_identity_id);

INSERT INTO questions (
  id,
  type,
  label,
  description,
  maximum_score,
  lover_eligible,
  options,
  minimum,
  maximum,
  minimum_label,
  maximum_label
)
VALUES
  (
    'romance',
    'integer_range',
    'Niveau de romantisme',
    'De 0 « pas du tout » à 10 « cœur en marshmallow »',
    10,
    true,
    NULL,
    0,
    10,
    'Discret',
    'Très romantique'
  ),
  (
    'love-language',
    'single_choice',
    'Langage de l''amour',
    'Comment Camille montre-t-elle son affection ?',
    10,
    true,
    '[{"id":"words","label":"Mots valorisants"},{"id":"time","label":"Moments de qualité"},{"id":"acts","label":"Petites attentions"},{"id":"touch","label":"Contact physique"}]'::jsonb,
    NULL,
    NULL,
    NULL,
    NULL
  ),
  (
    'first-date',
    'single_choice',
    'Son rendez-vous idéal',
    'Le programme qui lui ressemble le plus',
    10,
    true,
    '[{"id":"restaurant","label":"Un dîner intimiste"},{"id":"picnic","label":"Un pique-nique improvisé"},{"id":"concert","label":"Un concert"},{"id":"escape","label":"Un escape game"}]'::jsonb,
    NULL,
    NULL,
    NULL,
    NULL
  ),
  (
    'weekend',
    'binary_choice',
    'Pour un week-end à deux',
    'Elle préfère…',
    8,
    true,
    '[{"id":"planned","label":"Tout organiser"},{"id":"improvise","label":"Tout improviser"}]'::jsonb,
    NULL,
    NULL,
    NULL,
    NULL
  ),
  (
    'intimacy',
    'integer_range',
    'Importance de parler de ses envies',
    'Une question intime glissée naturellement dans le jeu',
    10,
    true,
    NULL,
    0,
    10,
    'Peu important',
    'Essentiel'
  );

INSERT INTO game_configs (id, name, is_system)
VALUES ('00000000-0000-0000-0000-000000000001', 'Classique', true);

INSERT INTO game_config_questions (game_config_id, question_id, position)
VALUES
  ('00000000-0000-0000-0000-000000000001', 'romance', 0),
  ('00000000-0000-0000-0000-000000000001', 'love-language', 1),
  ('00000000-0000-0000-0000-000000000001', 'first-date', 2),
  ('00000000-0000-0000-0000-000000000001', 'weekend', 3),
  ('00000000-0000-0000-0000-000000000001', 'intimacy', 4);

WITH default_snapshot AS (
  SELECT jsonb_build_object(
    'sourceConfigId', config.id,
    'sourceVersion', config.version,
    'name', config.name,
    'questions', jsonb_agg(
      jsonb_strip_nulls(jsonb_build_object(
        'id', question.id,
        'type', question.type,
        'label', question.label,
        'description', question.description,
        'maximumScore', question.maximum_score,
        'loverEligible', question.lover_eligible,
        'options', question.options,
        'minimum', question.minimum,
        'maximum', question.maximum,
        'minimumLabel', question.minimum_label,
        'maximumLabel', question.maximum_label
      )) ORDER BY item.position
    )
  ) AS snapshot
  FROM game_configs config
  JOIN game_config_questions item ON item.game_config_id = config.id
  JOIN questions question ON question.id = item.question_id
  WHERE config.id = '00000000-0000-0000-0000-000000000001'
  GROUP BY config.id
)
UPDATE lobbies
SET
  game_config_id = '00000000-0000-0000-0000-000000000001',
  game_config_version = 1,
  game_config_snapshot = default_snapshot.snapshot
FROM default_snapshot
WHERE lobbies.game_config_snapshot IS NULL;

ALTER TABLE lobbies
  ALTER COLUMN game_config_version SET NOT NULL,
  ALTER COLUMN game_config_snapshot SET NOT NULL;

-- +goose Down
ALTER TABLE lobbies
  DROP COLUMN IF EXISTS game_config_snapshot,
  DROP COLUMN IF EXISTS game_config_version,
  DROP COLUMN IF EXISTS game_config_id,
  DROP COLUMN IF EXISTS owner_identity_id;
DROP TABLE IF EXISTS game_config_questions;
DROP TABLE IF EXISTS game_configs;
DROP TABLE IF EXISTS questions;
DROP TABLE IF EXISTS host_identities;
