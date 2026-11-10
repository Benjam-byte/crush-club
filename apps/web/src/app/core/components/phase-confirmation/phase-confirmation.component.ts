import { ChangeDetectionStrategy, Component, inject, input } from '@angular/core';
import { IonIcon } from '@ionic/angular/standalone';
import { GameStateService } from '../../services/game-state.service';
import { AwaitingPlayersComponent } from '../awaiting-players/awaiting-players.component';
import { PlayerProgressComponent } from '../player-progress/player-progress.component';
import { WaitingGameLauncherComponent } from '../waiting-game-launcher/waiting-game-launcher.component';
import { WaitingGameComponent } from '../waiting-game/waiting-game.component';

@Component({
  selector: 'app-phase-confirmation',
  imports: [AwaitingPlayersComponent, IonIcon, PlayerProgressComponent, WaitingGameComponent, WaitingGameLauncherComponent],
  templateUrl: './phase-confirmation.component.html',
  styleUrl: './phase-confirmation.component.scss',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class PhaseConfirmationComponent {
  readonly title = input.required<string>();
  readonly message = input('Le prochain écran s’ouvrira automatiquement.');
  readonly progressCount = input<number>();
  readonly progressRequired = input<number>();
  readonly progressLabel = input('joueurs ont terminé');
  /** Off when the page renders its own game further down, below extra waiting content. The host never gets it. */
  readonly hasWaitingGame = input(true);
  protected readonly gameState = inject(GameStateService);
}
