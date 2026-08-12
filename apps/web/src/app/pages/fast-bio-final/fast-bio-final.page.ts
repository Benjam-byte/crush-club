import { ChangeDetectionStrategy, Component, computed, inject, signal } from '@angular/core';
import type { OnInit } from '@angular/core';
import { ActivatedRoute, Router } from '@angular/router';
import { IonButton, IonContent, IonIcon } from '@ionic/angular/standalone';
import { PageHeaderComponent } from '@core/components/page-header/page-header.component';
import { GameStateService } from '@core/services/game-state.service';

@Component({
  selector: 'app-fast-bio-final-page',
  imports: [IonButton, IonContent, IonIcon, PageHeaderComponent],
  templateUrl: './fast-bio-final.page.html',
  styleUrl: './fast-bio-final.page.scss',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class FastBioFinalPage implements OnInit {
  protected readonly gameState = inject(GameStateService);
  private readonly route = inject(ActivatedRoute);
  private readonly router = inject(Router);

  protected readonly leaderboard = computed(() => this.gameState.fastBioGame()?.leaderboard ?? []);
  protected readonly winner = computed(() => this.leaderboard()[0]);
  protected readonly isReplaying = signal(false);
  protected readonly isLeaving = signal(false);

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

  protected async onLeave(): Promise<void> {
    if (this.isLeaving()) {
      return;
    }
    this.isLeaving.set(true);
    try {
      await this.gameState.leaveGame();
      await this.router.navigate(['/']);
    } finally {
      this.isLeaving.set(false);
    }
  }

  protected initials(displayName: string): string {
    return displayName.trim().split(/\s+/).slice(0, 2).map((part) => part[0]?.toUpperCase() ?? '').join('');
  }
}
