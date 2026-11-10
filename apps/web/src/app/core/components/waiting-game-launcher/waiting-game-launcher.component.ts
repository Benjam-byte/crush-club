import { ChangeDetectionStrategy, Component, inject, input, signal } from '@angular/core';
import { IonButton, IonIcon, IonModal } from '@ionic/angular/standalone';
import { GameStateService } from '../../services/game-state.service';
import { WaitingGameComponent } from '../waiting-game/waiting-game.component';

// Shared across instances: once the player opened the mini-game, featured buttons stop wiggling for the session.
const hasOpenedGame = signal(false);

/**
 * Opens the waiting mini-game in a modal; the featured variant is a highlighted button that wiggles until first used.
 * Renders nothing for the host.
 */
@Component({
  selector: 'app-waiting-game-launcher',
  imports: [IonButton, IonIcon, IonModal, WaitingGameComponent],
  templateUrl: './waiting-game-launcher.component.html',
  styleUrl: './waiting-game-launcher.component.scss',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class WaitingGameLauncherComponent {
  readonly featured = input(false);
  protected readonly gameState = inject(GameStateService);
  protected readonly isOpen = signal(false);
  protected readonly hasOpenedGame = hasOpenedGame.asReadonly();

  protected open(): void {
    this.isOpen.set(true);
    hasOpenedGame.set(true);
  }
}
