import type { LobbyPlayer, LobbyStateResponse } from '@core/models/game.models';

export interface LobbyNotice {
  /** Stable per event, so the same notice is never queued twice. */
  id: string
  kind: 'joined' | 'join-request'
  message: string
}

/**
 * Notices worth a toast between two snapshots of the same session: players the host let into the
 * game (for everyone) and new requests to join it (for the host only).
 */
export function collectLobbyNotices(
  previous: LobbyStateResponse | null,
  next: LobbyStateResponse,
): readonly LobbyNotice[] {
  if (previous?.code !== next.code || previous.currentPlayerId !== next.currentPlayerId) {
    return [];
  }
  if (next.currentPlayerAdmission === 'pending') {
    return [];
  }
  const previouslyPendingIds = new Set((previous.pendingPlayers ?? []).map((player) => player.id));
  const notices: LobbyNotice[] = [];
  for (const player of next.players) {
    if (!previouslyPendingIds.has(player.id)) {
      continue;
    }
    notices.push({
      id: `joined:${player.id}`,
      kind: 'joined',
      message: player.id === next.currentPlayerId ? 'Tu rejoins la partie !' : `${player.displayName} rejoint la partie`,
    });
  }
  const isHost = next.players.some((player) => player.id === next.currentPlayerId && player.isHost);
  if (isHost) {
    for (const player of next.pendingPlayers ?? []) {
      if (!previouslyPendingIds.has(player.id)) {
        notices.push({
          id: `request:${player.id}`,
          kind: 'join-request',
          message: `${player.displayName} demande à rejoindre la partie`,
        });
      }
    }
  }
  return notices;
}

/** The players a phase is waiting for, in the server's order, the current player left out. */
export function resolveAwaitingPlayers(
  awaitingPlayerIds: readonly string[],
  players: readonly LobbyPlayer[],
): readonly LobbyPlayer[] {
  const playerById = new Map(players.map((player) => [player.id, player]));
  return awaitingPlayerIds
    .map((id) => playerById.get(id))
    .filter((player): player is LobbyPlayer => player !== undefined && !player.isCurrentPlayer);
}
