import { ChangeDetectionStrategy, Component, inject } from '@angular/core';
import { IonIcon } from '@ionic/angular/standalone';
import { GameStateService } from '../../services/game-state.service';

/** Names the players the current phase is still waiting for. Renders nothing when nobody is late. */
@Component({
  selector: 'app-awaiting-players',
  imports: [IonIcon],
  templateUrl: './awaiting-players.component.html',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class AwaitingPlayersComponent {
  protected readonly players = inject(GameStateService).awaitingPlayers;
}
