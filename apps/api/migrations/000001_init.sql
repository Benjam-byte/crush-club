-- +goose Up
CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TYPE lobby_status AS ENUM (
  'waiting_for_players',
  'preparing_photos',
  'ready_to_start',
  'in_game',
  'completed',
  'expired'
);

CREATE TYPE player_ready_status AS ENUM ('joining', 'preparing_photos', 'ready');

CREATE TABLE lobbies (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  code text NOT NULL UNIQUE,
  status lobby_status NOT NULL DEFAULT 'waiting_for_players',
  max_players integer NOT NULL DEFAULT 8 CHECK (max_players BETWEEN 4 AND 10),
  settings jsonb NOT NULL DEFAULT '{}'::jsonb,
  host_player_id uuid,
  expires_at timestamptz NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE players (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  lobby_id uuid NOT NULL REFERENCES lobbies(id) ON DELETE CASCADE,
  display_name text NOT NULL CHECK (char_length(display_name) BETWEEN 1 AND 32),
  color text,
  is_host boolean NOT NULL DEFAULT false,
  adult_confirmed boolean NOT NULL,
  ready_status player_ready_status NOT NULL DEFAULT 'joining',
  reconnect_token_hash text NOT NULL,
  joined_at timestamptz NOT NULL DEFAULT now(),
  last_seen_at timestamptz NOT NULL DEFAULT now()
);

ALTER TABLE lobbies
  ADD CONSTRAINT lobbies_host_player_fk
  FOREIGN KEY (host_player_id) REFERENCES players(id) ON DELETE SET NULL;

CREATE TABLE player_photos (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  player_id uuid NOT NULL REFERENCES players(id) ON DELETE CASCADE,
  storage_key text NOT NULL UNIQUE,
  position smallint NOT NULL CHECK (position BETWEEN 1 AND 4),
  width integer NOT NULL,
  height integer NOT NULL,
  content_type text NOT NULL,
  size_bytes bigint NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (player_id, position)
);

CREATE INDEX players_lobby_idx ON players(lobby_id);
CREATE INDEX lobbies_expires_idx ON lobbies(expires_at);

-- +goose Down
DROP TABLE IF EXISTS player_photos;
ALTER TABLE lobbies DROP CONSTRAINT IF EXISTS lobbies_host_player_fk;
DROP TABLE IF EXISTS players;
DROP TABLE IF EXISTS lobbies;
DROP TYPE IF EXISTS player_ready_status;
DROP TYPE IF EXISTS lobby_status;
