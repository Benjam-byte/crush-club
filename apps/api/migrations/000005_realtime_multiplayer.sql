-- +goose Up
ALTER TABLE lobbies
  ADD COLUMN revision bigint NOT NULL DEFAULT 1 CHECK (revision > 0);

ALTER TABLE lobbies
  DROP CONSTRAINT lobbies_max_players_check,
  ADD CONSTRAINT lobbies_max_players_check CHECK (max_players BETWEEN 2 AND 10);

ALTER TABLE players
  ADD COLUMN disconnected_at timestamptz,
  ADD COLUMN excluded_at timestamptz,
  ADD COLUMN updated_at timestamptz NOT NULL DEFAULT now();

CREATE UNIQUE INDEX players_lobby_display_name_ci_idx
  ON players (lobby_id, lower(display_name))
  WHERE excluded_at IS NULL;

CREATE TABLE games (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  lobby_id uuid NOT NULL UNIQUE REFERENCES lobbies(id) ON DELETE CASCADE,
  phase text NOT NULL DEFAULT 'collecting_submissions'
    CHECK (phase IN ('collecting_submissions', 'reveal_and_vote', 'round_results', 'completed')),
  current_round_number integer NOT NULL DEFAULT 1 CHECK (current_round_number > 0),
  total_rounds integer NOT NULL CHECK (total_rounds BETWEEN 2 AND 10),
  started_at timestamptz NOT NULL DEFAULT now(),
  completed_at timestamptz,
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE game_participants (
  game_id uuid NOT NULL REFERENCES games(id) ON DELETE CASCADE,
  player_id uuid NOT NULL REFERENCES players(id) ON DELETE CASCADE,
  seat integer NOT NULL CHECK (seat >= 0),
  is_active boolean NOT NULL DEFAULT true,
  cumulative_score integer NOT NULL DEFAULT 0,
  exact_count integer NOT NULL DEFAULT 0 CHECK (exact_count >= 0),
  tagline_bonus_count integer NOT NULL DEFAULT 0 CHECK (tagline_bonus_count >= 0),
  PRIMARY KEY (game_id, player_id),
  UNIQUE (game_id, seat)
);

CREATE TABLE game_rounds (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  game_id uuid NOT NULL REFERENCES games(id) ON DELETE CASCADE,
  round_number integer NOT NULL CHECK (round_number > 0),
  subject_player_id uuid NOT NULL REFERENCES players(id) ON DELETE RESTRICT,
  status text NOT NULL DEFAULT 'collecting_submissions'
    CHECK (status IN ('collecting_submissions', 'reveal_and_vote', 'round_results', 'skipped')),
  voted_submission_id uuid,
  started_at timestamptz NOT NULL DEFAULT now(),
  completed_at timestamptz,
  UNIQUE (game_id, round_number),
  UNIQUE (game_id, subject_player_id)
);

CREATE TABLE round_submissions (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  round_id uuid NOT NULL REFERENCES game_rounds(id) ON DELETE CASCADE,
  player_id uuid NOT NULL REFERENCES players(id) ON DELETE CASCADE,
  kind text NOT NULL CHECK (kind IN ('official', 'prediction')),
  tagline text,
  bio_answers jsonb NOT NULL DEFAULT '{}'::jsonb,
  question_answers jsonb NOT NULL DEFAULT '{}'::jsonb,
  lover_question_id text,
  base_score integer NOT NULL DEFAULT 0,
  lover_adjustment integer NOT NULL DEFAULT 0,
  tagline_bonus integer NOT NULL DEFAULT 0,
  total_score integer NOT NULL DEFAULT 0,
  exact_count integer NOT NULL DEFAULT 0 CHECK (exact_count >= 0),
  score_lines jsonb NOT NULL DEFAULT '[]'::jsonb,
  submitted_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (round_id, player_id)
);

ALTER TABLE game_rounds
  ADD CONSTRAINT game_rounds_voted_submission_fk
  FOREIGN KEY (voted_submission_id) REFERENCES round_submissions(id) ON DELETE SET NULL;

CREATE INDEX games_lobby_idx ON games(lobby_id);
CREATE INDEX game_rounds_game_idx ON game_rounds(game_id, round_number);
CREATE INDEX round_submissions_round_idx ON round_submissions(round_id);
CREATE INDEX players_reconnect_hash_idx ON players(reconnect_token_hash);

-- +goose Down
DROP INDEX IF EXISTS players_reconnect_hash_idx;
DROP INDEX IF EXISTS round_submissions_round_idx;
DROP INDEX IF EXISTS game_rounds_game_idx;
DROP INDEX IF EXISTS games_lobby_idx;
ALTER TABLE game_rounds DROP CONSTRAINT IF EXISTS game_rounds_voted_submission_fk;
DROP TABLE IF EXISTS round_submissions;
DROP TABLE IF EXISTS game_rounds;
DROP TABLE IF EXISTS game_participants;
DROP TABLE IF EXISTS games;
DROP INDEX IF EXISTS players_lobby_display_name_ci_idx;
ALTER TABLE players
  DROP COLUMN IF EXISTS updated_at,
  DROP COLUMN IF EXISTS excluded_at,
  DROP COLUMN IF EXISTS disconnected_at;
ALTER TABLE lobbies DROP COLUMN IF EXISTS revision;
ALTER TABLE lobbies
  DROP CONSTRAINT lobbies_max_players_check,
  ADD CONSTRAINT lobbies_max_players_check CHECK (max_players BETWEEN 4 AND 10);
