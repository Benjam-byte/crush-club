import { ChangeDetectionStrategy, Component, inject } from '@angular/core';
import { IonButton, IonContent, IonIcon, IonModal } from '@ionic/angular/standalone';
import type { LobbyPlayer } from '@core/models/game.models';
import { GameStateService } from '../../services/game-state.service';
import { SessionMenuService } from '../../services/session-menu.service';

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
  protected readonly gameState = inject(GameStateService);
  protected readonly sessionMenu = inject(SessionMenuService);

  protected async onAdmit(player: LobbyPlayer): Promise<void> {
    await this.ignoreReportedFailure(this.gameState.admitPlayer(player.id));
  }

  protected async onRefuse(player: LobbyPlayer): Promise<void> {
    await this.ignoreReportedFailure(this.gameState.refusePlayer(player.id));
  }

  protected async onKick(player: LobbyPlayer): Promise<void> {
    await this.sessionMenu.confirmKick(player);
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
