-- +goose Up
ALTER TABLE lobbies
  DROP CONSTRAINT lobbies_mode_check,
  ADD CONSTRAINT lobbies_mode_check CHECK (mode IN ('classic', 'fast_bio', 'zero_to_100', 'situation', 'wavelength'));

-- A Wavelength game cycles through the theme phases once per round: the
-- group votes a fresh spectrum before every round, so round_number tracks the
-- round being prepared or played.
CREATE TABLE wavelength_games (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  lobby_id uuid NOT NULL REFERENCES lobbies(id) ON DELETE CASCADE,
  phase text NOT NULL DEFAULT 'collecting_themes'
    CHECK (phase IN ('collecting_themes', 'ranking_themes', 'playing', 'completed')),
  round_number smallint NOT NULL DEFAULT 1 CHECK (round_number >= 1),
  theme_phase_deadline timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  completed_at timestamptz
);

CREATE INDEX wavelength_games_lobby_idx ON wavelength_games(lobby_id, created_at DESC);

-- Both labels are NULL when the player passed.
CREATE TABLE wavelength_theme_submissions (
  game_id uuid NOT NULL REFERENCES wavelength_games(id) ON DELETE CASCADE,
  round_number smallint NOT NULL,
  player_id uuid NOT NULL REFERENCES players(id) ON DELETE CASCADE,
  left_label text,
  right_label text,
  submitted_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (game_id, round_number, player_id),
  CHECK ((left_label IS NULL) = (right_label IS NULL))
);

CREATE TABLE wavelength_theme_rankings (
  game_id uuid NOT NULL REFERENCES wavelength_games(id) ON DELETE CASCADE,
  round_number smallint NOT NULL,
  voter_player_id uuid NOT NULL REFERENCES players(id) ON DELETE CASCADE,
  ranking jsonb NOT NULL,
  submitted_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (game_id, round_number, voter_player_id)
);

CREATE TABLE wavelength_rounds (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  game_id uuid NOT NULL REFERENCES wavelength_games(id) ON DELETE CASCADE,
  round_number smallint NOT NULL CHECK (round_number >= 1),
  left_label text NOT NULL,
  right_label text NOT NULL,
  psychic_player_id uuid NOT NULL REFERENCES players(id) ON DELETE CASCADE,
  target smallint NOT NULL CHECK (target BETWEEN 0 AND 100),
  phase text NOT NULL DEFAULT 'clue'
    CHECK (phase IN ('clue', 'guessing', 'results', 'completed')),
  phase_deadline timestamptz NOT NULL,
  psychic_points integer,
  started_at timestamptz NOT NULL DEFAULT now(),
  completed_at timestamptz,
  UNIQUE (game_id, round_number)
);

CREATE INDEX wavelength_rounds_game_idx ON wavelength_rounds(game_id, round_number);

CREATE TABLE wavelength_guesses (
  round_id uuid NOT NULL REFERENCES wavelength_rounds(id) ON DELETE CASCADE,
  player_id uuid NOT NULL REFERENCES players(id) ON DELETE CASCADE,
  position smallint NOT NULL CHECK (position BETWEEN 0 AND 100),
  points integer,
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (round_id, player_id)
);

-- +goose Down
DROP TABLE IF EXISTS wavelength_guesses;
DROP INDEX IF EXISTS wavelength_rounds_game_idx;
DROP TABLE IF EXISTS wavelength_rounds;
DROP TABLE IF EXISTS wavelength_theme_rankings;
DROP TABLE IF EXISTS wavelength_theme_submissions;
DROP INDEX IF EXISTS wavelength_games_lobby_idx;
DROP TABLE IF EXISTS wavelength_games;
ALTER TABLE lobbies
  DROP CONSTRAINT lobbies_mode_check,
  ADD CONSTRAINT lobbies_mode_check CHECK (mode IN ('classic', 'fast_bio', 'zero_to_100', 'situation'));
