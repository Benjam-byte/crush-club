import { effect, inject, Injectable, signal } from '@angular/core';
import { Router } from '@angular/router';
import { ActionSheetController, AlertController } from '@ionic/angular/standalone';
import type { LobbyPlayer } from '../models/game.models';
import { GameStateService } from './game-state.service';

const confirmRole = 'confirm';

/** The "…" menu of a lobby or game page, the players panel it opens, and the host's kick confirmation. */
@Injectable({
  providedIn: 'root',
})
export class SessionMenuService {
  private readonly actionSheetController = inject(ActionSheetController);
  private readonly alertController = inject(AlertController);
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

  /** Asks the host to confirm, then removes the player: from the lobby's player cards and from the players panel. */
  async confirmKick(player: LobbyPlayer): Promise<void> {
    const alert = await this.alertController.create({
      header: `Exclure ${player.displayName} ?`,
      message: this.gameState.lobbyStatus() === 'in_game'
        ? 'La partie continuera sans ce joueur. Pour revenir, il devra demander à rejoindre et attendre ton accord.'
        : 'Ce joueur perdra sa place dans le lobby.',
      buttons: [
        { text: 'Annuler', role: 'cancel' },
        { text: 'Exclure', role: confirmRole },
      ],
    });
    await alert.present();
    const { role } = await alert.onDidDismiss();
    if (role !== confirmRole) {
      return;
    }
    try {
      await this.gameState.kickPlayer(player.id);
    } catch {
      // Reported through GameStateService.errorMessage.
    }
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
