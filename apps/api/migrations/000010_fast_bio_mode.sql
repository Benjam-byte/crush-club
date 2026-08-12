-- +goose Up
ALTER TABLE lobbies
  ADD COLUMN mode text NOT NULL DEFAULT 'classic' CHECK (mode IN ('classic', 'fast_bio'));

ALTER TABLE lobbies
  ALTER COLUMN max_players DROP NOT NULL;

ALTER TABLE lobbies
  DROP CONSTRAINT lobbies_max_players_check,
  ADD CONSTRAINT lobbies_max_players_check CHECK (max_players IS NULL OR max_players BETWEEN 2 AND 10);

CREATE TABLE fast_bio_games (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  lobby_id uuid NOT NULL REFERENCES lobbies(id) ON DELETE CASCADE,
  phase text NOT NULL DEFAULT 'collecting_themes'
    CHECK (phase IN ('collecting_themes', 'ranking_themes', 'playing', 'completed')),
  created_at timestamptz NOT NULL DEFAULT now(),
  completed_at timestamptz
);

CREATE INDEX fast_bio_games_lobby_idx ON fast_bio_games(lobby_id, created_at DESC);

CREATE TABLE fast_bio_theme_submissions (
  game_id uuid NOT NULL REFERENCES fast_bio_games(id) ON DELETE CASCADE,
  player_id uuid NOT NULL REFERENCES players(id) ON DELETE CASCADE,
  theme_label text,
  submitted_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (game_id, player_id)
);

CREATE TABLE fast_bio_theme_rankings (
  game_id uuid NOT NULL REFERENCES fast_bio_games(id) ON DELETE CASCADE,
  voter_player_id uuid NOT NULL REFERENCES players(id) ON DELETE CASCADE,
  ranking jsonb NOT NULL,
  submitted_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (game_id, voter_player_id)
);

CREATE TABLE fast_bio_selected_themes (
  game_id uuid NOT NULL REFERENCES fast_bio_games(id) ON DELETE CASCADE,
  position smallint NOT NULL CHECK (position BETWEEN 1 AND 3),
  theme_label text NOT NULL,
  PRIMARY KEY (game_id, position)
);

CREATE TABLE fast_bio_rounds (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  game_id uuid NOT NULL REFERENCES fast_bio_games(id) ON DELETE CASCADE,
  round_number smallint NOT NULL CHECK (round_number BETWEEN 1 AND 3),
  theme_label text NOT NULL,
  phase text NOT NULL DEFAULT 'submitting'
    CHECK (phase IN ('submitting', 'reviewing', 'completed')),
  submission_deadline timestamptz NOT NULL,
  review_index integer NOT NULL DEFAULT 0,
  started_at timestamptz NOT NULL DEFAULT now(),
  completed_at timestamptz,
  UNIQUE (game_id, round_number)
);

CREATE INDEX fast_bio_rounds_game_idx ON fast_bio_rounds(game_id, round_number);

CREATE TABLE fast_bio_assignments (
  round_id uuid NOT NULL REFERENCES fast_bio_rounds(id) ON DELETE CASCADE,
  author_player_id uuid NOT NULL REFERENCES players(id) ON DELETE CASCADE,
  target_player_id uuid NOT NULL REFERENCES players(id) ON DELETE CASCADE,
  PRIMARY KEY (round_id, author_player_id),
  UNIQUE (round_id, target_player_id),
  CHECK (author_player_id <> target_player_id)
);

CREATE TABLE fast_bio_proposals (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  round_id uuid NOT NULL REFERENCES fast_bio_rounds(id) ON DELETE CASCADE,
  author_player_id uuid NOT NULL REFERENCES players(id) ON DELETE CASCADE,
  target_player_id uuid NOT NULL REFERENCES players(id) ON DELETE CASCADE,
  storage_key text NOT NULL UNIQUE,
  content_type text NOT NULL,
  width integer NOT NULL,
  height integer NOT NULL,
  size_bytes bigint NOT NULL,
  bio text NOT NULL CHECK (char_length(bio) BETWEEN 1 AND 280),
  submitted_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (round_id, author_player_id)
);

CREATE INDEX fast_bio_proposals_round_idx ON fast_bio_proposals(round_id);

CREATE TABLE fast_bio_reactions (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  proposal_id uuid NOT NULL REFERENCES fast_bio_proposals(id) ON DELETE CASCADE,
  voter_player_id uuid NOT NULL REFERENCES players(id) ON DELETE CASCADE,
  emoji text NOT NULL CHECK (emoji IN ('❤️', '😂', '😐', '🤮')),
  points integer NOT NULL CHECK (points >= 0),
  created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (proposal_id, voter_player_id)
);

CREATE INDEX fast_bio_reactions_proposal_idx ON fast_bio_reactions(proposal_id);

-- +goose Down
DROP INDEX IF EXISTS fast_bio_reactions_proposal_idx;
DROP TABLE IF EXISTS fast_bio_reactions;
DROP INDEX IF EXISTS fast_bio_proposals_round_idx;
DROP TABLE IF EXISTS fast_bio_proposals;
DROP TABLE IF EXISTS fast_bio_assignments;
DROP INDEX IF EXISTS fast_bio_rounds_game_idx;
DROP TABLE IF EXISTS fast_bio_rounds;
DROP TABLE IF EXISTS fast_bio_selected_themes;
DROP TABLE IF EXISTS fast_bio_theme_rankings;
DROP TABLE IF EXISTS fast_bio_theme_submissions;
DROP INDEX IF EXISTS fast_bio_games_lobby_idx;
DROP TABLE IF EXISTS fast_bio_games;
ALTER TABLE lobbies
  DROP CONSTRAINT lobbies_max_players_check,
  ADD CONSTRAINT lobbies_max_players_check CHECK (max_players BETWEEN 2 AND 10);
ALTER TABLE lobbies
  ALTER COLUMN max_players SET NOT NULL;
ALTER TABLE lobbies DROP COLUMN IF EXISTS mode;
