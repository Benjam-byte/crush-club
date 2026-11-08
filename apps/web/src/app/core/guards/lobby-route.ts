import type { LobbyMode, LobbyStateResponse } from '@core/models/game.models';

export function shouldApplyLobbySnapshot(
  current: LobbyStateResponse | null,
  next: LobbyStateResponse,
): boolean {
  return current === null || next.code !== current.code || next.revision > current.revision;
}

export function activeNavigationUrl(currentUrl: string, pendingUrl?: string): string {
  return (pendingUrl ?? currentUrl).split('?')[0];
}

/** Code of the lobby a `/lobby/…` or `/game/…` URL belongs to, or null for any other page. */
export function findLobbyCodeInUrl(url: string): string | null {
  return /^\/(?:lobby|game)\/([^/?#]+)/.exec(url)?.[1]?.toUpperCase() ?? null;
}

/**
 * Leaving the lobby flow costs the player their place, so it must be confirmed — unless they
 * no longer hold a live session for this lobby (already left, excluded, lobby gone).
 */
export function shouldConfirmLobbyExit(currentUrl: string, destinationUrl: string, activeSessionCode: string | null): boolean {
  const currentCode = findLobbyCodeInUrl(currentUrl);
  if (currentCode === null || currentCode !== activeSessionCode) {
    return false;
  }
  return findLobbyCodeInUrl(destinationUrl) !== currentCode;
}

/** Where a player who joined a game in progress waits for the host's approval. */
export function pendingApprovalUrl(code: string): string {
  return `/lobby/${code}/pending`;
}

export function isPendingAdmission(state: LobbyStateResponse): boolean {
  return state.currentPlayerAdmission === 'pending';
}

/** Modes with crowd-sourced themes and no classic photo-preparation step. */
export function isUncappedLobbyMode(mode: LobbyMode): boolean {
  return mode === 'fast_bio' || mode === 'zero_to_100' || mode === 'situation' || mode === 'wavelength' ||
    mode === 'secret_ranking';
}

/**
 * Expected route for the "crowd-sourced theme/situation + rounds" modes
 * (Fast Bio, 0 à 100, Situation, Wavelength), which all share the same
 * top-level shape: theme selection, then per-round pages, then a final
 * leaderboard (Wavelength goes back to theme selection before every round).
 * Returns the lobby URL whenever there is no game in progress yet, so callers
 * that only care about "is this URL allowed" don't need a separate
 * not-started branch.
 */
export function uncappedModeDestination(state: LobbyStateResponse): string {
  const lobbyUrl = `/lobby/${state.code}`;
  if (isPendingAdmission(state)) {
    return pendingApprovalUrl(state.code);
  }
  if (state.status !== 'in_game') {
    return lobbyUrl;
  }
  if (state.mode === 'fast_bio') {
    const fastBio = state.fastBioGame;
    if (!fastBio) {
      return lobbyUrl;
    }
    if (fastBio.phase === 'collecting_themes' || fastBio.phase === 'ranking_themes') {
      return `/game/${state.code}/theme-selection`;
    }
    if (fastBio.phase === 'playing' && fastBio.roundNumber) {
      const roundBase = `/game/${state.code}/fast-bio/${fastBio.roundNumber}`;
      return fastBio.roundPhase === 'submitting' ? `${roundBase}/assignment` : `${roundBase}/review`;
    }
    if (fastBio.phase === 'completed') {
      return `/game/${state.code}/fast-bio/final`;
    }
    return lobbyUrl;
  }
  if (state.mode === 'zero_to_100') {
    const zeroToHundred = state.zeroToHundredGame;
    if (!zeroToHundred) {
      return lobbyUrl;
    }
    if (zeroToHundred.phase === 'collecting_themes' || zeroToHundred.phase === 'ranking_themes') {
      return `/game/${state.code}/theme-selection`;
    }
    if (zeroToHundred.phase === 'playing' && zeroToHundred.roundNumber) {
      const roundBase = `/game/${state.code}/zero-to-100/${zeroToHundred.roundNumber}`;
      return zeroToHundred.roundPhase === 'guessing' ? `${roundBase}/guess` : `${roundBase}/results`;
    }
    if (zeroToHundred.phase === 'completed') {
      return `/game/${state.code}/zero-to-100/final`;
    }
    return lobbyUrl;
  }
  if (state.mode === 'situation') {
    const situation = state.situationGame;
    if (!situation) {
      return lobbyUrl;
    }
    if (situation.phase === 'collecting_themes' || situation.phase === 'ranking_themes') {
      return `/game/${state.code}/theme-selection`;
    }
    if (situation.phase === 'playing' && situation.roundNumber) {
      const roundBase = `/game/${state.code}/situation/${situation.roundNumber}`;
      switch (situation.roundPhase) {
        case 'proposing':
          return `${roundBase}/propose`;
        case 'dueling':
          return `${roundBase}/duel`;
        case 'ranking':
          return `${roundBase}/ranking`;
        default:
          return `${roundBase}/results`;
      }
    }
    if (situation.phase === 'completed') {
      return `/game/${state.code}/situation/final`;
    }
    return lobbyUrl;
  }
  if (state.mode === 'wavelength') {
    const wavelength = state.wavelengthGame;
    if (!wavelength) {
      return lobbyUrl;
    }
    if (wavelength.phase === 'collecting_themes' || wavelength.phase === 'ranking_themes') {
      return `/game/${state.code}/theme-selection`;
    }
    if (wavelength.phase === 'playing' && wavelength.roundNumber) {
      const roundBase = `/game/${state.code}/wavelength/${wavelength.roundNumber}`;
      return wavelength.roundPhase === 'clue' || wavelength.roundPhase === 'guessing'
        ? `${roundBase}/play`
        : `${roundBase}/results`;
    }
    if (wavelength.phase === 'completed') {
      return `/game/${state.code}/wavelength/final`;
    }
    return lobbyUrl;
  }
  if (state.mode === 'secret_ranking') {
    return secretRankingDestination(state, lobbyUrl);
  }
  return lobbyUrl;
}

/** The phones stay on one table page while they are numbered and while the reveal plays. */
function secretRankingDestination(state: LobbyStateResponse, lobbyUrl: string): string {
  const secretRanking = state.secretRankingGame;
  if (!secretRanking) {
    return lobbyUrl;
  }
  if (secretRanking.phase === 'collecting_themes' || secretRanking.phase === 'ranking_themes') {
    return `/game/${state.code}/theme-selection`;
  }
  if (secretRanking.phase === 'completed') {
    return `/game/${state.code}/secret-ranking/final`;
  }
  if (secretRanking.phase !== 'playing' || !secretRanking.roundNumber) {
    return lobbyUrl;
  }
  const roundBase = `/game/${state.code}/secret-ranking/${secretRanking.roundNumber}`;
  switch (secretRanking.roundPhase) {
    case 'secret':
      return `${roundBase}/secret`;
    case 'placing':
    case 'revealing':
      return `${roundBase}/table`;
    default:
      return `${roundBase}/results`;
  }
}

export function permittedLobbyUrl(state: LobbyStateResponse, requestedUrl: string): string {
  const lobbyUrl = `/lobby/${state.code}`;
  if (isPendingAdmission(state)) {
    return pendingApprovalUrl(state.code);
  }

  if (isUncappedLobbyMode(state.mode)) {
    // These modes have no classic photo-preparation step and exactly one
    // valid destination per phase — unlike the classic branch below, /photos
    // is never a permitted URL here.
    return uncappedModeDestination(state);
  }

  const photosUrl = `${lobbyUrl}/photos`;
  if (!state.game || (state.status !== 'in_game' && state.status !== 'completed')) {
    return requestedUrl === lobbyUrl || requestedUrl === photosUrl ? requestedUrl : lobbyUrl;
  }

  const roundUrl = `/game/${state.code}/round/${state.game.roundNumber}`;
  if (state.game.phase === 'completed') {
    return `/game/${state.code}/final`;
  }
  if (state.game.phase === 'between_rounds') {
    return lobbyUrl;
  }
  if (state.game.phase === 'reveal_and_vote') {
    return `${roundUrl}/reveal`;
  }
  if (state.game.phase === 'round_results') {
    const scoresUrl = `${roundUrl}/scores`;
    return requestedUrl === scoresUrl || requestedUrl.startsWith(`${scoresUrl}/`) ? requestedUrl : scoresUrl;
  }

  const roleUrl = `${roundUrl}/role`;
  const profileUrl = `${roundUrl}/profile`;
  const reviewUrl = `${roundUrl}/review`;
  if (state.game.submitted) {
    return reviewUrl;
  }
  return requestedUrl === roleUrl || requestedUrl === profileUrl || requestedUrl === reviewUrl
    ? requestedUrl
    : roleUrl;
}
