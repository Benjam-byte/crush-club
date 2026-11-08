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
    const alert = await this.alertController.create({ ...this.confirmationCopy(), buttons: [
      { text: 'Rester', role: 'cancel' },
      { text: this.gameState.isPendingApproval() ? 'Annuler ma demande' : 'Quitter', role: confirmRole },
    ] });
    await alert.present();
    const { role } = await alert.onDidDismiss();
    return role === confirmRole;
  }

  private confirmationCopy(): { header: string, message: string } {
    if (this.gameState.isPendingApproval()) {
      return { header: 'Annuler ta demande ?', message: 'Tu ne rejoindras pas la partie en cours.' };
    }
    const hostNote = this.gameState.isHost() ? ' Ton rôle d’hôte passera à un autre joueur.' : '';
    if (this.gameState.lobbyStatus() === 'in_game') {
      return {
        header: 'Quitter la partie ?',
        message: `La partie continuera sans toi : plus personne ne t’attendra.${hostNote}`,
      };
    }
    return { header: 'Quitter le lobby ?', message: `Tu perdras ta place dans ce lobby.${hostNote}` };
  }
}
