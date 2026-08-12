-- +goose Up
ALTER TABLE lobbies
  DROP CONSTRAINT lobbies_mode_check,
  ADD CONSTRAINT lobbies_mode_check CHECK (mode IN ('classic', 'fast_bio', 'zero_to_100'));

CREATE TABLE zero_to_100_games (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  lobby_id uuid NOT NULL REFERENCES lobbies(id) ON DELETE CASCADE,
  phase text NOT NULL DEFAULT 'collecting_themes'
    CHECK (phase IN ('collecting_themes', 'ranking_themes', 'playing', 'completed')),
  created_at timestamptz NOT NULL DEFAULT now(),
  completed_at timestamptz
);

CREATE INDEX zero_to_100_games_lobby_idx ON zero_to_100_games(lobby_id, created_at DESC);

CREATE TABLE zero_to_100_theme_submissions (
  game_id uuid NOT NULL REFERENCES zero_to_100_games(id) ON DELETE CASCADE,
  player_id uuid NOT NULL REFERENCES players(id) ON DELETE CASCADE,
  theme_label text,
  submitted_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (game_id, player_id)
);

CREATE TABLE zero_to_100_theme_rankings (
  game_id uuid NOT NULL REFERENCES zero_to_100_games(id) ON DELETE CASCADE,
  voter_player_id uuid NOT NULL REFERENCES players(id) ON DELETE CASCADE,
  ranking jsonb NOT NULL,
  submitted_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (game_id, voter_player_id)
);

CREATE TABLE zero_to_100_selected_themes (
  game_id uuid NOT NULL REFERENCES zero_to_100_games(id) ON DELETE CASCADE,
  position smallint NOT NULL CHECK (position BETWEEN 1 AND 3),
  theme_label text NOT NULL,
  PRIMARY KEY (game_id, position)
);

CREATE TABLE zero_to_100_rounds (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  game_id uuid NOT NULL REFERENCES zero_to_100_games(id) ON DELETE CASCADE,
  round_number smallint NOT NULL CHECK (round_number BETWEEN 1 AND 3),
  theme_label text NOT NULL,
  phase text NOT NULL DEFAULT 'guessing'
    CHECK (phase IN ('guessing', 'results', 'completed')),
  submission_deadline timestamptz NOT NULL,
  started_at timestamptz NOT NULL DEFAULT now(),
  completed_at timestamptz,
  UNIQUE (game_id, round_number)
);

CREATE INDEX zero_to_100_rounds_game_idx ON zero_to_100_rounds(game_id, round_number);

CREATE TABLE zero_to_100_nominees (
  round_id uuid NOT NULL REFERENCES zero_to_100_rounds(id) ON DELETE CASCADE,
  player_id uuid NOT NULL REFERENCES players(id) ON DELETE CASCADE,
  seat smallint NOT NULL CHECK (seat BETWEEN 0 AND 2),
  PRIMARY KEY (round_id, player_id),
  UNIQUE (round_id, seat)
);

CREATE TABLE zero_to_100_guesses (
  round_id uuid NOT NULL REFERENCES zero_to_100_rounds(id) ON DELETE CASCADE,
  guesser_player_id uuid NOT NULL REFERENCES players(id) ON DELETE CASCADE,
  nominee_player_id uuid NOT NULL REFERENCES players(id) ON DELETE CASCADE,
  position smallint NOT NULL CHECK (position BETWEEN 0 AND 100),
  points integer,
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (round_id, guesser_player_id, nominee_player_id)
);

CREATE INDEX zero_to_100_guesses_round_idx ON zero_to_100_guesses(round_id);
CREATE INDEX zero_to_100_guesses_nominee_idx ON zero_to_100_guesses(round_id, nominee_player_id);

-- +goose Down
DROP INDEX IF EXISTS zero_to_100_guesses_nominee_idx;
DROP INDEX IF EXISTS zero_to_100_guesses_round_idx;
DROP TABLE IF EXISTS zero_to_100_guesses;
DROP TABLE IF EXISTS zero_to_100_nominees;
DROP INDEX IF EXISTS zero_to_100_rounds_game_idx;
DROP TABLE IF EXISTS zero_to_100_rounds;
DROP TABLE IF EXISTS zero_to_100_selected_themes;
DROP TABLE IF EXISTS zero_to_100_theme_rankings;
DROP TABLE IF EXISTS zero_to_100_theme_submissions;
DROP INDEX IF EXISTS zero_to_100_games_lobby_idx;
DROP TABLE IF EXISTS zero_to_100_games;
ALTER TABLE lobbies
  DROP CONSTRAINT lobbies_mode_check,
  ADD CONSTRAINT lobbies_mode_check CHECK (mode IN ('classic', 'fast_bio'));
