import { FinalLeaderboardComponent } from '@core/components/final-leaderboard/final-leaderboard.component';
import { ChangeDetectionStrategy, Component, computed, inject, signal } from '@angular/core';
import type { OnInit } from '@angular/core';
import { ActivatedRoute, Router } from '@angular/router';
import { IonButton, IonContent, } from '@ionic/angular/standalone';
import { PageHeaderComponent } from '@core/components/page-header/page-header.component';
import { WaitingCardComponent } from '@core/components/waiting-card/waiting-card.component';
import { GameStateService } from '@core/services/game-state.service';
import { SessionMenuService } from '@core/services/session-menu.service';

@Component({
  selector: 'app-fast-bio-final-page',
  imports: [FinalLeaderboardComponent, IonButton, IonContent, PageHeaderComponent, WaitingCardComponent],
  templateUrl: './fast-bio-final.page.html',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class FastBioFinalPage implements OnInit {
  protected readonly gameState = inject(GameStateService);
  protected readonly sessionMenu = inject(SessionMenuService);
  private readonly route = inject(ActivatedRoute);
  private readonly router = inject(Router);

  protected readonly leaderboard = computed(() => this.gameState.fastBioGame()?.leaderboard ?? []);
  protected readonly finalEntries = computed(() => this.leaderboard().map(entry => ({ id: entry.playerId, name: entry.displayName, score: entry.score })));
  protected readonly isReplaying = signal(false);

  async ngOnInit(): Promise<void> {
    try {
      await this.gameState.refreshLobby(this.route.snapshot.paramMap.get('code') ?? undefined);
    } catch {
      await this.router.navigate(['/join'], { queryParams: { code: this.route.snapshot.paramMap.get('code') } });
    }
  }

  protected async onReplay(): Promise<void> {
    if (this.isReplaying() || !this.gameState.isHost()) {
      return;
    }
    this.isReplaying.set(true);
    try {
      await this.gameState.replayFastBio();
    } catch {
      // GameState exposes the API error.
    } finally {
      this.isReplaying.set(false);
    }
  }

  protected initials(displayName: string): string {
    return displayName.trim().split(/\s+/).slice(0, 2).map((part) => part[0]?.toUpperCase() ?? '').join('');
  }
}
