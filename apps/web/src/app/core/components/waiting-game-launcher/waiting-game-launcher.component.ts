import { ChangeDetectionStrategy, Component, signal } from '@angular/core';
import { IonButton, IonModal } from '@ionic/angular/standalone';
import { WaitingGameComponent } from '../waiting-game/waiting-game.component';

@Component({
  selector: 'app-waiting-game-launcher',
  imports: [IonButton, IonModal, WaitingGameComponent],
  templateUrl: './waiting-game-launcher.component.html',
  styleUrl: './waiting-game-launcher.component.scss',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class WaitingGameLauncherComponent {
  protected readonly isOpen = signal(false);
}
