import { inject } from '@angular/core';
import type { CanDeactivateFn } from '@angular/router';
import { GameStateService } from '@core/services/game-state.service';
import { LobbyExitService } from '@core/services/lobby-exit.service';
import { shouldConfirmLobbyExit } from './lobby-route';

/**
 * Catches every way out of a lobby: header back arrow, browser back, "Quitter" buttons.
 * The navigation is cancelled at once and replayed after confirmation, so a snapshot-driven
 * redirect arriving while the dialog is open cannot swallow the exit.
 */
export const lobbyExitGuard: CanDeactivateFn<unknown> = (_component, _currentRoute, currentState, nextState) => {
  const gameState = inject(GameStateService);
  if (!shouldConfirmLobbyExit(currentState.url, nextState.url, gameState.activeSessionCode())) {
    return true;
  }
  void inject(LobbyExitService).confirmExit(nextState.url);
  return false;
};
