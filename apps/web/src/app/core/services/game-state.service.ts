import { computed, inject, Injectable, signal } from '@angular/core';
import type {
  AnswerValue,
  LobbyPlayer,
  LobbyResponse,
  LobbyStatus,
  PlayerRole,
  QuestionnaireSnapshot,
} from '@core/models/game.models';
import { LobbyApiService } from './lobby-api.service';

interface StoredDraft {
  tagline: string
  answerByQuestionId: Record<string, AnswerValue>
  bioAnswerByCategoryId: Record<string, string>
  loverQuestionId: string | null
}

interface StoredSession {
  displayName: string
  lobbyCode: string
  isHost: boolean
  lobbyStatus: LobbyStatus
  gameConfigId: string | null
  gameConfigName: string
  gameConfigVersion: number
  gameConfigQuestionCount: number
}

const sessionStorageKey = 'crush-club-session-v3';

@Injectable({
  providedIn: 'root',
})
export class GameStateService {
  private readonly lobbyApi = inject(LobbyApiService);
  private readonly storedSession = this.readStoredSession();
  private readonly displayNameState = signal(this.storedSession.displayName);
  private readonly lobbyCodeState = signal(this.storedSession.lobbyCode);
  private readonly isHostState = signal(this.storedSession.isHost);
  private readonly lobbyStatusState = signal<LobbyStatus>(this.storedSession.lobbyStatus);
  private readonly roleState = signal<PlayerRole>('cupid');
  private readonly playerListState = signal<readonly LobbyPlayer[]>(
    this.createPlayerList(this.storedSession.displayName, this.storedSession.isHost),
  );
  private readonly gameConfigIdState = signal<string | null>(this.storedSession.gameConfigId);
  private readonly gameConfigNameState = signal(this.storedSession.gameConfigName);
  private readonly gameConfigVersionState = signal(this.storedSession.gameConfigVersion);
  private readonly gameConfigQuestionCountState = signal(this.storedSession.gameConfigQuestionCount);
  private readonly questionnaireSnapshotState = signal<QuestionnaireSnapshot | null>(null);
  private readonly taglineState = signal('');
  private readonly answerByQuestionIdState = signal<Record<string, AnswerValue>>({});
  private readonly bioAnswerByCategoryIdState = signal<Record<string, string>>({});
  private readonly loverQuestionIdState = signal<string | null>(null);
  private readonly bestTaglineProfileIdState = signal<string | null>(null);
  private readonly isProfileLockedState = signal(false);
  private readonly isQuestionnaireLoadingState = signal(false);
  private readonly errorMessageState = signal<string | null>(null);

  readonly displayName = this.displayNameState.asReadonly();
  readonly lobbyCode = this.lobbyCodeState.asReadonly();
  readonly isHost = this.isHostState.asReadonly();
  readonly lobbyStatus = this.lobbyStatusState.asReadonly();
  readonly role = this.roleState.asReadonly();
  readonly playerList = this.playerListState.asReadonly();
  readonly gameConfigId = this.gameConfigIdState.asReadonly();
  readonly gameConfigName = this.gameConfigNameState.asReadonly();
  readonly gameConfigVersion = this.gameConfigVersionState.asReadonly();
  readonly gameConfigQuestionCount = this.gameConfigQuestionCountState.asReadonly();
  readonly questionnaireSnapshot = this.questionnaireSnapshotState.asReadonly();
  readonly activeQuestionList = computed(() => this.questionnaireSnapshot()?.questions ?? []);
  readonly tagline = this.taglineState.asReadonly();
  readonly answerByQuestionId = this.answerByQuestionIdState.asReadonly();
  readonly bioAnswerByCategoryId = this.bioAnswerByCategoryIdState.asReadonly();
  readonly loverQuestionId = this.loverQuestionIdState.asReadonly();
  readonly bestTaglineProfileId = this.bestTaglineProfileIdState.asReadonly();
  readonly isProfileLocked = this.isProfileLockedState.asReadonly();
  readonly isQuestionnaireLoading = this.isQuestionnaireLoadingState.asReadonly();
  readonly errorMessage = this.errorMessageState.asReadonly();

  readonly currentPlayer = computed(() => {
    return this.playerList().find((player) => player.isCurrentPlayer) ?? this.playerList()[0];
  });

  readonly isEveryPlayerReady = computed(() => {
    return this.playerList().every((player) => player.readyStatus === 'ready');
  });

  readonly canStartGame = computed(() => {
    return this.isHost() && this.isEveryPlayerReady();
  });

  async createLobby(
    displayName: string,
    gameConfigId: string,
    adultConfirmed: boolean,
  ): Promise<string> {
    this.errorMessageState.set(null);
    try {
      const lobby = await this.lobbyApi.create({
        displayName,
        adultConfirmed,
        maxPlayers: 8,
        gameConfigId,
      });
      this.initializeSession(displayName, lobby, true);
      if (lobby.reconnectToken) {
        localStorage.setItem(`crush-club-reconnect-${lobby.code}`, lobby.reconnectToken);
      }
      return lobby.code;
    } catch (error: unknown) {
      this.errorMessageState.set('Impossible de créer le lobby pour le moment.');
      throw error;
    }
  }

