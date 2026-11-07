import { inject, Injectable } from '@angular/core';
import { Router } from '@angular/router';
import { AlertController } from '@ionic/angular/standalone';
import { GameStateService } from './game-state.service';

const confirmRole = 'confirm';

@Injectable({
  providedIn: 'root',
})
export class LobbyExitService {
  private readonly alertController = inject(AlertController);
  private readonly gameState = inject(GameStateService);
  private readonly router = inject(Router);
  private isConfirming = false;

  /** Asks before leaving; once confirmed, the player really leaves the lobby, then reaches the destination. */
  async confirmExit(destinationUrl: string): Promise<void> {
    // A double tap or a second back gesture must not stack another dialog.
    if (this.isConfirming) {
      return;
    }
    this.isConfirming = true;
    try {
      const isConfirmed = await this.askConfirmation();
      if (!isConfirmed) {
        return;
      }
      await this.gameState.leaveGame();
      await this.router.navigateByUrl(destinationUrl);
    } finally {
      this.isConfirming = false;
    }
  }

  private async askConfirmation(): Promise<boolean> {
    const alert = await this.alertController.create({
      header: 'Quitter le lobby ?',
      message: this.gameState.isHost()
        ? 'Tu perdras ta place dans ce lobby et ton rôle d’hôte passera à un autre joueur.'
        : 'Tu perdras ta place dans ce lobby.',
      buttons: [
        { text: 'Rester', role: 'cancel' },
        { text: 'Quitter', role: confirmRole },
      ],
    });
    await alert.present();
    const { role } = await alert.onDidDismiss();
    return role === confirmRole;
  }
}
