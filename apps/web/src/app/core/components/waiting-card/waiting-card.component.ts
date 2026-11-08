import { ChangeDetectionStrategy, Component, input } from '@angular/core';
import { IonIcon } from '@ionic/angular/standalone';
import { WaitingGameComponent } from '../waiting-game/waiting-game.component';

/** "Waiting for someone else" card, with a mini-game to pass the time. */
@Component({
  selector: 'app-waiting-card',
  imports: [IonIcon, WaitingGameComponent],
  templateUrl: './waiting-card.component.html',
  styleUrl: './waiting-card.component.scss',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class WaitingCardComponent {
  readonly title = input.required<string>();
  readonly message = input<string>();
}
