import { ChangeDetectionStrategy, Component, computed, inject, signal } from '@angular/core';
import type { OnInit } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { ActivatedRoute, Router } from '@angular/router';
import {
  IonButton,
  IonContent,
  IonIcon,
  IonInput,
  IonSelect,
  IonSelectOption,
  IonToggle,
} from '@ionic/angular/standalone';
import type { GameConfig } from '@core/models/game.models';
import { PageHeaderComponent } from '../../core/components/page-header/page-header.component';
import { GameConfigService } from '../../core/services/game-config.service';
import {
  createBlankEditorQuestion,
  createEditorQuestion,
  isEditorQuestionValid,
  moveEditorQuestion,
  toGameConfigQuestionInput,
  type EditorQuestion,
  type EditorQuestionType,
} from './game-config-editor';

@Component({
  selector: 'app-game-config-form-page',
  imports: [
    FormsModule,
    IonButton,
    IonContent,
    IonIcon,
    IonInput,
    IonSelect,
    IonSelectOption,
    IonToggle,
    PageHeaderComponent,
  ],
  templateUrl: './game-config-form.page.html',
  styleUrl: './game-config-form.page.scss',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class GameConfigFormPage implements OnInit {
  protected readonly gameConfigs = inject(GameConfigService);
  protected readonly editingConfig = signal<GameConfig | null>(null);
  protected readonly editorName = signal('');
  protected readonly editorIsPublic = signal(false);
  protected readonly editorQuestions = signal<readonly EditorQuestion[]>([]);
  protected readonly isPageLoading = signal(true);
  protected readonly isReady = signal(false);
  protected readonly isSaving = signal(false);
  protected readonly pageError = signal<string | null>(null);
  private readonly route = inject(ActivatedRoute);
  private readonly router = inject(Router);
  private nextQuestionKey = 1;

  protected readonly pageTitle = computed(() => {
    return this.editingConfig() ? 'Modifier le formulaire' : 'Nouveau formulaire';
  });
  protected readonly canSave = computed(() => {
    return this.isReady()
      && this.editorName().trim().length > 0
      && this.editorQuestions().length > 0
      && this.editorQuestions().every(isEditorQuestionValid)
      && !this.isSaving();
  });

  async ngOnInit(): Promise<void> {
    await this.loadEditor();
  }

  protected onNameInput(event: Event): void {
    this.editorName.set(this.eventValue(event));
  }

  protected onVisibilityChange(event: Event): void {
    const isPublic = (event as CustomEvent<{ checked: boolean }>).detail.checked;
    this.editorIsPublic.set(isPublic);
  }

  protected addCustomQuestion(): void {
    this.editorQuestions.update((questions) => [
      ...questions,
      createBlankEditorQuestion(this.createQuestionKey()),
    ]);
  }

  protected removeQuestion(key: string): void {
    this.editorQuestions.update((questions) => {
      return questions.filter((question) => question.key !== key);
    });
  }

  protected moveQuestion(key: string, direction: -1 | 1): void {
    this.editorQuestions.update((questions) => moveEditorQuestion(questions, key, direction));
  }

  protected updateLabel(key: string, event: Event): void {
    this.updateQuestion(key, { label: this.eventValue(event) });
  }

  protected updateType(key: string, event: Event): void {
    const type = (event as CustomEvent<{ value: EditorQuestionType }>).detail.value;
    const patch: Partial<EditorQuestion> = { type };
    if (type === 'single_choice') {
      patch.options = ['', ''];
    }
    if (type === 'integer_range') {
      patch.minimum = 0;
      patch.maximum = 10;
      patch.options = [];
    }
    if (type === 'binary_choice') {
      patch.options = [];
    }
    this.updateQuestion(key, patch);
  }

  protected updateMinimum(key: string, event: Event): void {
    this.updateQuestion(key, { minimum: Number(this.eventValue(event)) });
  }

  protected updateMaximum(key: string, event: Event): void {
    this.updateQuestion(key, { maximum: Number(this.eventValue(event)) });
  }

  protected updateOption(key: string, optionIndex: number, event: Event): void {
    const optionValue = this.eventValue(event);
    this.editorQuestions.update((questions) => questions.map((question) => {
      if (question.key !== key) {
        return question;
      }
      const options = [...question.options];
      options[optionIndex] = optionValue;
      return { ...question, options };
    }));
  }

  protected addOption(key: string): void {
    this.editorQuestions.update((questions) => questions.map((question) => {
      return question.key === key && question.options.length < 20
        ? { ...question, options: [...question.options, ''] }
        : question;
    }));
  }

  protected removeOption(key: string, optionIndex: number): void {
    this.editorQuestions.update((questions) => questions.map((question) => {
      return question.key === key
        ? { ...question, options: question.options.filter((_, index) => index !== optionIndex) }
        : question;
    }));
  }

  protected async save(): Promise<void> {
    if (!this.canSave()) {
      return;
    }
    this.isSaving.set(true);
    try {
      const questionInputs = this.editorQuestions().map(toGameConfigQuestionInput);
      const currentConfig = this.editingConfig();
      if (currentConfig) {
        await this.gameConfigs.update(
          currentConfig,
          this.editorName().trim(),
          questionInputs,
          this.editorIsPublic(),
        );
      } else {
        await this.gameConfigs.create(
          this.editorName().trim(),
          questionInputs,
          this.editorIsPublic(),
        );
      }
      await this.goBack();
    } catch {
      // The service exposes the API error.
    } finally {
      this.isSaving.set(false);
    }
  }

  protected async goBack(): Promise<void> {
    await this.router.navigate(['/game-configs']);
  }

  protected async retry(): Promise<void> {
    await this.loadEditor(true);
  }

  private async loadEditor(force = false): Promise<void> {
    this.isPageLoading.set(true);
    this.isReady.set(false);
    this.pageError.set(null);
    try {
      await this.gameConfigs.initialize(force);
      const configId = this.route.snapshot.paramMap.get('id');
      if (configId) {
        const config = this.gameConfigs.configList().find((candidate) => {
          return candidate.id === configId && candidate.kind === 'personal';
        });
        if (!config) {
          this.pageError.set('Ce formulaire est introuvable ou ne t’appartient pas.');
          return;
        }
        this.editingConfig.set(config);
        this.editorName.set(config.name);
        this.editorIsPublic.set(config.isPublic);
        this.editorQuestions.set(
          config.questions.map((question) => createEditorQuestion(question, this.createQuestionKey())),
        );
      } else {
        this.editingConfig.set(null);
        this.editorName.set('Mon questionnaire');
        this.editorIsPublic.set(false);
        this.editorQuestions.set([createBlankEditorQuestion(this.createQuestionKey())]);
      }
      this.isReady.set(true);
    } catch {
      this.pageError.set('Impossible de charger l’éditeur pour le moment.');
    } finally {
      this.isPageLoading.set(false);
    }
  }

  private updateQuestion(key: string, patch: Partial<EditorQuestion>): void {
    this.editorQuestions.update((questions) => questions.map((question) => {
      return question.key === key ? { ...question, ...patch } : question;
    }));
  }

  private eventValue(event: Event): string {
    const detailValue = (event as CustomEvent<{ value?: string | number | null }>).detail?.value;
    if (detailValue !== undefined && detailValue !== null) {
      return String(detailValue);
    }
    return String((event.target as HTMLInputElement & { value?: string }).value ?? '');
  }

  private createQuestionKey(): string {
    const key = `editor-question-${this.nextQuestionKey}`;
    this.nextQuestionKey += 1;
    return key;
  }
}
