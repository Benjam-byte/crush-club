import { ChangeDetectionStrategy, Component, computed, inject, input } from '@angular/core';
import { ActivatedRoute, RouterLink } from '@angular/router';
import { IonIcon } from '@ionic/angular/standalone';
import { GameStateService } from '../../services/game-state.service';
import { SessionMenuService } from '../../services/session-menu.service';
import { BrandMarkComponent } from '../brand-mark/brand-mark.component';

@Component({
  selector: 'app-page-header',
  imports: [BrandMarkComponent, IonIcon, RouterLink],
  templateUrl: './page-header.component.html',
  styleUrl: './page-header.component.scss',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class PageHeaderComponent {
  private readonly gameState = inject(GameStateService);
  private readonly sessionMenu = inject(SessionMenuService);
  private readonly routeLobbyCode = inject(ActivatedRoute).snapshot.paramMap.get('code')?.toUpperCase() ?? null;

  readonly title = input.required<string>();
  readonly backLink = input<string>();
  readonly centerTitle = input(false);
  readonly wrapTitle = input(false);
  readonly showDecoration = input(true);

  /** Lobby and game pages of the player's own session get the "…" menu instead of the decoration. */
  protected readonly showSessionMenu = computed(() => {
    return this.routeLobbyCode !== null && this.routeLobbyCode === this.gameState.activeSessionCode();
  });
  protected readonly pendingRequestCount = this.gameState.pendingRequestCount;

  protected openSessionMenu(): void {
    void this.sessionMenu.openMenu();
  }
}
