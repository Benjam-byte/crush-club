import { inject } from '@angular/core';
import { Router, type CanActivateFn } from '@angular/router';
import { GameStateService } from '@core/services/game-state.service';
import { permittedLobbyUrl } from './lobby-route';

export const lobbySessionGuard: CanActivateFn = async (_route, routerState) => {
  const gameState = inject(GameStateService);
  const router = inject(Router);
  const match = /^\/(?:lobby|game)\/([^/?]+)/.exec(routerState.url);
  const code = match?.[1]?.toUpperCase() ?? '';
  try {
    await gameState.refreshLobby(code);
  } catch {
    return router.createUrlTree(['/join'], { queryParams: code ? { code } : undefined });
  }
  const state = gameState.state();
  if (!state) {
    return router.createUrlTree(['/join'], { queryParams: code ? { code } : undefined });
  }
  const requestedUrl = routerState.url.split('?')[0];
  const permittedUrl = permittedLobbyUrl(state, requestedUrl);
  return requestedUrl === permittedUrl ? true : router.parseUrl(permittedUrl);
};
