import type { LobbyStateResponse } from '@core/models/game.models';

export function shouldApplyLobbySnapshot(
  current: LobbyStateResponse | null,
  next: LobbyStateResponse,
): boolean {
  return current === null || next.code !== current.code || next.revision > current.revision;
}

export function activeNavigationUrl(currentUrl: string, pendingUrl?: string): string {
  return (pendingUrl ?? currentUrl).split('?')[0];
}

export function permittedLobbyUrl(state: LobbyStateResponse, requestedUrl: string): string {
  const lobbyUrl = `/lobby/${state.code}`;
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
