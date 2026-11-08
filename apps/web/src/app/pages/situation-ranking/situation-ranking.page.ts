import { InfoSheetComponent } from '@core/components/info-sheet/info-sheet.component';
import { ChangeDetectionStrategy, Component, computed, effect, inject, signal } from '@angular/core';
import type { OnInit } from '@angular/core';
import { ActivatedRoute, Router } from '@angular/router';
import { IonButton, IonContent, IonIcon } from '@ionic/angular/standalone';
import type { SituationProposalView } from '@core/models/game.models';
import { PageHeaderComponent } from '@core/components/page-header/page-header.component';
import { PhaseConfirmationComponent } from '@core/components/phase-confirmation/phase-confirmation.component';
import { GameStateService } from '@core/services/game-state.service';
import { placeLabel, pointsForPlace, pointsLabel } from '@core/services/situation-points';
import {
  clearRankingSlot,
  createRankingSlotList,
  findUnplacedIdList,
  isRankingComplete,
  placeInFirstFreeSlot,
} from './ranking-slots';
import type { RankingSlotList } from './ranking-slots';

type PodiumTier = 'gold' | 'silver' | 'bronze' | 'plain';

interface PodiumPlace {
  slotIndex: number
  placeLabel: string
  pointsText: string
  hasTrophy: boolean
  badgeClass: string
  pointsClass: string
  candidate: SituationProposalView | null
}

const podiumTierList: readonly PodiumTier[] = ['gold', 'silver', 'bronze'];

// Icons carry the medal colour; point texts stay ink-900 on a tinted pill for contrast.
const podiumTierStyleMap: Record<PodiumTier, Pick<PodiumPlace, 'badgeClass' | 'pointsClass'>> = {
  gold: { badgeClass: 'bg-gold-50 text-gold-500', pointsClass: 'bg-gold-50' },
  silver: { badgeClass: 'bg-silver-50 text-silver-500', pointsClass: 'bg-silver-50' },
  bronze: { badgeClass: 'bg-bronze-50 text-bronze-500', pointsClass: 'bg-bronze-50' },
  plain: { badgeClass: 'bg-peach-50 text-ink-600', pointsClass: 'bg-peach-50' },
};

@Component({
  selector: 'app-situation-ranking-page',
  imports: [InfoSheetComponent, IonButton, IonContent, IonIcon, PageHeaderComponent, PhaseConfirmationComponent],
  templateUrl: './situation-ranking.page.html',
  styleUrl: './situation-ranking.page.scss',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class SituationRankingPage implements OnInit {
  protected readonly gameState = inject(GameStateService);
  private readonly router = inject(Router);
  private readonly route = inject(ActivatedRoute);

  protected readonly isSubmittingRanking = signal(false);
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

  protected readonly podiumPlaceList = computed<PodiumPlace[]>(() => {
    const finalistCount = this.slotList().length;
    return this.slotList().map((candidateId, slotIndex) => {
      const tier = podiumTierList[slotIndex] ?? 'plain';
      return {
        slotIndex,
        placeLabel: placeLabel(slotIndex + 1),
        pointsText: `+${pointsLabel(pointsForPlace(slotIndex, finalistCount))}`,
        hasTrophy: tier !== 'plain',
        ...podiumTierStyleMap[tier],
        candidate: candidateId === null ? null : this.candidatesById().get(candidateId) ?? null,
      };
    });
  });
  protected readonly unplacedCandidateList = computed(() =>
    findUnplacedIdList(this.slotList(), this.candidateIdList())
      .map((candidateId) => this.candidatesById().get(candidateId))
      .filter((candidate) => candidate !== undefined),
  );
  protected readonly placedCount = computed(() => this.slotList().length - this.unplacedCandidateList().length);
  protected readonly nextFreeSlotIndex = computed(() => this.slotList().indexOf(null));
  /** "En 2e · +3 pts": what tapping an answer would give it right now. */
  protected readonly nextPlaceHint = computed(() => {
    const slotIndex = this.nextFreeSlotIndex();
    if (slotIndex === -1) {
      return '';
    }
    return `En ${placeLabel(slotIndex + 1)} · +${pointsLabel(pointsForPlace(slotIndex, this.slotList().length))}`;
  });
  protected readonly isComplete = computed(() => this.slotList().length > 0 && isRankingComplete(this.slotList()));
  protected readonly submitLabel = computed(() => {
    if (this.isSubmittingRanking()) {
      return 'Envoi…';
    }
    if (this.isComplete()) {
      return 'Valider mon classement';
    }
    const remainingCount = this.unplacedCandidateList().length;
    return remainingCount === 1 ? 'Place encore 1 réponse' : `Place encore ${remainingCount} réponses`;
  });

  private rankingInitializedKey: string | null = null;

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
    }
  }

  protected onPlaceCandidate(candidateId: string): void {
    this.slotList.update((slotList) => placeInFirstFreeSlot(slotList, candidateId, this.candidateIdList()));
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
}