  async joinLobby(displayName: string, lobbyCode: string): Promise<string> {
    const normalizedLobbyCode = lobbyCode.toUpperCase();
    this.errorMessageState.set(null);
    try {
      const lobby = await this.lobbyApi.get(normalizedLobbyCode);
      this.initializeSession(displayName, lobby, false);
      return normalizedLobbyCode;
    } catch (error: unknown) {
      this.errorMessageState.set('Ce lobby est introuvable ou a expiré.');
      throw error;
    }
  }

  async refreshLobby(): Promise<void> {
    const lobby = await this.lobbyApi.get(this.lobbyCode());
    this.applyLobby(lobby);
  }

  async changeLobbyConfig(gameConfigId: string): Promise<void> {
    this.errorMessageState.set(null);
    try {
      const lobby = await this.lobbyApi.changeConfig(this.lobbyCode(), gameConfigId);
      this.applyLobby(lobby);
      this.questionnaireSnapshotState.set(null);
    } catch (error: unknown) {
      this.errorMessageState.set('La configuration du lobby n’a pas pu être modifiée.');
      throw error;
    }
  }

  async loadQuestionnaire(force = false): Promise<void> {
    if (!force && this.questionnaireSnapshot()?.sourceVersion === this.gameConfigVersion()) {
      return;
    }
    this.isQuestionnaireLoadingState.set(true);
    this.errorMessageState.set(null);
    try {
      const snapshot = await this.lobbyApi.getQuestionnaire(this.lobbyCode());
      this.questionnaireSnapshotState.set(snapshot);
      this.gameConfigIdState.set(snapshot.sourceConfigId);
      this.gameConfigNameState.set(snapshot.name);
      this.gameConfigVersionState.set(snapshot.sourceVersion);
      this.gameConfigQuestionCountState.set(snapshot.questions.length);
      this.restoreDraft(snapshot);
      this.persistSession();
    } catch (error: unknown) {
      this.errorMessageState.set('Le questionnaire n’a pas pu être chargé.');
      throw error;
    } finally {
      this.isQuestionnaireLoadingState.set(false);
    }
  }

  setCurrentPlayerReady(): void {
    this.playerListState.update((playerList) => {
      return playerList.map((player) => {
        if (!player.isCurrentPlayer) {
          return player;
        }

        return {
          ...player,
          readyStatus: 'ready',
        };
      });
    });
    this.lobbyStatusState.set('ready_to_start');
    this.persistSession();
  }

  async startGame(): Promise<void> {
    if (!this.canStartGame()) {
      return;
    }
    const lobby = await this.lobbyApi.start(this.lobbyCode());
    this.applyLobby(lobby);
    await this.loadQuestionnaire(true);
  }

  saveTagline(tagline: string): void {
    this.taglineState.set(tagline);
    this.persistDraft();
  }

  saveBioAnswer(categoryId: string, optionId: string): void {
    this.bioAnswerByCategoryIdState.update((answerByCategoryId) => {
      return {
        ...answerByCategoryId,
        [categoryId]: optionId,
      };
    });
    this.persistDraft();
  }

  saveQuestionAnswer(questionId: string, answer: AnswerValue): void {
    if (!this.activeQuestionList().some((question) => question.id === questionId)) {
      return;
    }
    this.answerByQuestionIdState.update((answerByQuestionId) => {
      return {
        ...answerByQuestionId,
        [questionId]: answer,
      };
    });
    this.persistDraft();
  }

  selectLoverQuestion(questionId: string): void {
    const question = this.activeQuestionList().find((candidate) => candidate.id === questionId);
    if (!question?.loverEligible) {
      return;
    }
    this.loverQuestionIdState.set(questionId);
    this.persistDraft();
  }

  selectBestTagline(profileId: string): void {
    this.bestTaglineProfileIdState.set(profileId);
  }

  lockProfile(): void {
    this.isProfileLockedState.set(true);
    localStorage.removeItem(this.currentDraftStorageKey());
  }

  resetGame(): void {
    localStorage.removeItem(this.currentDraftStorageKey());
    localStorage.removeItem(sessionStorageKey);
    this.displayNameState.set('Léa');
    this.lobbyCodeState.set('AB7K');
    this.isHostState.set(true);
    this.lobbyStatusState.set('preparing_photos');
    this.playerListState.set(this.createPlayerList('Léa'));
    this.gameConfigIdState.set(null);
    this.gameConfigNameState.set('');
    this.gameConfigVersionState.set(0);
    this.gameConfigQuestionCountState.set(0);
    this.questionnaireSnapshotState.set(null);
    this.taglineState.set('');
    this.answerByQuestionIdState.set({});
    this.bioAnswerByCategoryIdState.set({});
    this.loverQuestionIdState.set(null);
    this.bestTaglineProfileIdState.set(null);
    this.isProfileLockedState.set(false);
  }

