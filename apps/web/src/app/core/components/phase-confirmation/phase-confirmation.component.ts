import { ChangeDetectionStrategy, Component, input } from '@angular/core';
import { IonIcon } from '@ionic/angular/standalone';

@Component({
  selector: 'app-phase-confirmation',
  imports: [IonIcon],
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
}
