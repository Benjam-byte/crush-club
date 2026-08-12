import { ChangeDetectionStrategy, Component, inject } from '@angular/core';
import { IonApp, IonRouterOutlet, IonToast } from '@ionic/angular/standalone';
import { AppUpdateService } from '@core/services/app-update.service';
import { GameStateService } from '@core/services/game-state.service';
import { HostDisconnectedPlayersComponent } from './core/components/host-disconnected-players/host-disconnected-players.component';

@Component({
  selector: 'app-root',
  imports: [HostDisconnectedPlayersComponent, IonApp, IonRouterOutlet, IonToast],
  templateUrl: './app.component.html',
  styleUrl: './app.component.scss',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class AppComponent {
  protected readonly appUpdate = inject(AppUpdateService);
  protected readonly gameState = inject(GameStateService);

  protected readonly updateToastButtons = [
    {
      text: 'Recharger',
      handler: () => this.appUpdate.reload(),
    },
    {
      text: 'Plus tard',
      role: 'cancel',
    },
  ];
}
