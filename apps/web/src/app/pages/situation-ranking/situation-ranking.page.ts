import { ChangeDetectionStrategy, Component, computed, effect, inject, signal } from '@angular/core';
import type { OnInit } from '@angular/core';
import { ActivatedRoute, Router } from '@angular/router';
import { IonButton, IonContent, IonIcon } from '@ionic/angular/standalone';
import type { SituationProposalView } from '@core/models/game.models';
import { PageHeaderComponent } from '@core/components/page-header/page-header.component';
import { GameStateService } from '@core/services/game-state.service';

@Component({
  selector: 'app-situation-ranking-page',
  imports: [IonButton, IonContent, IonIcon, PageHeaderComponent],
  templateUrl: './situation-ranking.page.html',
  styleUrl: './situation-ranking.page.scss',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class SituationRankingPage implements OnInit {
  protected readonly gameState = inject(GameStateService);
  private readonly router = inject(Router);
  private readonly route = inject(ActivatedRoute);

  protected readonly isSubmittingRanking = signal(false);
  protected readonly ranking = signal<readonly string[]>([]);
  protected readonly candidatesById = computed(() => {
    const map = new Map<string, SituationProposalView>();
    for (const candidate of this.gameState.situationGame()?.rankingCandidates ?? []) {
      map.set(candidate.id, candidate);
    }
    return map;
  });

  private rankingInitializedKey: string | null = null;

  constructor() {
    effect(() => {
      const candidates = this.gameState.situationGame()?.rankingCandidates;
      if (!candidates) {
        return;
      }
      const ids = candidates.map((candidate) => candidate.id);
      const idsKey = ids.join('|');
      if (idsKey === this.rankingInitializedKey) {
        return;
      }
      this.rankingInitializedKey = idsKey;
      this.ranking.set(ids);
    });
  }

  async ngOnInit(): Promise<void> {
    try {
      await this.gameState.refreshLobby(this.route.snapshot.paramMap.get('code') ?? undefined);
    } catch {
      await this.router.navigate(['/join'], { queryParams: { code: this.route.snapshot.paramMap.get('code') } });
    }
  }

  protected candidateFor(proposalId: string): SituationProposalView | undefined {
    return this.candidatesById().get(proposalId);
  }

  protected moveUp(index: number): void {
    if (index <= 0) {
      return;
    }
    this.ranking.update((current) => {
      const next = [...current];
      [next[index - 1], next[index]] = [next[index], next[index - 1]];
      return next;
    });
  }

  protected moveDown(index: number): void {
    this.ranking.update((current) => {
      if (index >= current.length - 1) {
        return current;
      }
      const next = [...current];
      [next[index], next[index + 1]] = [next[index + 1], next[index]];
      return next;
    });
  }

  protected async onSubmitRanking(): Promise<void> {
    if (this.isSubmittingRanking() || this.ranking().length === 0) {
      return;
    }
    this.isSubmittingRanking.set(true);
    try {
      await this.gameState.submitSituationRanking(this.ranking());
    } catch {
      // GameState exposes the API error.
    } finally {
      this.isSubmittingRanking.set(false);
    }
  }
}
