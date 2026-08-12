import { ChangeDetectionStrategy, Component, computed, inject, signal } from '@angular/core';
import type { OnDestroy, OnInit } from '@angular/core';
import { ActivatedRoute, Router } from '@angular/router';
import { IonContent, IonIcon } from '@ionic/angular/standalone';
import { PageHeaderComponent } from '@core/components/page-header/page-header.component';
import { GameStateService } from '@core/services/game-state.service';

@Component({
  selector: 'app-situation-duel-page',
  imports: [IonContent, IonIcon, PageHeaderComponent],
  templateUrl: './situation-duel.page.html',
  styleUrl: './situation-duel.page.scss',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class SituationDuelPage implements OnInit, OnDestroy {
  protected readonly gameState = inject(GameStateService);
  private readonly router = inject(Router);
  private readonly route = inject(ActivatedRoute);

  protected readonly isVoting = signal(false);
  protected readonly duel = computed(() => this.gameState.situationGame()?.currentDuel ?? null);
  protected readonly remainingSeconds = signal(0);
  protected readonly countdownLabel = computed(() => {
    const totalSeconds = this.remainingSeconds();
    const minuteCount = Math.floor(totalSeconds / 60);
    const secondCount = totalSeconds % 60;
    return `${minuteCount.toString().padStart(2, '0')}:${secondCount.toString().padStart(2, '0')}`;
  });
  protected readonly isCountdownWarning = computed(() => this.remainingSeconds() <= 15);

  private countdownInterval: ReturnType<typeof setInterval> | null = null;

  async ngOnInit(): Promise<void> {
    try {
      await this.gameState.refreshLobby(this.route.snapshot.paramMap.get('code') ?? undefined);
    } catch {
      await this.router.navigate(['/join'], { queryParams: { code: this.route.snapshot.paramMap.get('code') } });
      return;
    }
    this.refreshCountdown();
    this.countdownInterval = setInterval(() => this.refreshCountdown(), 250);
  }

  ngOnDestroy(): void {
    if (this.countdownInterval !== null) {
      clearInterval(this.countdownInterval);
    }
  }

  protected async onVote(proposalId: string): Promise<void> {
    const duel = this.duel();
    if (!duel || this.isVoting()) {
      return;
    }
    this.isVoting.set(true);
    try {
      await this.gameState.voteSituationDuel(duel.id, proposalId);
    } catch {
      // GameState exposes the API error.
    } finally {
      this.isVoting.set(false);
    }
  }

  private refreshCountdown(): void {
    const deadline = this.duel()?.deadline;
    if (!deadline) {
      this.remainingSeconds.set(0);
      return;
    }
    const remainingMs = new Date(deadline).getTime() - Date.now();
    this.remainingSeconds.set(Math.max(0, Math.ceil(remainingMs / 1000)));
  }
}
