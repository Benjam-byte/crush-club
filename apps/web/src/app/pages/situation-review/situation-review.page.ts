import { ChangeDetectionStrategy, Component, computed, inject, signal } from '@angular/core';
import type { OnInit } from '@angular/core';
import { ActivatedRoute, Router } from '@angular/router';
import { IonButton, IonContent, IonIcon } from '@ionic/angular/standalone';
import { PageHeaderComponent } from '@core/components/page-header/page-header.component';
import { GameStateService } from '@core/services/game-state.service';

@Component({
  selector: 'app-situation-review-page',
  imports: [IonButton, IonContent, IonIcon, PageHeaderComponent],
  templateUrl: './situation-review.page.html',
  styleUrl: './situation-review.page.scss',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class SituationReviewPage implements OnInit {
  protected readonly gameState = inject(GameStateService);
  private readonly router = inject(Router);
  private readonly route = inject(ActivatedRoute);

  protected readonly isAdvancing = signal(false);
  protected readonly proposal = computed(() => this.gameState.situationGame()?.currentProposal ?? null);
  protected readonly positionLabel = computed(() => {
    const situation = this.gameState.situationGame();
    if (!situation || !situation.proposalCount) {
      return '';
    }
    return `${(situation.reviewIndex ?? 0) + 1} / ${situation.proposalCount}`;
  });

  async ngOnInit(): Promise<void> {
    try {
      await this.gameState.refreshLobby(this.route.snapshot.paramMap.get('code') ?? undefined);
    } catch {
      await this.router.navigate(['/join'], { queryParams: { code: this.route.snapshot.paramMap.get('code') } });
    }
  }

  protected async onAdvance(direction: 'next' | 'previous'): Promise<void> {
    if (this.isAdvancing() || !this.gameState.situationGame()?.isHostReview) {
      return;
    }
    this.isAdvancing.set(true);
    try {
      await this.gameState.advanceSituationReview(direction);
    } catch {
      // GameState exposes the API error.
    } finally {
      this.isAdvancing.set(false);
    }
  }
}
