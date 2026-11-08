import { describe, expect, it } from 'vitest';
import type { LobbyStateResponse } from '@core/models/game.models';
import { findLobbyCodeInUrl, permittedLobbyUrl, shouldConfirmLobbyExit, uncappedModeDestination } from './lobby-route';

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

  it('keeps the wavelength clue and guessing phases on the same round page', () => {
    const wavelengthState = (roundPhase: 'clue' | 'guessing' | 'results') => baseState({
      mode: 'wavelength',
      wavelengthGame: {
        id: 'g1',
        phase: 'playing',
        themeSubmitted: true,
        themeRanked: true,
        roundNumber: 2,
        totalRounds: 4,
        roundPhase,
        submitted: false,
      },
    });
    expect(uncappedModeDestination(wavelengthState('clue'))).toBe('/game/ABCD/wavelength/2/play');
    expect(uncappedModeDestination(wavelengthState('guessing'))).toBe('/game/ABCD/wavelength/2/play');
    expect(uncappedModeDestination(wavelengthState('results'))).toBe('/game/ABCD/wavelength/2/results');
  });

  it('sends a wavelength game back to theme selection between rounds', () => {
    const state = baseState({
      mode: 'wavelength',
      wavelengthGame: {
        id: 'g1',
        phase: 'collecting_themes',
        themeSubmitted: false,
        themeRanked: false,
        roundNumber: 3,
        submitted: false,
      },
    });
    expect(permittedLobbyUrl(state, '/game/ABCD/wavelength/2/results')).toBe('/game/ABCD/theme-selection');
  });

  it('sends an uncapped-mode lobby that has not started back to the waiting room', () => {
    const state = baseState({ mode: 'zero_to_100', status: 'waiting_for_players' });
    expect(uncappedModeDestination(state)).toBe('/lobby/ABCD');
  });
});

describe('findLobbyCodeInUrl', () => {
  it('reads the code of lobby and game URLs', () => {
    expect(findLobbyCodeInUrl('/lobby/abcd')).toBe('ABCD');
    expect(findLobbyCodeInUrl('/lobby/ABCD/photos?step=2')).toBe('ABCD');
    expect(findLobbyCodeInUrl('/game/ABCD/round/1/role')).toBe('ABCD');
  });

  it('returns null outside the lobby flow', () => {
    expect(findLobbyCodeInUrl('/')).toBeNull();
    expect(findLobbyCodeInUrl('/join?code=ABCD')).toBeNull();
  });
});

describe('shouldConfirmLobbyExit', () => {
  it('asks before leaving the lobby flow with a live session', () => {
    expect(shouldConfirmLobbyExit('/lobby/ABCD', '/', 'ABCD')).toBe(true);
    expect(shouldConfirmLobbyExit('/game/ABCD/round/2/reveal', '/join?code=ABCD', 'ABCD')).toBe(true);
  });

  it('lets the game move between its own pages', () => {
    expect(shouldConfirmLobbyExit('/lobby/ABCD', '/game/ABCD/round/1/role', 'ABCD')).toBe(false);
    expect(shouldConfirmLobbyExit('/game/ABCD/round/1/scores', '/lobby/ABCD', 'ABCD')).toBe(false);
  });

  it('lets the player go once the session is over or belongs to another lobby', () => {
    expect(shouldConfirmLobbyExit('/lobby/ABCD', '/', null)).toBe(false);
    expect(shouldConfirmLobbyExit('/lobby/ABCD', '/', 'WXYZ')).toBe(false);
  });

  it('never asks on pages outside the lobby flow', () => {
    expect(shouldConfirmLobbyExit('/join', '/', 'ABCD')).toBe(false);
  });
});
