import { ChangeDetectionStrategy, Component, computed, inject } from '@angular/core';
import { IonIcon } from '@ionic/angular/standalone';
import { GameStateService } from '../../services/game-state.service';

interface PlayerProgressChip {
  id: string;
  name: string;
  isDone: boolean;
  icon: string;
  colorClass: string;
  label: string;
}

/** Where everyone stands in the current phase, as the lobby's player cards: still answering and offline first, then done. */
@Component({
  selector: 'app-player-progress',
  imports: [IonIcon],
  templateUrl: './player-progress.component.html',
  styleUrl: './player-progress.component.scss',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class PlayerProgressComponent {
  private readonly gameState = inject(GameStateService);

  protected readonly chips = computed<PlayerProgressChip[]>(() => {
    const awaitingIdSet = new Set(this.gameState.awaitingPlayers().map((player) => player.id));
    const chips = this.gameState.playerList().map((player): PlayerProgressChip => {
      const base = { id: player.id, name: player.displayName };
      if (!awaitingIdSet.has(player.id)) {
        return { ...base, isDone: true, icon: 'checkmark-circle', colorClass: 'text-success-500', label: 'A terminé' };
      }
      if (!player.connected) {
        return { ...base, isDone: false, icon: 'cloud-offline-outline', colorClass: 'text-warning-500', label: 'Hors ligne' };
      }
      return { ...base, isDone: false, icon: 'hourglass-outline', colorClass: 'text-warning-500', label: 'En cours' };
    });
    return [...chips.filter((chip) => !chip.isDone), ...chips.filter((chip) => chip.isDone)];
  });
}
