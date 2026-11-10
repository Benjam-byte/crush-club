import { InfoSheetComponent } from '@core/components/info-sheet/info-sheet.component';
import { ChangeDetectionStrategy, Component, computed, effect, inject, signal } from '@angular/core';
import type { OnDestroy, OnInit } from '@angular/core';
import { ActivatedRoute, Router } from '@angular/router';
import { IonButton, IonContent, IonIcon } from '@ionic/angular/standalone';
import type { SituationProposalView } from '@core/models/game.models';
import { PageHeaderComponent } from '@core/components/page-header/page-header.component';
import { PhaseConfirmationComponent } from '@core/components/phase-confirmation/phase-confirmation.component';
import { PhaseTimerComponent } from '@core/components/phase-timer/phase-timer.component';
import { GameStateService } from '@core/services/game-state.service';
import { placeLabel, pointsForPlace, pointsLabel } from '@core/services/situation-points';
import { clearRankingSlot, createRankingSlotList, isRankingComplete, placeInFirstFreeSlot } from './ranking-slots';
import type { RankingSlotList } from './ranking-slots';

type PodiumTier = 'gold' | 'silver' | 'bronze' | 'plain';

interface PodiumStep {
  slotIndex: number
  placeLabel: string
  pointsText: string
  tier: PodiumTier
  heightPx: number
  candidate: SituationProposalView | null
}

interface CandidateCard {
  candidate: SituationProposalView
  slotIndex: number | null
  tier: PodiumTier | null
  placeText: string
}

const podiumTierList: readonly PodiumTier[] = ['gold', 'silver', 'bronze'];
// Step heights by place: the winner's step stands tallest; the last one still fits its points.
const podiumStepHeightList = [88, 70, 56, 46];
// Classic podium order, second on the left of the winner: 2, 1, 3, then 4.
const podiumDisplayOrder = [1, 0, 2, 3];

@Component({
  selector: 'app-situation-ranking-page',
  imports: [
    InfoSheetComponent,
    IonButton,
    IonContent,
    IonIcon,
    PageHeaderComponent,
    PhaseConfirmationComponent,
    PhaseTimerComponent,
  ],
  templateUrl: './situation-ranking.page.html',
  styleUrl: './situation-ranking.page.scss',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class SituationRankingPage implements OnInit, OnDestroy {
  protected readonly gameState = inject(GameStateService);
  private readonly router = inject(Router);
  private readonly route = inject(ActivatedRoute);

  protected readonly isSubmittingRanking = signal(false);
  protected readonly remainingSeconds = signal(0);
  protected readonly slotList = signal<RankingSlotList>([]);
  private readonly candidateList = computed(() => this.gameState.situationGame()?.rankingCandidates ?? []);
  private readonly candidateIdList = computed(() => this.candidateList().map((candidate) => candidate.id));
  private readonly candidatesById = computed(() => {
    const map = new Map<string, SituationProposalView>();
    for (const candidate of this.candidateList()) {
      map.set(candidate.id, candidate);
    }
    return map;
  });

  /** The podium's steps in display order (2, 1, 3, 4), each with its colour, points and the answer placed on it. */
  protected readonly podiumStepList = computed<PodiumStep[]>(() => {
    const slotList = this.slotList();
    return podiumDisplayOrder
      .filter((slotIndex) => slotIndex < slotList.length)
      .map((slotIndex) => {
        const candidateId = slotList[slotIndex];
        return {
          slotIndex,
          placeLabel: placeLabel(slotIndex + 1),
          pointsText: `+${pointsLabel(pointsForPlace(slotIndex, slotList.length))}`,
          tier: podiumTierList[slotIndex] ?? 'plain',
          heightPx: podiumStepHeightList[slotIndex] ?? 46,
          candidate: candidateId === null ? null : this.candidatesById().get(candidateId) ?? null,
        };
      });
  });
  /** Every answer, placed or not, so each reason stays readable; a tap places it or takes it back. */
  protected readonly candidateCardList = computed<CandidateCard[]>(() => {
    const slotList = this.slotList();
    return this.candidateList().map((candidate) => {
      const slotIndex = slotList.indexOf(candidate.id);
      if (slotIndex === -1) {
        return { candidate, slotIndex: null, tier: null, placeText: '' };
      }
      return {
        candidate,
        slotIndex,
        tier: podiumTierList[slotIndex] ?? 'plain',
        placeText: `${placeLabel(slotIndex + 1)} · +${pointsLabel(pointsForPlace(slotIndex, slotList.length))}`,
      };
    });
  });
  protected readonly nextFreeSlotIndex = computed(() => this.slotList().indexOf(null));
  protected readonly isComplete = computed(() => this.slotList().length > 0 && isRankingComplete(this.slotList()));
  protected readonly submitLabel = computed(() => {
    if (this.isSubmittingRanking()) {
      return 'Envoi…';
    }
    if (this.isComplete()) {
      return 'Valider mon classement';
    }
    const remainingCount = this.slotList().filter((slot) => slot === null).length;
    return remainingCount === 1 ? 'Place encore 1 réponse' : `Place encore ${remainingCount} réponses`;
  });

  private rankingInitializedKey: string | null = null;
  private countdownInterval: ReturnType<typeof setInterval> | null = null;

  constructor() {
    effect(() => {
      const candidateIdList = this.candidateIdList();
      const idsKey = candidateIdList.join('|');
      if (candidateIdList.length === 0 || idsKey === this.rankingInitializedKey) {
        return;
      }
      this.rankingInitializedKey = idsKey;
      this.slotList.set(createRankingSlotList(candidateIdList));
    });
  }

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

  protected onToggleCandidate(card: CandidateCard): void {
    if (card.slotIndex === null) {
      this.slotList.update((slotList) => placeInFirstFreeSlot(slotList, card.candidate.id, this.candidateIdList()));
      return;
    }
    this.onClearSlot(card.slotIndex);
  }

  protected onClearSlot(slotIndex: number): void {
    this.slotList.update((slotList) => clearRankingSlot(slotList, slotIndex));
  }

  protected async onSubmitRanking(): Promise<void> {
    const slotList = this.slotList();
    if (this.isSubmittingRanking() || slotList.length === 0 || !isRankingComplete(slotList)) {
      return;
    }
    this.isSubmittingRanking.set(true);
    try {
      await this.gameState.submitSituationRanking(slotList);
    } catch {
      // GameState exposes the API error.
    } finally {
      this.isSubmittingRanking.set(false);
    }
  }

  protected initials(displayName: string): string {
    return displayName.trim().split(/\s+/).slice(0, 2).map((part) => part[0]?.toUpperCase() ?? '').join('');
  }

  private refreshCountdown(): void {
    const deadline = this.gameState.situationGame()?.rankingDeadline;
    if (!deadline) {
      this.remainingSeconds.set(0);
      return;
    }
    const remainingMs = new Date(deadline).getTime() - Date.now();
    this.remainingSeconds.set(Math.max(0, Math.ceil(remainingMs / 1000)));
  }
}
