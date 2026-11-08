-- +goose Up
ALTER TABLE lobbies
  DROP CONSTRAINT lobbies_mode_check,
  ADD CONSTRAINT lobbies_mode_check
    CHECK (mode IN ('classic', 'fast_bio', 'zero_to_100', 'situation', 'wavelength', 'secret_ranking'));

-- « Le classement secret »: the group votes three questions ("Pour combien
-- d'argent accepterais-tu de … ?"), then plays one round per question.
CREATE TABLE secret_ranking_games (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  lobby_id uuid NOT NULL REFERENCES lobbies(id) ON DELETE CASCADE,
  phase text NOT NULL DEFAULT 'collecting_themes'
    CHECK (phase IN ('collecting_themes', 'ranking_themes', 'playing', 'completed')),
  theme_phase_deadline timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  completed_at timestamptz
);

CREATE INDEX secret_ranking_games_lobby_idx ON secret_ranking_games(lobby_id, created_at DESC);

-- theme_label is NULL when the player passed.
CREATE TABLE secret_ranking_theme_submissions (
  game_id uuid NOT NULL REFERENCES secret_ranking_games(id) ON DELETE CASCADE,
  player_id uuid NOT NULL REFERENCES players(id) ON DELETE CASCADE,
  theme_label text,
  submitted_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (game_id, player_id)
);

CREATE TABLE secret_ranking_theme_rankings (
  game_id uuid NOT NULL REFERENCES secret_ranking_games(id) ON DELETE CASCADE,
  voter_player_id uuid NOT NULL REFERENCES players(id) ON DELETE CASCADE,
  ranking jsonb NOT NULL,
  submitted_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (game_id, voter_player_id)
);

CREATE TABLE secret_ranking_selected_themes (
  game_id uuid NOT NULL REFERENCES secret_ranking_games(id) ON DELETE CASCADE,
  position smallint NOT NULL CHECK (position BETWEEN 1 AND 3),
  theme_label text NOT NULL,
  PRIMARY KEY (game_id, position)
);

-- A round goes secret (everyone bets an amount) → placing (phones lined up
-- on the table and numbered) → revealing (domino reveal, scored) → results
-- (debrief) → completed. lock_at is set while the numbers form a full
-- permutation: the reveal starts when it passes. reveal_at is when phone #1
-- flips. positions_reset_at is when a departure wiped the numbers, so the
-- phones can ask the table to number again.
CREATE TABLE secret_ranking_rounds (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  game_id uuid NOT NULL REFERENCES secret_ranking_games(id) ON DELETE CASCADE,
  round_number smallint NOT NULL CHECK (round_number BETWEEN 1 AND 3),
  theme_label text NOT NULL,
  phase text NOT NULL DEFAULT 'secret'
    CHECK (phase IN ('secret', 'placing', 'revealing', 'results', 'completed')),
  secret_deadline timestamptz NOT NULL,
  lock_at timestamptz,
  positions_reset_at timestamptz,
  reveal_at timestamptz,
  well_placed_count integer,
  participant_count integer,
  cancelled boolean NOT NULL DEFAULT false,
  started_at timestamptz NOT NULL DEFAULT now(),
  completed_at timestamptz,
  UNIQUE (game_id, round_number)
);

CREATE INDEX secret_ranking_rounds_game_idx ON secret_ranking_rounds(game_id, round_number);

-- One row per participant of a round: a player takes part once they bet.
CREATE TABLE secret_ranking_entries (
  round_id uuid NOT NULL REFERENCES secret_ranking_rounds(id) ON DELETE CASCADE,
  player_id uuid NOT NULL REFERENCES players(id) ON DELETE CASCADE,
  amount bigint NOT NULL CHECK (amount BETWEEN 0 AND 1000000000),
  position smallint CHECK (position >= 1),
  well_placed boolean,
  true_rank_min smallint,
  true_rank_max smallint,
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (round_id, player_id)
);

-- +goose Down
DROP TABLE IF EXISTS secret_ranking_entries;
DROP INDEX IF EXISTS secret_ranking_rounds_game_idx;
DROP TABLE IF EXISTS secret_ranking_rounds;
DROP TABLE IF EXISTS secret_ranking_selected_themes;
DROP TABLE IF EXISTS secret_ranking_theme_rankings;
DROP TABLE IF EXISTS secret_ranking_theme_submissions;
DROP INDEX IF EXISTS secret_ranking_games_lobby_idx;
DROP TABLE IF EXISTS secret_ranking_games;
ALTER TABLE lobbies
  DROP CONSTRAINT lobbies_mode_check,
  ADD CONSTRAINT lobbies_mode_check CHECK (mode IN ('classic', 'fast_bio', 'zero_to_100', 'situation', 'wavelength'));
