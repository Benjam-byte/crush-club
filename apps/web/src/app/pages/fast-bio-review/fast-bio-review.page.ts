import { ChangeDetectionStrategy, Component, computed, effect, inject, signal, viewChild } from '@angular/core';
import type { OnInit } from '@angular/core';
import { ActivatedRoute, Router } from '@angular/router';
import { IonButton, IonContent, IonIcon } from '@ionic/angular/standalone';
import { fastBioReactionEmojis, fastBioReactionPoints } from '@core/models/game.models';
import { PageHeaderComponent } from '@core/components/page-header/page-header.component';
import { ReactionBurstComponent } from '@core/components/reaction-burst/reaction-burst.component';
import { GameStateService } from '@core/services/game-state.service';
import { RealtimeLobbyService } from '@core/services/realtime-lobby.service';

@Component({
  selector: 'app-fast-bio-review-page',
  imports: [IonButton, IonContent, IonIcon, PageHeaderComponent, ReactionBurstComponent],
  templateUrl: './fast-bio-review.page.html',
  styleUrl: './fast-bio-review.page.scss',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class FastBioReviewPage implements OnInit {
  protected readonly gameState = inject(GameStateService);
  private readonly realtime = inject(RealtimeLobbyService);
  private readonly router = inject(Router);
  private readonly route = inject(ActivatedRoute);
  private readonly burst = viewChild(ReactionBurstComponent);

  protected readonly isAdvancing = signal(false);
  protected readonly isReacting = signal(false);
  protected readonly emojiList = fastBioReactionEmojis;
  protected readonly reactionPoints = fastBioReactionPoints;
  protected readonly proposal = computed(() => this.gameState.fastBioGame()?.currentProposal ?? null);
  protected readonly canReact = computed(() => {
    const proposal = this.proposal();
    return proposal !== null && proposal.authorPlayerId !== this.gameState.currentPlayer().id;
  });
  protected readonly positionLabel = computed(() => {
    const fastBio = this.gameState.fastBioGame();
    if (!fastBio || !fastBio.proposalCount) {
      return '';
    }
    return `${(fastBio.reviewIndex ?? 0) + 1} / ${fastBio.proposalCount}`;
  });
  protected readonly allReactionsSubmitted = computed(() => {
    const fastBio = this.gameState.fastBioGame();
    if (!fastBio || fastBio.reactionProgressRequired === undefined) {
      return false;
    }
    return (fastBio.reactionProgressCount ?? 0) >= fastBio.reactionProgressRequired;
  });

  constructor() {
    effect(() => {
      const events = this.realtime.reactionEvents();
      if (events.length === 0) {
        return;
      }
      const burst = this.burst();
      for (const event of events) {
        burst?.push(event.emoji);
      }
      this.realtime.clearReactionEvents();
    });
  }

  async ngOnInit(): Promise<void> {
    try {
      await this.gameState.refreshLobby(this.route.snapshot.paramMap.get('code') ?? undefined);
    } catch {
      await this.router.navigate(['/join'], { queryParams: { code: this.route.snapshot.paramMap.get('code') } });
    }
  }

  protected async onAdvance(direction: 'next' | 'previous'): Promise<void> {
    if (
      this.isAdvancing() ||
      !this.gameState.fastBioGame()?.isHostReview ||
      (direction === 'next' && !this.allReactionsSubmitted())
    ) {
      return;
    }
    this.isAdvancing.set(true);
    try {
      await this.gameState.advanceFastBioReview(direction);
    } catch {
      // GameState exposes the API error.
    } finally {
      this.isAdvancing.set(false);
    }
  }

  protected async onReact(emoji: string): Promise<void> {
    const proposal = this.proposal();
    if (!proposal || this.isReacting() || !this.canReact()) {
      return;
    }
    this.isReacting.set(true);
    try {
      await this.gameState.reactToFastBioProposal(proposal.id, emoji);
      this.burst()?.push(emoji);
    } finally {
      this.isReacting.set(false);
    }
  }

  protected reactionCount(emoji: string): number {
    return this.proposal()?.reactions.find((reaction) => reaction.emoji === emoji)?.count ?? 0;
  }

  protected initials(displayName: string): string {
    return displayName.trim().split(/\s+/).slice(0, 2).map((part) => part[0]?.toUpperCase() ?? '').join('');
  }
}
