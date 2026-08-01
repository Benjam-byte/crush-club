import { HttpClient, HttpErrorResponse } from '@angular/common/http';
import { Injectable, inject, signal } from '@angular/core';
import { firstValueFrom } from 'rxjs';
import type {
  GameConfig,
  GameConfigQuestionInput,
  QuestionDefinition,
} from '@core/models/game.models';

interface SaveGameConfigInput {
  name: string
  isPublic: boolean
  questions: readonly GameConfigQuestionInput[]
  expectedVersion?: number
}

@Injectable({
  providedIn: 'root',
})
export class GameConfigService {
  private readonly http = inject(HttpClient);
  private initializationPromise: Promise<void> | null = null;
  private readonly questionListState = signal<readonly QuestionDefinition[]>([]);
  private readonly configListState = signal<readonly GameConfig[]>([]);
  private readonly isLoadingState = signal(false);
  private readonly errorMessageState = signal<string | null>(null);

  readonly questionList = this.questionListState.asReadonly();
  readonly configList = this.configListState.asReadonly();
  readonly isLoading = this.isLoadingState.asReadonly();
  readonly errorMessage = this.errorMessageState.asReadonly();

  initialize(force = false): Promise<void> {
    if (force) {
      this.initializationPromise = null;
    }
    this.initializationPromise ??= this.loadInitialData();
    return this.initializationPromise;
  }

  async create(
    name: string,
    questions: readonly GameConfigQuestionInput[],
    isPublic = false,
  ): Promise<GameConfig> {
    this.errorMessageState.set(null);
    try {
      const createdConfig = await firstValueFrom(
        this.http.post<GameConfig>('/api/v1/game-configs', { name, questions, isPublic }),
      );
      this.configListState.update((configList) => [...configList, createdConfig]);
      return createdConfig;
    } catch (error: unknown) {
      this.captureError(error);
      throw error;
    }
  }

  async update(
    config: GameConfig,
    name: string,
    questions: readonly GameConfigQuestionInput[],
    isPublic: boolean,
  ): Promise<GameConfig> {
    this.errorMessageState.set(null);
    const input: SaveGameConfigInput = {
      name,
      isPublic,
      questions,
      expectedVersion: config.version,
    };
    try {
      const updatedConfig = await firstValueFrom(
        this.http.put<GameConfig>(`/api/v1/game-configs/${config.id}`, input),
      );
      this.configListState.update((configList) => {
        return configList.map((candidate) => candidate.id === config.id ? updatedConfig : candidate);
      });
      return updatedConfig;
    } catch (error: unknown) {
      this.captureError(error);
      throw error;
    }
  }

  async duplicate(config: GameConfig): Promise<GameConfig> {
    const questions = config.questions.map<GameConfigQuestionInput>((question) => {
      if (question.kind !== 'personal') {
        return { questionId: question.id };
      }
      return {
        label: question.label,
        type: question.type as GameConfigQuestionInput['type'],
        options: question.options?.map((option) => option.label),
        minimum: question.minimum,
        maximum: question.maximum,
      };
    });
    return this.create(`${config.name} (copie)`, questions, false);
  }

  async delete(configId: string): Promise<void> {
    this.errorMessageState.set(null);
    try {
      await firstValueFrom(this.http.delete<void>(`/api/v1/game-configs/${configId}`));
      this.configListState.update((configList) => {
        return configList.filter((config) => config.id !== configId);
      });
    } catch (error: unknown) {
      this.captureError(error);
      throw error;
    }
  }

  private async loadInitialData(): Promise<void> {
    this.isLoadingState.set(true);
    this.errorMessageState.set(null);
    try {
      await firstValueFrom(this.http.post<void>('/api/v1/host-session', null));
      const [questionList, configList] = await Promise.all([
        firstValueFrom(this.http.get<readonly QuestionDefinition[]>('/api/v1/questions')),
        firstValueFrom(this.http.get<readonly GameConfig[]>('/api/v1/game-configs')),
      ]);
      this.questionListState.set(questionList);
      this.configListState.set(configList);
    } catch (error: unknown) {
      this.initializationPromise = null;
      this.captureError(error);
      throw error;
    } finally {
      this.isLoadingState.set(false);
    }
  }

  private captureError(error: unknown): void {
    if (error instanceof HttpErrorResponse) {
      const message = (error.error as { error?: { message?: string } } | null)?.error?.message;
      this.errorMessageState.set(message ?? 'Le serveur ne répond pas. Réessaie dans un instant.');
      return;
    }
    this.errorMessageState.set('Une erreur inattendue est survenue.');
  }
}
