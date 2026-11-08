import { ChangeDetectionStrategy, Component, inject } from '@angular/core';
import { toSignal } from '@angular/core/rxjs-interop';
import { NavigationEnd, Router } from '@angular/router';
import { IonApp, IonRouterOutlet, IonToast } from '@ionic/angular/standalone';
import { filter, map } from 'rxjs';
import { findLobbyCodeInUrl } from '@core/guards/lobby-route';
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
  private readonly router = inject(Router);

  // Ionic plays the swipe-back animation before route guards run, so a refused lobby exit
  // would leave the screen half-transitioned: the gesture is off inside a lobby.
  protected readonly isSwipeBackEnabled = toSignal(
    this.router.events.pipe(
      filter((event) => event instanceof NavigationEnd),
      map((event) => findLobbyCodeInUrl(event.urlAfterRedirects) === null),
    ),
    { initialValue: true },
  );

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
