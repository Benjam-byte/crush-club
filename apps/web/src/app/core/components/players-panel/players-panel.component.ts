import { ChangeDetectionStrategy, Component, inject } from '@angular/core';
import { AlertController, IonButton, IonContent, IonIcon, IonModal } from '@ionic/angular/standalone';
import type { LobbyPlayer } from '@core/models/game.models';
import { GameStateService } from '../../services/game-state.service';
import { SessionMenuService } from '../../services/session-menu.service';

const confirmRole = 'confirm';

/**
 * Everyone in the lobby, opened from the "…" menu. The host answers join requests and can remove
 * any other player from here; the others only see the list.
 */
@Component({
  selector: 'app-players-panel',
  imports: [IonButton, IonContent, IonIcon, IonModal],
  templateUrl: './players-panel.component.html',
  styleUrl: './players-panel.component.scss',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class PlayersPanelComponent {
  private readonly alertController = inject(AlertController);
  protected readonly gameState = inject(GameStateService);
  protected readonly sessionMenu = inject(SessionMenuService);

  protected async onAdmit(player: LobbyPlayer): Promise<void> {
    await this.ignoreReportedFailure(this.gameState.admitPlayer(player.id));
  }

  protected async onRefuse(player: LobbyPlayer): Promise<void> {
    await this.ignoreReportedFailure(this.gameState.refusePlayer(player.id));
  }

  protected async onKick(player: LobbyPlayer): Promise<void> {
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
    if (role === confirmRole) {
      await this.ignoreReportedFailure(this.gameState.kickPlayer(player.id));
    }
  }

  /** A failed command is already shown by the global error toast. */
  private async ignoreReportedFailure(command: Promise<void>): Promise<void> {
    try {
      await command;
    } catch {
      // Reported through GameStateService.errorMessage.
    }
  }
}
