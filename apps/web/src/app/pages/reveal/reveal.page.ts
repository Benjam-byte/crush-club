import { ChangeDetectionStrategy, Component, computed, inject, signal } from '@angular/core';
import type { OnDestroy, OnInit } from '@angular/core';
import { Router } from '@angular/router';
import { IonButton, IonContent, IonIcon } from '@ionic/angular/standalone';
import { DatingProfileCardComponent } from '../../core/components/dating-profile-card/dating-profile-card.component';
import { PageHeaderComponent } from '../../core/components/page-header/page-header.component';
import { ProfilePortraitComponent } from '../../core/components/profile-portrait/profile-portrait.component';
import { createProfileSubmissionList } from '../../core/data/profile-submissions.data';
import { GameStateService } from '../../core/services/game-state.service';

type RevealStep = 'official' | 'waiting' | 'versions' | 'vote';

interface OfficialQuestionAnswer {
  id: string
  label: string
  value: string
  icon: string
  score?: number
  maximumScore?: number
}

@Component({
  selector: 'app-reveal-page',
  imports: [
    DatingProfileCardComponent,
    IonButton,
    IonContent,
    IonIcon,
    PageHeaderComponent,
    ProfilePortraitComponent,
  ],
  templateUrl: './reveal.page.html',
  styleUrl: './reveal.page.scss',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class RevealPage implements OnDestroy, OnInit {
  protected readonly gameState = inject(GameStateService);
  private readonly router = inject(Router);
  private waitingTimeoutId: ReturnType<typeof setTimeout> | undefined;

  protected readonly revealStep = signal<RevealStep>('official');
  protected readonly selectedVersionId = signal<string | null>(null);
  protected readonly submittedProfileCount = signal(3);
  protected readonly revealTitle = computed(() => {
    switch (this.revealStep()) {
      case 'waiting':
        return 'En attente du groupe';
      case 'versions':
        return 'Les versions\ndu groupe';
      case 'vote':
        return 'Vote de Camille';
      default:
        return 'Révélation';
    }
  });
  protected readonly profileSubmissionList = computed(() => {
    const currentPlayer = this.gameState.currentPlayer();
    return createProfileSubmissionList(
      {
        authorName: currentPlayer.displayName,
        avatarIndex: currentPlayer.avatarIndex,
        tagline: this.gameState.tagline(),
        bioAnswerByCategoryId: this.gameState.bioAnswerByCategoryId(),
        answerByQuestionId: this.gameState.answerByQuestionId(),
      },
      this.gameState.activeQuestionList(),
    );
  });
  protected readonly submissionStatusList = computed(() => {
    return this.profileSubmissionList().map((submission, index) => ({
      ...submission,
      isSubmitted: index < this.submittedProfileCount(),
    }));
  });
  protected readonly isEveryProfileSubmitted = computed(() => {
    return this.submittedProfileCount() === this.profileSubmissionList().length;
  });
  private readonly officialQuestionAnswerById: Readonly<Record<string, OfficialQuestionAnswer>> = Object.fromEntries([
    {
      id: 'romance',
      label: 'Niveau de romantisme',
      value: '7/10',
      icon: 'heart',
      score: 7,
      maximumScore: 10,
    },
    { id: 'love-language', label: 'Langage d’amour', value: 'Moments de qualité', icon: 'chatbubble-ellipses' },
    { id: 'first-date', label: 'Rendez-vous idéal', value: 'Pique-nique improvisé', icon: 'restaurant' },
    { id: 'weekend', label: 'Week-end à deux', value: 'Tout improviser', icon: 'compass' },
    {
      id: 'intimacy',
      label: 'Parler de ses envies',
      value: '8/10',
      icon: 'sparkles',
      score: 8,
      maximumScore: 10,
    },
  ].map((answer) => [answer.id, answer]));
  protected readonly officialQuestionAnswerList = computed(() => {
    return this.gameState.activeQuestionList().flatMap((question) => {
      const answer = this.officialQuestionAnswerById[question.id];
      return answer ? [answer] : [];
    });
  });

  async ngOnInit(): Promise<void> {
    try {
      await this.gameState.loadQuestionnaire();
    } catch {
      // GameState exposes the load error.
    }
  }

  ngOnDestroy(): void {
    if (this.waitingTimeoutId !== undefined) {
      clearTimeout(this.waitingTimeoutId);
    }
  }

  protected onShowWaiting(): void {
    this.submittedProfileCount.set(3);
    this.revealStep.set('waiting');
    this.waitingTimeoutId = setTimeout(() => {
      this.submittedProfileCount.set(this.profileSubmissionList().length);
      this.waitingTimeoutId = undefined;
    }, 1200);
  }

  protected onShowVersions(): void {
    if (!this.isEveryProfileSubmitted()) {
      return;
    }

    this.revealStep.set('versions');
  }

  protected onShowVote(): void {
    this.revealStep.set('vote');
  }

  protected onVersionSelect(versionId: string): void {
    this.selectedVersionId.set(versionId);
  }

  protected onCompare(): void {
    const selectedVersionId = this.selectedVersionId();
    if (selectedVersionId === null) {
      return;
    }

    this.gameState.selectBestTagline(selectedVersionId);
    void this.router.navigate(['/game/demo/reveal/1/scores']);
  }
}
