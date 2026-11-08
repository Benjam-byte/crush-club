import { effect, inject, Injectable, signal } from '@angular/core';
import { Router } from '@angular/router';
import { ActionSheetController } from '@ionic/angular/standalone';
import { GameStateService } from './game-state.service';

/** The "…" menu of a lobby or game page, and the players panel it opens. */
@Injectable({
  providedIn: 'root',
})
export class SessionMenuService {
  private readonly actionSheetController = inject(ActionSheetController);
  private readonly gameState = inject(GameStateService);
  private readonly router = inject(Router);
  private readonly playersPanelOpenState = signal(false);
  private isMenuOpen = false;

  readonly isPlayersPanelOpen = this.playersPanelOpenState.asReadonly();

  constructor() {
    // The panel belongs to the lobby session: it closes once the player left or was removed.
    effect(() => {
      if (this.gameState.activeSessionCode() === null) {
        this.playersPanelOpenState.set(false);
      }
    });
  }

  async openMenu(): Promise<void> {
    // A double tap must not stack a second sheet.
    if (this.isMenuOpen) {
      return;
    }
    this.isMenuOpen = true;
    try {
      const actionSheet = await this.actionSheetController.create({
        header: `Partie ${this.gameState.lobbyCode()}`,
        buttons: [
          { text: this.playersButtonLabel(), icon: 'people-outline', handler: () => this.openPlayersPanel() },
          {
            text: this.leaveButtonLabel(),
            role: 'destructive',
            icon: 'exit-outline',
            // Going home hands over to the lobby exit guard, which confirms and really leaves.
            handler: () => void this.router.navigateByUrl('/'),
          },
          { text: 'Annuler', role: 'cancel' },
        ],
      });
      await actionSheet.present();
      await actionSheet.onDidDismiss();
    } finally {
      this.isMenuOpen = false;
    }
  }

  openPlayersPanel(): void {
    this.playersPanelOpenState.set(true);
  }

  closePlayersPanel(): void {
    this.playersPanelOpenState.set(false);
  }

  private playersButtonLabel(): string {
    const label = `Joueurs (${this.gameState.playerList().length})`;
    const pendingCount = this.gameState.pendingRequestCount();
    return pendingCount > 0 ? `${label} · ${pendingCount} en attente` : label;
  }

  private leaveButtonLabel(): string {
    if (this.gameState.isPendingApproval()) {
      return 'Annuler ma demande';
    }
    return this.gameState.lobbyStatus() === 'in_game' ? 'Quitter la partie' : 'Quitter le lobby';
  }
}