  private initializeSession(displayName: string, lobby: LobbyResponse, isHost: boolean): void {
    this.displayNameState.set(displayName);
    this.lobbyCodeState.set(lobby.code);
    this.isHostState.set(isHost);
    this.playerListState.set(this.createPlayerList(displayName, isHost));
    this.applyLobby(lobby);
    this.questionnaireSnapshotState.set(null);
    this.taglineState.set('');
    this.answerByQuestionIdState.set({});
    this.bioAnswerByCategoryIdState.set({});
    this.loverQuestionIdState.set(null);
    this.persistSession();
  }

  private applyLobby(lobby: LobbyResponse): void {
    this.lobbyCodeState.set(lobby.code);
    this.lobbyStatusState.set(lobby.status);
    this.gameConfigIdState.set(lobby.gameConfig.id);
    this.gameConfigNameState.set(lobby.gameConfig.name);
    this.gameConfigVersionState.set(lobby.gameConfig.version);
    this.gameConfigQuestionCountState.set(lobby.gameConfig.questionCount);
    this.persistSession();
  }

  private createPlayerList(displayName: string, isHost = true): readonly LobbyPlayer[] {
    return [
      {
        id: 'player-current',
        displayName,
        avatarIndex: 0,
        isHost,
        isCurrentPlayer: true,
        readyStatus: 'preparing_photos',
      },
      {
        id: 'player-marco',
        displayName: 'Marco',
        avatarIndex: 1,
        isHost: !isHost,
        isCurrentPlayer: false,
        readyStatus: 'ready',
      },
      {
        id: 'player-ines',
        displayName: 'Inès',
        avatarIndex: 2,
        isHost: false,
        isCurrentPlayer: false,
        readyStatus: 'ready',
      },
      {
        id: 'player-tom',
        displayName: 'Tom',
        avatarIndex: 3,
        isHost: false,
        isCurrentPlayer: false,
        readyStatus: 'ready',
      },
    ];
  }

  private restoreDraft(snapshot: QuestionnaireSnapshot): void {
    const storedDraft = this.readStoredDraft();
    const validQuestionIDs = new Set(snapshot.questions.map((question) => question.id));
    const validAnswerByQuestionId = Object.fromEntries(
      Object.entries(storedDraft.answerByQuestionId).filter(([questionId]) => validQuestionIDs.has(questionId)),
    );
    const loverQuestion = snapshot.questions.find((question) => {
      return question.id === storedDraft.loverQuestionId && question.loverEligible;
    });
    this.taglineState.set(storedDraft.tagline);
    this.answerByQuestionIdState.set(validAnswerByQuestionId);
    this.bioAnswerByCategoryIdState.set(storedDraft.bioAnswerByCategoryId);
    this.loverQuestionIdState.set(loverQuestion?.id ?? null);
  }

  private persistDraft(): void {
    const storedDraft: StoredDraft = {
      tagline: this.tagline(),
      answerByQuestionId: this.answerByQuestionId(),
      bioAnswerByCategoryId: this.bioAnswerByCategoryId(),
      loverQuestionId: this.loverQuestionId(),
    };
    localStorage.setItem(this.currentDraftStorageKey(), JSON.stringify(storedDraft));
  }

  private readStoredDraft(): StoredDraft {
    const emptyDraft: StoredDraft = {
      tagline: '',
      answerByQuestionId: {},
      bioAnswerByCategoryId: {},
      loverQuestionId: null,
    };
    const storedValue = localStorage.getItem(this.currentDraftStorageKey());
    if (storedValue === null) {
      return emptyDraft;
    }
    try {
      return JSON.parse(storedValue) as StoredDraft;
    } catch (error: unknown) {
      console.error('Unable to restore the local questionnaire draft', error);
      return emptyDraft;
    }
  }

  private currentDraftStorageKey(): string {
    const playerKey = this.displayName().trim().toLocaleLowerCase().replace(/\s+/g, '-');
    return `crush-club-draft:${this.lobbyCode()}:${playerKey}:${this.gameConfigId() ?? 'none'}:${this.gameConfigVersion()}`;
  }

  private persistSession(): void {
    const session: StoredSession = {
      displayName: this.displayName(),
      lobbyCode: this.lobbyCode(),
      isHost: this.isHost(),
      lobbyStatus: this.lobbyStatus(),
      gameConfigId: this.gameConfigId(),
      gameConfigName: this.gameConfigName(),
      gameConfigVersion: this.gameConfigVersion(),
      gameConfigQuestionCount: this.gameConfigQuestionCount(),
    };
    localStorage.setItem(sessionStorageKey, JSON.stringify(session));
  }

  private readStoredSession(): StoredSession {
    const fallback: StoredSession = {
      displayName: 'Léa',
      lobbyCode: 'AB7K',
      isHost: true,
      lobbyStatus: 'preparing_photos',
      gameConfigId: null,
      gameConfigName: '',
      gameConfigVersion: 0,
      gameConfigQuestionCount: 0,
    };
    const storedValue = localStorage.getItem(sessionStorageKey);
    if (!storedValue) {
      return fallback;
    }
    try {
      return { ...fallback, ...(JSON.parse(storedValue) as Partial<StoredSession>) };
    } catch {
      return fallback;
    }
  }
}
