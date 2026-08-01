import { ChangeDetectionStrategy, Component, computed, inject, signal } from '@angular/core';
import type { OnInit } from '@angular/core';
import { Router } from '@angular/router';
import { IonButton, IonContent, IonIcon } from '@ionic/angular/standalone';
import type { AnswerValue } from '@core/models/game.models';
import type { BioCategory } from '@core/models/game.models';
import { PageHeaderComponent } from '../../core/components/page-header/page-header.component';
import { QuestionCardComponent } from '../../core/components/question-card/question-card.component';
import { GameStateService } from '../../core/services/game-state.service';

const taglineMaximumLength = 100;

@Component({
  selector: 'app-questionnaire-page',
  imports: [IonButton, IonContent, IonIcon, PageHeaderComponent, QuestionCardComponent],
  templateUrl: './questionnaire.page.html',
  styleUrl: './questionnaire.page.scss',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class QuestionnairePage implements OnInit {
  protected readonly gameState = inject(GameStateService);
  private readonly router = inject(Router);

  protected readonly bioCategoryList = computed<readonly BioCategory[]>(() => {
    return this.gameState.profileFieldList().map((field) => ({
      id: field.id,
      label: field.label,
      optionList: field.options,
    }));
  });
  protected readonly hasTaglineStep = computed(() => this.gameState.role() !== 'lover');
  protected readonly taglineMaximumLength = taglineMaximumLength;
  protected readonly requiredAnswerCount = computed(() => {
    return (this.hasTaglineStep() ? 1 : 0) + this.bioCategoryList().length + this.gameState.activeQuestionList().length;
  });
  protected readonly stepNumberList = computed(() => {
    return Array.from({ length: this.requiredAnswerCount() }, (_, index) => index + 1);
  });
  protected readonly currentStepIndex = signal(0);
  protected readonly currentStepNumber = computed(() => this.currentStepIndex() + 1);
  protected readonly currentBioCategory = computed(() => {
    const categoryIndex = this.currentStepIndex() - (this.hasTaglineStep() ? 1 : 0);
    const categoryList = this.bioCategoryList();
    return categoryIndex >= 0 && categoryIndex < categoryList.length
      ? categoryList[categoryIndex]
      : undefined;
  });
  protected readonly currentQuestion = computed(() => {
    const questionIndex = this.currentStepIndex() - this.bioCategoryList().length - (this.hasTaglineStep() ? 1 : 0);
    const questionList = this.gameState.activeQuestionList();
    return questionIndex >= 0 && questionIndex < questionList.length
      ? questionList[questionIndex]
      : undefined;
  });
  protected readonly isFinalStep = computed(() => {
    return this.currentStepIndex() === this.requiredAnswerCount() - 1;
  });
  protected readonly completedAnswerCount = computed(() => {
    const hasTagline = !this.hasTaglineStep() || this.gameState.tagline().trim().length > 0;
    const bioAnswerByCategoryId = this.gameState.bioAnswerByCategoryId();
    const answerByQuestionId = this.gameState.answerByQuestionId();
    const bioAnswerCount = this.bioCategoryList().filter(
      (category) => bioAnswerByCategoryId[category.id] !== undefined,
    ).length;
    const questionAnswerCount = this.gameState.activeQuestionList().filter(
      (question) => answerByQuestionId[question.id] !== undefined,
    ).length;
    return (this.hasTaglineStep() && hasTagline ? 1 : 0) + bioAnswerCount + questionAnswerCount;
  });
  protected readonly canContinue = computed(() => {
    return this.completedAnswerCount() === this.requiredAnswerCount();
  });
  protected readonly currentStepAnswered = computed(() => {
    if (this.hasTaglineStep() && this.currentStepIndex() === 0) {
      return this.gameState.tagline().trim().length > 0;
    }

    const category = this.currentBioCategory();
    if (category) {
      return this.gameState.bioAnswerByCategoryId()[category.id] !== undefined;
    }

    const question = this.currentQuestion();
    return question
      ? this.gameState.answerByQuestionId()[question.id] !== undefined
      : false;
  });
  protected readonly canAdvance = computed(() => {
    return this.isFinalStep() ? this.canContinue() : this.currentStepAnswered();
  });

  async ngOnInit(): Promise<void> {
    try {
      await this.gameState.loadQuestionnaire();
    } catch {
      // GameState exposes the load error in the template.
    }
  }

  protected onTaglineInput(event: Event): void {
    const textareaElement = event.target as HTMLTextAreaElement;
    this.gameState.saveTagline(textareaElement.value);
  }

  protected onBioOptionSelect(categoryId: string, optionId: string): void {
    this.gameState.saveBioAnswer(categoryId, optionId);
  }

  protected onQuestionAnswer(questionId: string, answer: AnswerValue): void {
    this.gameState.saveQuestionAnswer(questionId, answer);
  }

  protected onPrevious(): void {
    this.currentStepIndex.update((stepIndex) => Math.max(0, stepIndex - 1));
  }

  protected onNext(): void {
    if (!this.canAdvance()) {
      return;
    }

    if (!this.isFinalStep()) {
      this.currentStepIndex.update((stepIndex) => stepIndex + 1);
      return;
    }

    const game = this.gameState.game();
    if (game) {
      void this.router.navigate(['/game', this.gameState.lobbyCode(), 'round', game.roundNumber, 'review']);
    }
  }
}
