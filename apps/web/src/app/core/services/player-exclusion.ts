import { signal } from '@angular/core';
import type { GamePhase, LobbyPlayer, LobbyStatus } from '@core/models/game.models';

export function canHostExcludePlayer(isHost: boolean, player: LobbyPlayer): boolean {
  return isHost && !player.isCurrentPlayer && !player.isHost && !player.connected && player.canExclude;
}

export function activeGameExclusionCandidates(
  isHost: boolean,
  lobbyStatus: LobbyStatus,
  phase: GamePhase | undefined,
  players: readonly LobbyPlayer[],
): readonly LobbyPlayer[] {
  if (!isHost || lobbyStatus !== 'in_game' || !phase || phase === 'between_rounds' || phase === 'completed') {
    return [];
  }
  return players.filter((player) => canHostExcludePlayer(true, player));
}

export class PlayerExclusionController {
  private readonly pendingPlayerIdSet = signal<ReadonlySet<string>>(new Set());
  readonly pendingPlayerIds = this.pendingPlayerIdSet.asReadonly();

  isPending(playerId: string): boolean {
    return this.pendingPlayerIdSet().has(playerId);
  }

  async run(playerId: string, action: () => Promise<void>): Promise<boolean> {
    if (this.isPending(playerId)) {
      return false;
    }
    this.pendingPlayerIdSet.update((current) => new Set([...current, playerId]));
    try {
      await action();
      return true;
    } finally {
      this.pendingPlayerIdSet.update((current) => {
        const next = new Set(current);
        next.delete(playerId);
        return next;
      });
    }
  }

  clear(): void {
    this.pendingPlayerIdSet.set(new Set());
  }
}
