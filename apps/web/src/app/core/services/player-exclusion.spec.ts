import { describe, expect, it, vi } from 'vitest';
import type { LobbyPlayer } from '@core/models/game.models';
import {
  activeGameExclusionCandidates,
  canHostExcludePlayer,
  PlayerExclusionController,
} from './player-exclusion';

function player(overrides: Partial<LobbyPlayer> = {}): LobbyPlayer {
  return {
    id: 'guest-1',
    displayName: 'Camille',
    isHost: false,
    isCurrentPlayer: false,
    readyStatus: 'ready',
    connected: false,
    disconnectedAt: new Date().toISOString(),
    canExclude: true,
    photoIds: [],
    joinedAt: new Date().toISOString(),
    ...overrides,
  };
}

describe('player exclusion visibility', () => {
  it('offers exclusion to the host as soon as another player is disconnected', () => {
    expect(canHostExcludePlayer(true, player())).toBe(true);
    expect(canHostExcludePlayer(false, player())).toBe(false);
    expect(canHostExcludePlayer(true, player({ connected: true, canExclude: false }))).toBe(false);
    expect(canHostExcludePlayer(true, player({ isHost: true }))).toBe(false);
    expect(canHostExcludePlayer(true, player({ isCurrentPlayer: true }))).toBe(false);
  });

  it('shows active-game candidates in every phase outside the lobby and hides reconnected players', () => {
    const disconnected = player();
    const connected = player({ id: 'guest-2', connected: true, canExclude: false });

    for (const phase of ['collecting_submissions', 'reveal_and_vote', 'round_results'] as const) {
      expect(activeGameExclusionCandidates(true, 'in_game', phase, [disconnected, connected])).toEqual([
        disconnected,
      ]);
    }
    expect(activeGameExclusionCandidates(true, 'in_game', 'between_rounds', [disconnected])).toEqual([]);
    expect(activeGameExclusionCandidates(false, 'in_game', 'collecting_submissions', [disconnected])).toEqual([]);
  });
});

describe('PlayerExclusionController', () => {
  it('prevents duplicate requests for the same player and unlocks after completion', async () => {
    const controller = new PlayerExclusionController();
    let finishRequest: (() => void) | undefined;
    const action = vi.fn(() => new Promise<void>((resolve) => {
      finishRequest = resolve;
    }));

    const firstRequest = controller.run('guest-1', action);
    const duplicateRequest = controller.run('guest-1', action);

    expect(controller.isPending('guest-1')).toBe(true);
    await expect(duplicateRequest).resolves.toBe(false);
    expect(action).toHaveBeenCalledOnce();
    finishRequest?.();
    await expect(firstRequest).resolves.toBe(true);
    expect(controller.isPending('guest-1')).toBe(false);
  });

  it('unlocks the player when the request fails', async () => {
    const controller = new PlayerExclusionController();

    await expect(controller.run('guest-1', async () => {
      throw new Error('network');
    })).rejects.toThrow('network');
    expect(controller.isPending('guest-1')).toBe(false);
  });
});
