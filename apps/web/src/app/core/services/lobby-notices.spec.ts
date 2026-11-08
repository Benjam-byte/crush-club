import { describe, expect, it } from 'vitest';
import type { LobbyPlayer, LobbyStateResponse } from '@core/models/game.models';
import { collectLobbyNotices, resolveAwaitingPlayers } from './lobby-notices';

function player(id: string, overrides: Partial<LobbyPlayer> = {}): Omit<LobbyPlayer, 'isCurrentPlayer'> {
  return {
    id,
    displayName: id.toUpperCase(),
    isHost: false,
    readyStatus: 'ready',
    connected: true,
    canExclude: false,
    photoIds: [],
    joinedAt: '2026-11-08T10:00:00Z',
    ...overrides,
  };
}

function state(overrides: Partial<LobbyStateResponse>): LobbyStateResponse {
  return {
    revision: 1,
    serverTime: '2026-11-08T10:00:00Z',
    code: 'ROOM',
    status: 'in_game',
    mode: 'wavelength',
    maxPlayers: null,
    currentPlayerId: 'host',
    currentPlayerAdmission: 'admitted',
    players: [player('host', { isHost: true }), player('ana')],
    pendingPlayers: [],
    awaitingPlayerIds: [],
    gameConfig: { id: 'config', name: 'Config', version: 1, questionCount: 0 },
    questionnaire: { sourceConfigId: 'config', sourceVersion: 1, name: 'Config', questions: [], profileFields: [] },
    ...overrides,
  };
}

describe('collectLobbyNotices', () => {
  it('announces an admitted player to everyone', () => {
    const previous = state({ currentPlayerId: 'ana', pendingPlayers: [player('zoe')] });
    const next = state({ currentPlayerId: 'ana', players: [...previous.players, player('zoe')], pendingPlayers: [] });
    expect(collectLobbyNotices(previous, next)).toEqual([
      { id: 'joined:zoe', kind: 'joined', message: 'ZOE rejoint la partie' },
    ]);
  });

  it('welcomes the admitted player themselves', () => {
    const previous = state({ currentPlayerId: 'zoe', currentPlayerAdmission: 'pending', pendingPlayers: [player('zoe')] });
    const next = state({ currentPlayerId: 'zoe', players: [...previous.players, player('zoe')], pendingPlayers: [] });
    expect(collectLobbyNotices(previous, next).map((notice) => notice.message)).toEqual(['Tu rejoins la partie !']);
  });

  it('tells only the host about new join requests', () => {
    const previous = state({});
    const next = state({ pendingPlayers: [player('zoe')] });
    expect(collectLobbyNotices(previous, next)).toEqual([
      { id: 'request:zoe', kind: 'join-request', message: 'ZOE demande à rejoindre la partie' },
    ]);
    expect(collectLobbyNotices({ ...previous, currentPlayerId: 'ana' }, { ...next, currentPlayerId: 'ana' })).toEqual([]);
  });

  it('stays quiet on the first snapshot, a refused request, another session or while pending', () => {
    const withRequest = state({ pendingPlayers: [player('zoe')] });
    expect(collectLobbyNotices(null, withRequest)).toEqual([]);
    expect(collectLobbyNotices(withRequest, state({ pendingPlayers: [] }))).toEqual([]);
    expect(collectLobbyNotices(state({ code: 'OTHER' }), withRequest)).toEqual([]);
    const pending = state({ currentPlayerId: 'zoe', currentPlayerAdmission: 'pending', pendingPlayers: [player('zoe'), player('max')] });
    expect(collectLobbyNotices(state({ currentPlayerId: 'zoe', pendingPlayers: [player('zoe')] }), pending)).toEqual([]);
  });
});

describe('resolveAwaitingPlayers', () => {
  it('keeps the server order, skips unknown ids and leaves the current player out', () => {
    const players: LobbyPlayer[] = [
      { ...player('host', { isHost: true }), isCurrentPlayer: true },
      { ...player('ana'), isCurrentPlayer: false },
      { ...player('bob'), isCurrentPlayer: false },
    ];
    expect(resolveAwaitingPlayers(['bob', 'gone', 'host', 'ana'], players).map((entry) => entry.id)).toEqual(['bob', 'ana']);
  });
});
