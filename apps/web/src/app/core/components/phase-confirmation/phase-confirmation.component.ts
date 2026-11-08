import { ChangeDetectionStrategy, Component, input } from '@angular/core';
import { IonIcon } from '@ionic/angular/standalone';
import { WaitingGameComponent } from '../waiting-game/waiting-game.component';

@Component({
  selector: 'app-phase-confirmation',
  imports: [IonIcon, WaitingGameComponent],
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
  /** Off when the page renders its own game further down, below extra waiting content. */
  readonly hasWaitingGame = input(true);
}
