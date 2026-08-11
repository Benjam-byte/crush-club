import { ChangeDetectionStrategy, Component } from '@angular/core';
import { IonApp, IonRouterOutlet } from '@ionic/angular/standalone';
import { HostDisconnectedPlayersComponent } from './core/components/host-disconnected-players/host-disconnected-players.component';

@Component({
  selector: 'app-root',
  imports: [HostDisconnectedPlayersComponent, IonApp, IonRouterOutlet],
  templateUrl: './app.component.html',
  styleUrl: './app.component.scss',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class AppComponent {}
