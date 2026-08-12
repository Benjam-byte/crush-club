-- +goose Up
ALTER TABLE lobbies
  DROP CONSTRAINT lobbies_mode_check,
  ADD CONSTRAINT lobbies_mode_check CHECK (mode IN ('classic', 'fast_bio', 'zero_to_100', 'situation'));

CREATE TABLE situation_games (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  lobby_id uuid NOT NULL REFERENCES lobbies(id) ON DELETE CASCADE,
  phase text NOT NULL DEFAULT 'collecting_themes'
    CHECK (phase IN ('collecting_themes', 'ranking_themes', 'playing', 'completed')),
  created_at timestamptz NOT NULL DEFAULT now(),
  completed_at timestamptz
);

CREATE INDEX situation_games_lobby_idx ON situation_games(lobby_id, created_at DESC);

CREATE TABLE situation_theme_submissions (
  game_id uuid NOT NULL REFERENCES situation_games(id) ON DELETE CASCADE,
  player_id uuid NOT NULL REFERENCES players(id) ON DELETE CASCADE,
  theme_label text,
  submitted_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (game_id, player_id)
);

CREATE TABLE situation_theme_rankings (
  game_id uuid NOT NULL REFERENCES situation_games(id) ON DELETE CASCADE,
  voter_player_id uuid NOT NULL REFERENCES players(id) ON DELETE CASCADE,
  ranking jsonb NOT NULL,
  submitted_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (game_id, voter_player_id)
);

CREATE TABLE situation_selected_themes (
  game_id uuid NOT NULL REFERENCES situation_games(id) ON DELETE CASCADE,
  position smallint NOT NULL CHECK (position BETWEEN 1 AND 3),
  theme_label text NOT NULL,
  PRIMARY KEY (game_id, position)
);

CREATE TABLE situation_rounds (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  game_id uuid NOT NULL REFERENCES situation_games(id) ON DELETE CASCADE,
  round_number smallint NOT NULL CHECK (round_number BETWEEN 1 AND 3),
  theme_label text NOT NULL,
  phase text NOT NULL DEFAULT 'proposing'
    CHECK (phase IN ('proposing', 'dueling', 'revealing', 'ranking', 'results', 'completed')),
  proposal_deadline timestamptz NOT NULL,
  review_index integer NOT NULL DEFAULT 0,
  started_at timestamptz NOT NULL DEFAULT now(),
  completed_at timestamptz,
  UNIQUE (game_id, round_number)
);

CREATE INDEX situation_rounds_game_idx ON situation_rounds(game_id, round_number);

CREATE TABLE situation_proposals (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  round_id uuid NOT NULL REFERENCES situation_rounds(id) ON DELETE CASCADE,
  author_player_id uuid NOT NULL REFERENCES players(id) ON DELETE CASCADE,
  chosen_player_id uuid NOT NULL REFERENCES players(id) ON DELETE CASCADE,
  reason text NOT NULL CHECK (char_length(reason) BETWEEN 1 AND 100),
  eliminated_at timestamptz,
  submitted_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (round_id, author_player_id),
  CHECK (author_player_id <> chosen_player_id)
);

CREATE INDEX situation_proposals_round_idx ON situation_proposals(round_id);

CREATE TABLE situation_group_members (
  round_id uuid NOT NULL REFERENCES situation_rounds(id) ON DELETE CASCADE,
  player_id uuid NOT NULL REFERENCES players(id) ON DELETE CASCADE,
  proposal_id uuid NOT NULL REFERENCES situation_proposals(id) ON DELETE CASCADE,
  PRIMARY KEY (round_id, player_id)
);

CREATE INDEX situation_group_members_proposal_idx ON situation_group_members(round_id, proposal_id);

CREATE TABLE situation_duels (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  round_id uuid NOT NULL REFERENCES situation_rounds(id) ON DELETE CASCADE,
  wave_number smallint NOT NULL,
  proposal_a_id uuid NOT NULL REFERENCES situation_proposals(id) ON DELETE CASCADE,
  proposal_b_id uuid NOT NULL REFERENCES situation_proposals(id) ON DELETE CASCADE,
  representative_a_player_id uuid NOT NULL REFERENCES players(id) ON DELETE CASCADE,
  representative_b_player_id uuid NOT NULL REFERENCES players(id) ON DELETE CASCADE,
  vote_a_proposal_id uuid REFERENCES situation_proposals(id) ON DELETE SET NULL,
  vote_b_proposal_id uuid REFERENCES situation_proposals(id) ON DELETE SET NULL,
  deadline timestamptz NOT NULL,
  winner_proposal_id uuid REFERENCES situation_proposals(id) ON DELETE SET NULL,
  resolved_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (round_id, wave_number, proposal_a_id)
);

CREATE INDEX situation_duels_round_wave_idx ON situation_duels(round_id, wave_number);

CREATE TABLE situation_final_rankings (
  round_id uuid NOT NULL REFERENCES situation_rounds(id) ON DELETE CASCADE,
  voter_player_id uuid NOT NULL REFERENCES players(id) ON DELETE CASCADE,
  ranking jsonb NOT NULL,
  submitted_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (round_id, voter_player_id)
);

CREATE TABLE situation_round_scores (
  round_id uuid NOT NULL REFERENCES situation_rounds(id) ON DELETE CASCADE,
  player_id uuid NOT NULL REFERENCES players(id) ON DELETE CASCADE,
  points integer NOT NULL,
  PRIMARY KEY (round_id, player_id)
);

-- +goose Down
DROP TABLE IF EXISTS situation_round_scores;
DROP TABLE IF EXISTS situation_final_rankings;
DROP INDEX IF EXISTS situation_duels_round_wave_idx;
DROP TABLE IF EXISTS situation_duels;
DROP INDEX IF EXISTS situation_group_members_proposal_idx;
DROP TABLE IF EXISTS situation_group_members;
DROP INDEX IF EXISTS situation_proposals_round_idx;
DROP TABLE IF EXISTS situation_proposals;
DROP INDEX IF EXISTS situation_rounds_game_idx;
DROP TABLE IF EXISTS situation_rounds;
DROP TABLE IF EXISTS situation_selected_themes;
DROP TABLE IF EXISTS situation_theme_rankings;
DROP TABLE IF EXISTS situation_theme_submissions;
DROP INDEX IF EXISTS situation_games_lobby_idx;
DROP TABLE IF EXISTS situation_games;
ALTER TABLE lobbies
  DROP CONSTRAINT lobbies_mode_check,
  ADD CONSTRAINT lobbies_mode_check CHECK (mode IN ('classic', 'fast_bio', 'zero_to_100'));
