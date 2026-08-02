import { ChangeDetectionStrategy, Component, computed, inject, signal } from '@angular/core';
import type { OnInit } from '@angular/core';
import { ActivatedRoute, Router } from '@angular/router';
import { IonButton, IonContent, IonIcon } from '@ionic/angular/standalone';
import { primaryPhotoQuestionId } from '@core/models/game.models';
import type { AnswerValue, QuestionDefinition } from '@core/models/game.models';
import { PageHeaderComponent } from '../../core/components/page-header/page-header.component';
import { GameStateService } from '../../core/services/game-state.service';

interface DisplayedAnswer {
  id: string
  label: string
  value: string
}

@Component({
  selector: 'app-reveal-page',
  imports: [IonButton, IonContent, IonIcon, PageHeaderComponent],
  templateUrl: './reveal.page.html',
  styleUrl: './reveal.page.scss',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class RevealPage implements OnInit {
  protected readonly gameState = inject(GameStateService);
  private readonly route = inject(ActivatedRoute);
  private readonly router = inject(Router);
  protected readonly selectedSubmissionId = signal<string | null>(null);
  protected readonly isVoting = signal(false);
  protected readonly subjectPhotoUrl = computed(() => {
    const photoAnswer = this.gameState.game()?.officialSubmission?.questionAnswers[primaryPhotoQuestionId];
    const photoId = typeof photoAnswer === 'string'
      ? photoAnswer
      : this.gameState.subjectPlayer().photoIds[0];
    return this.gameState.photoUrl(photoId);
  });
  protected readonly officialAnswerList = computed<readonly DisplayedAnswer[]>(() => {
    const submission = this.gameState.game()?.officialSubmission;
    if (!submission) {
      return [];
    }
    const bioAnswers = this.gameState.profileFieldList().map((field) => ({
      id: field.id,
      label: field.label,
      value: field.options.find((option) => option.id === submission.bioAnswers[field.id])?.label ?? 'Sans réponse',
    }));
    const questionAnswers = this.gameState.activeQuestionList().map((question) => ({
      id: question.id,
      label: question.label,
      value: this.formatAnswer(question, submission.questionAnswers[question.id]),
    }));
    return [...bioAnswers, ...questionAnswers];
  });

  async ngOnInit(): Promise<void> {
    try {
      await this.gameState.refreshLobby(this.route.snapshot.paramMap.get('code') ?? undefined);
    } catch {
      await this.router.navigate(['/join'], { queryParams: { code: this.route.snapshot.paramMap.get('code') } });
    }
  }

  protected selectSubmission(submissionId: string): void {
    if (this.gameState.role() === 'lover') {
      this.selectedSubmissionId.set(submissionId);
    }
  }

  protected async submitVote(): Promise<void> {
    const submissionId = this.selectedSubmissionId();
    if (!submissionId || this.gameState.role() !== 'lover') {
      return;
    }
    this.isVoting.set(true);
    try {
      await this.gameState.voteForTagline(submissionId);
    } catch {
      // GameState exposes the API error.
    } finally {
      this.isVoting.set(false);
    }
  }

  protected initials(displayName: string): string {
    return displayName.trim().split(/\s+/).slice(0, 2).map((part) => part[0]?.toUpperCase() ?? '').join('');
  }

  protected submissionBio(submissionId: string): string {
    const submission = this.gameState.game()?.submissions?.find((candidate) => candidate.id === submissionId);
    if (!submission) {
      return '';
    }
    return this.gameState.profileFieldList().map((field) => {
      return field.options.find((option) => option.id === submission.bioAnswers[field.id])?.label ?? '';
    }).filter(Boolean).join(' · ');
  }

  private formatAnswer(question: QuestionDefinition, answer: AnswerValue | undefined): string {
    if (typeof answer === 'number') {
      return `${answer}/${question.maximum ?? 10}`;
    }
    if (typeof answer === 'string') {
      return question.options?.find((option) => option.id === answer)?.label ?? answer;
    }
    return 'Sans réponse';
  }
}
