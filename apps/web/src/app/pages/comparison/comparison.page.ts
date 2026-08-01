import { ChangeDetectionStrategy, Component, computed, inject } from '@angular/core';
import type { OnInit } from '@angular/core';
import { Router } from '@angular/router';
import { IonButton, IonContent, IonIcon } from '@ionic/angular/standalone';
import {
  bestTaglineBonusPointCount,
  calculateSimilarityPercentage,
  createProfileSubmissionList,
  type ProfileSubmission,
} from '../../core/data/profile-submissions.data';
import { PageHeaderComponent } from '../../core/components/page-header/page-header.component';
import { ProfilePortraitComponent } from '../../core/components/profile-portrait/profile-portrait.component';
import { GameStateService } from '../../core/services/game-state.service';

interface ScoredProfileSubmission {
  submission: ProfileSubmission
  similarityPercentage: number
  taglineBonus: number
  finalScore: number
}

@Component({
  selector: 'app-comparison-page',
  imports: [IonButton, IonContent, IonIcon, PageHeaderComponent, ProfilePortraitComponent],
  templateUrl: './comparison.page.html',
  styleUrl: './comparison.page.scss',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class ComparisonPage implements OnInit {
  protected readonly gameState = inject(GameStateService);
  private readonly router = inject(Router);

  protected readonly bestTaglineBonusPointCount = bestTaglineBonusPointCount;
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
  protected readonly scoredProfileList = computed<readonly ScoredProfileSubmission[]>(() => {
    const bestTaglineProfileId = this.gameState.bestTaglineProfileId();
    return this.profileSubmissionList()
      .map((submission) => {
        const similarityPercentage = calculateSimilarityPercentage(
          submission,
          this.gameState.activeQuestionList(),
        );
        const taglineBonus =
          submission.id === bestTaglineProfileId ? bestTaglineBonusPointCount : 0;
        return {
          submission,
          similarityPercentage,
          taglineBonus,
          finalScore: similarityPercentage + taglineBonus,
        };
      })
      .sort((firstProfile, secondProfile) => {
        return (
          secondProfile.finalScore - firstProfile.finalScore ||
          secondProfile.similarityPercentage - firstProfile.similarityPercentage
        );
      });
  });

  async ngOnInit(): Promise<void> {
    try {
      await this.gameState.loadQuestionnaire();
    } catch {
      // GameState exposes the load error.
    }
  }
  protected onProfileSelect(profileId: string): void {
    void this.router.navigate(['/game/demo/reveal/1/comparison', profileId]);
  }

  protected onReturnToLobby(): void {
    void this.router.navigate(['/lobby', this.gameState.lobbyCode()]);
  }

  protected onQuitGame(): void {
    this.gameState.resetGame();
    void this.router.navigate(['/']);
  }
}
