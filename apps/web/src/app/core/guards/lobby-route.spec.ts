import { describe, expect, it } from 'vitest';
import type { LobbyStateResponse } from '@core/models/game.models';
import { permittedLobbyUrl, uncappedModeDestination } from './lobby-route';

function baseState(overrides: Partial<LobbyStateResponse>): LobbyStateResponse {
  return {
    revision: 1,
    serverTime: new Date().toISOString(),
    code: 'ABCD',
    status: 'in_game',
    mode: 'fast_bio',
    maxPlayers: null,
    currentPlayerId: 'p1',
    players: [],
    gameConfig: { id: 'cfg', name: 'Classique', version: 1, questionCount: 0 },
    questionnaire: { sourceConfigId: 'cfg', sourceVersion: 1, name: 'Classique', questions: [], profileFields: [] },
    ...overrides,
  };
}

describe('permittedLobbyUrl for the crowd-sourced-theme modes', () => {
  it('permits the theme-selection page for a fast_bio lobby collecting themes', () => {
    const state = baseState({
      mode: 'fast_bio',
      fastBioGame: {
        id: 'g1',
        phase: 'collecting_themes',
        themeSubmitted: false,
        themeRanked: false,
        submitted: false,
      },
    });
    expect(permittedLobbyUrl(state, '/game/ABCD/theme-selection')).toBe('/game/ABCD/theme-selection');
  });

  it('permits the assignment page for a fast_bio round in progress, not the lobby', () => {
    const state = baseState({
      mode: 'fast_bio',
      fastBioGame: {
        id: 'g1',
        phase: 'playing',
        themeSubmitted: true,
        themeRanked: true,
        roundNumber: 1,
        totalRounds: 3,
        roundPhase: 'submitting',
        submitted: false,
      },
    });
    // Regression: this guard used to only know about the classic `state.game`
    // field (always undefined for fast_bio), so it silently bounced every
    // fast_bio navigation back to the lobby.
    expect(permittedLobbyUrl(state, '/game/ABCD/fast-bio/1/assignment')).toBe('/game/ABCD/fast-bio/1/assignment');
    expect(permittedLobbyUrl(state, '/lobby/ABCD')).toBe('/game/ABCD/fast-bio/1/assignment');
  });

  it('never permits the classic 4-photo page for an uncapped mode', () => {
    const state = baseState({ mode: 'fast_bio', status: 'waiting_for_players' });
    expect(permittedLobbyUrl(state, '/lobby/ABCD/photos')).toBe('/lobby/ABCD');
  });

  it('routes a situation duel phase to the duel page', () => {
    const state = baseState({
      mode: 'situation',
      situationGame: {
        id: 'g1',
        phase: 'playing',
        themeSubmitted: true,
        themeRanked: true,
        roundNumber: 2,
        totalRounds: 3,
        roundPhase: 'dueling',
        submitted: true,
      },
    });
    expect(uncappedModeDestination(state)).toBe('/game/ABCD/situation/2/duel');
  });

  it('sends an uncapped-mode lobby that has not started back to the waiting room', () => {
    const state = baseState({ mode: 'zero_to_100', status: 'waiting_for_players' });
    expect(uncappedModeDestination(state)).toBe('/lobby/ABCD');
  });
});
