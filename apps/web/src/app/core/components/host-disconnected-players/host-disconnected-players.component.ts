import { ChangeDetectionStrategy, Component, computed, inject, signal } from '@angular/core';
import { IonIcon } from '@ionic/angular/standalone';
import { GameStateService } from '../../services/game-state.service';
import { activeGameExclusionCandidates } from '../../services/player-exclusion';

@Component({
  selector: 'app-host-disconnected-players',
  imports: [IonIcon],
  templateUrl: './host-disconnected-players.component.html',
  styleUrl: './host-disconnected-players.component.scss',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class HostDisconnectedPlayersComponent {
  protected readonly gameState = inject(GameStateService);
  protected readonly exclusionError = signal<string | null>(null);
  protected readonly offlinePlayers = computed(() => activeGameExclusionCandidates(
    this.gameState.isHost(),
    this.gameState.lobbyStatus(),
    this.gameState.game()?.phase,
    this.gameState.playerList(),
  ));
  protected readonly isActiveGamePhase = computed(() => {
    const phase = this.gameState.game()?.phase;
    return this.gameState.isHost() && this.gameState.lobbyStatus() === 'in_game' &&
      phase !== undefined && phase !== 'between_rounds' && phase !== 'completed';
  });

  protected async onExcludePlayer(playerId: string): Promise<void> {
    this.exclusionError.set(null);
    try {
      await this.gameState.excludePlayer(playerId);
    } catch {
      this.exclusionError.set(
        this.gameState.errorMessage() ?? 'Impossible d’exclure ce joueur. Vérifie ta connexion puis réessaie.',
      );
    }
  }

  protected dismissError(): void {
    this.exclusionError.set(null);
  }
}
