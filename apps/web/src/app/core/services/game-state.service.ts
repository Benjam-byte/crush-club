import { HttpErrorResponse } from '@angular/common/http';
import { computed, effect, inject, Injectable, signal } from '@angular/core';
import { Router } from '@angular/router';
import type {
  AnswerValue,
  LobbyPlayer,
  LobbyStateResponse,
  LobbyStatus,
  PlayerRole,
  QuestionnaireSnapshot,
  RoundSubmissionInput,
} from '@core/models/game.models';
import { LobbyApiService } from './lobby-api.service';
import { RealtimeLobbyService } from './realtime-lobby.service';

interface StoredDraft {
  tagline: string
  answerByQuestionId: Record<string, AnswerValue>
  bioAnswerByCategoryId: Record<string, string>
  loverQuestionId: string | null
}

interface StoredPlayerSession {
  lobbyCode: string
  playerId: string
  reconnectToken: string
}

const sessionStorageKey = 'crush-club-player-session-v4';

const emptyPlayer: LobbyPlayer = {
  id: '',
  displayName: '',
  isHost: false,
  isCurrentPlayer: true,
  readyStatus: 'joining',
  connected: false,
  canExclude: false,
  photoIds: [],
  joinedAt: '',
};

@Injectable({
  providedIn: 'root',
})
export class GameStateService {
  private readonly lobbyApi = inject(LobbyApiService);
  private readonly realtime = inject(RealtimeLobbyService);
  private readonly router = inject(Router);
  private readonly storedSessionState = signal<StoredPlayerSession | null>(this.readStoredSession());
  private readonly taglineState = signal('');
  private readonly answerByQuestionIdState = signal<Record<string, AnswerValue>>({});
  private readonly bioAnswerByCategoryIdState = signal<Record<string, string>>({});
  private readonly loverQuestionIdState = signal<string | null>(null);
  private readonly errorMessageState = signal<string | null>(null);
  private readonly photoUrlByIdState = signal<Readonly<Record<string, string>>>({});
  private readonly loadingPhotoIdSet = new Set<string>();
  private restoredDraftKey = '';

  readonly state = this.realtime.state;
  readonly connectionStatus = this.realtime.connectionStatus;
  readonly isRealtimeConnected = computed(() => this.connectionStatus() === 'connected');
  readonly lobbyCode = computed(() => this.state()?.code ?? this.storedSessionState()?.lobbyCode ?? '');
  readonly lobbyStatus = computed<LobbyStatus>(() => this.state()?.status ?? 'waiting_for_players');
  readonly displayName = computed(() => this.currentPlayer().displayName);
  readonly isHost = computed(() => this.currentPlayer().isHost);
  readonly playerList = computed<readonly LobbyPlayer[]>(() => {
    const state = this.state();
    if (!state) {
      return [];
    }
    return state.players.map((player) => ({
      ...player,
      isCurrentPlayer: player.id === state.currentPlayerId,
    }));
  });
  readonly currentPlayer = computed<LobbyPlayer>(() => {
    return this.playerList().find((player) => player.isCurrentPlayer) ?? emptyPlayer;
  });
  readonly gameConfigId = computed<string | null>(() => this.state()?.gameConfig.id ?? null);
  readonly gameConfigName = computed(() => this.state()?.gameConfig.name ?? '');
  readonly gameConfigVersion = computed(() => this.state()?.gameConfig.version ?? 0);
  readonly gameConfigQuestionCount = computed(() => this.state()?.gameConfig.questionCount ?? 0);
  readonly questionnaireSnapshot = computed<QuestionnaireSnapshot | null>(() => this.state()?.questionnaire ?? null);
  readonly activeQuestionList = computed(() => this.questionnaireSnapshot()?.questions ?? []);
  readonly profileFieldList = computed(() => this.questionnaireSnapshot()?.profileFields ?? []);
  readonly role = computed<PlayerRole>(() => this.state()?.game?.role === 'subject' ? 'lover' : 'cupid');
  readonly game = computed(() => this.state()?.game ?? null);
  readonly subjectPlayer = computed(() => {
    const subjectID = this.game()?.subjectPlayerId;
    return this.playerList().find((player) => player.id === subjectID) ?? emptyPlayer;
  });
  readonly nextSubjectPlayer = computed(() => {
    const subjectID = this.game()?.nextSubjectPlayerId;
    return this.playerList().find((player) => player.id === subjectID) ?? emptyPlayer;
  });
  readonly tagline = this.taglineState.asReadonly();
  readonly answerByQuestionId = this.answerByQuestionIdState.asReadonly();
  readonly bioAnswerByCategoryId = this.bioAnswerByCategoryIdState.asReadonly();
  readonly loverQuestionId = this.loverQuestionIdState.asReadonly();
  readonly isProfileLocked = computed(() => this.game()?.submitted ?? false);
  readonly isQuestionnaireLoading = computed(() => this.state() === null);
  readonly errorMessage = this.errorMessageState.asReadonly();
  readonly canStartGame = computed(() => {
    const players = this.playerList();
    return this.game() === null &&
      this.isHost() &&
      this.isRealtimeConnected() &&
      players.length >= 2 &&
      players.length <= 10 &&
      players.every((player) => player.connected && player.readyStatus === 'ready' && player.photoIds.length === 4);
  });
  readonly canStartNextRound = computed(() => {
    const game = this.game();
    const players = this.playerList();
    return game?.phase === 'between_rounds' &&
      this.isHost() &&
      this.isRealtimeConnected() &&
      players.length >= 2 &&
      players.every((player) => player.connected);
  });

  constructor() {
    effect(() => {
      const state = this.state();
      if (state) {
        void this.loadMissingPhotos(state);
        void this.synchronizeRoute(state);
      }
    });
  }

  async createLobby(displayName: string, gameConfigId: string): Promise<string> {
    this.errorMessageState.set(null);
    try {
      const session = await this.lobbyApi.create({
        displayName,
        maxPlayers: 10,
        gameConfigId,
      });
      this.establishSession(session.state, session.reconnectToken);
      return session.state.code;
    } catch (error: unknown) {
      this.captureError(error, 'Impossible de créer le lobby pour le moment.');
      throw error;
    }
  }

  async joinLobby(displayName: string, lobbyCode: string): Promise<string> {
    const normalizedCode = lobbyCode.toUpperCase();
    this.errorMessageState.set(null);
    try {
      const session = await this.lobbyApi.join(normalizedCode, displayName);
      this.establishSession(session.state, session.reconnectToken);
      return session.state.code;
    } catch (error: unknown) {
      this.captureError(error, 'Ce lobby est introuvable, complet ou déjà lancé.');
      throw error;
    }
  }

  async refreshLobby(routeCode?: string): Promise<void> {
    const session = this.storedSessionState();
    const code = (routeCode ?? session?.lobbyCode ?? '').toUpperCase();
    if (!session || session.lobbyCode !== code) {
      this.errorMessageState.set('Rejoins ce lobby pour accéder à sa partie.');
      throw new Error('No player session for lobby');
    }
    this.errorMessageState.set(null);
    try {
      const state = await this.lobbyApi.getState(code, session.reconnectToken);
      this.realtime.setState(state);
      this.realtime.connect(code, session.reconnectToken);
    } catch (error: unknown) {
      this.captureError(error, 'Ta session joueur a expiré. Rejoins à nouveau le lobby.');
      throw error;
    }
  }

  async changeLobbyConfig(gameConfigId: string): Promise<void> {
    await this.runStateCommand(
      (code, token) => this.lobbyApi.changeConfig(code, token, gameConfigId),
      'La configuration du lobby n’a pas pu être modifiée.',
    );
  }

  async uploadPhotos(photoList: readonly File[]): Promise<void> {
    await this.runStateCommand(
      (code, token) => this.lobbyApi.uploadPhotos(code, token, photoList),
      'Les quatre photos n’ont pas pu être envoyées.',
    );
  }

  async loadQuestionnaire(): Promise<void> {
    if (!this.state()) {
      await this.refreshLobby();
    }
    this.restoreDraftForCurrentRound();
  }

  async startGame(): Promise<void> {
    if (!this.canStartGame()) {
      return;
    }
    await this.runStateCommand(
      (code, token) => this.lobbyApi.start(code, token),
      'La partie n’a pas pu démarrer.',
    );
  }

  saveTagline(tagline: string): void {
    this.taglineState.set(tagline);
    this.persistDraft();
  }

  saveBioAnswer(categoryId: string, optionId: string): void {
    this.bioAnswerByCategoryIdState.update((answerByCategoryId) => ({
      ...answerByCategoryId,
      [categoryId]: optionId,
    }));
    this.persistDraft();
  }

  saveQuestionAnswer(questionId: string, answer: AnswerValue): void {
    if (!this.activeQuestionList().some((question) => question.id === questionId)) {
      return;
    }
    this.answerByQuestionIdState.update((answerByQuestionId) => ({
      ...answerByQuestionId,
      [questionId]: answer,
    }));
    this.persistDraft();
  }

  selectLoverQuestion(questionId: string): void {
    const question = this.activeQuestionList().find((candidate) => candidate.id === questionId);
    if (!question?.loverEligible || this.role() === 'lover') {
      return;
    }
    this.loverQuestionIdState.set(questionId);
    this.persistDraft();
  }

  async lockProfile(): Promise<void> {
    const game = this.game();
    if (!game || game.submitted) {
      return;
    }
    const input: RoundSubmissionInput = {
      bioAnswers: this.bioAnswerByCategoryId(),
      questionAnswers: this.answerByQuestionId(),
    };
    if (game.role === 'cupid') {
      input.tagline = this.tagline().trim();
      input.loverQuestionId = this.loverQuestionId() ?? undefined;
    }
    await this.runStateCommand(
      (code, token) => this.lobbyApi.submit(code, token, input),
      'Ton profil n’a pas pu être verrouillé.',
    );
    localStorage.removeItem(this.currentDraftStorageKey());
  }

  async voteForTagline(submissionId: string): Promise<void> {
    await this.runStateCommand(
      (code, token) => this.lobbyApi.vote(code, token, submissionId),
      'Le vote n’a pas pu être enregistré.',
    );
  }

  async returnToLobby(): Promise<void> {
    await this.runStateCommand(
      (code, token) => this.lobbyApi.closeCurrentRound(code, token),
      'Le retour au lobby n’a pas pu être synchronisé.',
    );
  }

  async startNextRound(): Promise<void> {
    if (!this.canStartNextRound()) {
      return;
    }
    await this.runStateCommand(
      (code, token) => this.lobbyApi.startNextRound(code, token),
      'La manche suivante n’a pas pu démarrer.',
    );
  }

  async excludePlayer(playerId: string): Promise<void> {
    await this.runStateCommand(
      (code, token) => this.lobbyApi.excludePlayer(code, token, playerId),
      'Ce joueur ne peut pas encore être exclu.',
    );
  }

  photoUrl(photoId: string | undefined): string | null {
    return photoId ? this.photoUrlByIdState()[photoId] ?? null : null;
  }

  resetGame(): void {
    localStorage.removeItem(this.currentDraftStorageKey());
    localStorage.removeItem(sessionStorageKey);
    this.storedSessionState.set(null);
    this.realtime.disconnect();
    for (const url of Object.values(this.photoUrlByIdState())) {
      URL.revokeObjectURL(url);
    }
    this.photoUrlByIdState.set({});
    this.taglineState.set('');
    this.answerByQuestionIdState.set({});
    this.bioAnswerByCategoryIdState.set({});
    this.loverQuestionIdState.set(null);
    this.errorMessageState.set(null);
    this.restoredDraftKey = '';
  }

  private establishSession(state: LobbyStateResponse, reconnectToken: string): void {
    const session: StoredPlayerSession = {
      lobbyCode: state.code,
      playerId: state.currentPlayerId,
      reconnectToken,
    };
    localStorage.setItem(sessionStorageKey, JSON.stringify(session));
    this.storedSessionState.set(session);
    this.realtime.setState(state);
    this.realtime.connect(state.code, reconnectToken);
    this.restoredDraftKey = '';
  }

  private async runStateCommand(
    command: (code: string, reconnectToken: string) => Promise<LobbyStateResponse>,
    fallbackMessage: string,
  ): Promise<void> {
    const session = this.storedSessionState();
    if (!session) {
      this.errorMessageState.set('Ta session joueur est introuvable.');
      throw new Error('Missing player session');
    }
    this.errorMessageState.set(null);
    try {
      const state = await command(session.lobbyCode, session.reconnectToken);
      this.realtime.setState(state);
    } catch (error: unknown) {
      this.captureError(error, fallbackMessage);
      throw error;
    }
  }

  private restoreDraftForCurrentRound(): void {
    const draftKey = this.currentDraftStorageKey();
    if (this.restoredDraftKey === draftKey) {
      return;
    }
    this.restoredDraftKey = draftKey;
    const storedValue = localStorage.getItem(draftKey);
    let draft: StoredDraft = {
      tagline: '',
      answerByQuestionId: {},
      bioAnswerByCategoryId: {},
      loverQuestionId: null,
    };
    if (storedValue) {
      try {
        draft = { ...draft, ...(JSON.parse(storedValue) as Partial<StoredDraft>) };
      } catch (error: unknown) {
        console.error('Unable to restore the local questionnaire draft', error);
      }
    }
    const validQuestionIDs = new Set(this.activeQuestionList().map((question) => question.id));
    this.taglineState.set(draft.tagline);
    this.answerByQuestionIdState.set(Object.fromEntries(
      Object.entries(draft.answerByQuestionId).filter(([questionId]) => validQuestionIDs.has(questionId)),
    ));
    this.bioAnswerByCategoryIdState.set(draft.bioAnswerByCategoryId);
    const loverQuestion = this.activeQuestionList().find((question) => {
      return question.id === draft.loverQuestionId && question.loverEligible;
    });
    this.loverQuestionIdState.set(this.role() === 'lover' ? null : loverQuestion?.id ?? null);
  }

  private persistDraft(): void {
    if (!this.state()?.game || this.isProfileLocked()) {
      return;
    }
    const draft: StoredDraft = {
      tagline: this.tagline(),
      answerByQuestionId: this.answerByQuestionId(),
      bioAnswerByCategoryId: this.bioAnswerByCategoryId(),
      loverQuestionId: this.loverQuestionId(),
    };
    localStorage.setItem(this.currentDraftStorageKey(), JSON.stringify(draft));
  }

  private currentDraftStorageKey(): string {
    const session = this.storedSessionState();
    const game = this.game();
    return `crush-club-draft:${session?.lobbyCode ?? 'none'}:${session?.playerId ?? 'none'}:${game?.roundNumber ?? 0}:${this.gameConfigVersion()}`;
  }

  private readStoredSession(): StoredPlayerSession | null {
    const storedValue = localStorage.getItem(sessionStorageKey);
    if (!storedValue) {
      return null;
    }
    try {
      const session = JSON.parse(storedValue) as Partial<StoredPlayerSession>;
      if (!session.lobbyCode || !session.playerId || !session.reconnectToken) {
        return null;
      }
      return session as StoredPlayerSession;
    } catch {
      return null;
    }
  }

  private async loadMissingPhotos(state: LobbyStateResponse): Promise<void> {
    const session = this.storedSessionState();
    if (!session || session.lobbyCode !== state.code) {
      return;
    }
    const photoIDList = state.players.flatMap((player) => player.photoIds);
    for (const photoID of photoIDList) {
      if (this.photoUrlByIdState()[photoID] || this.loadingPhotoIdSet.has(photoID)) {
        continue;
      }
      this.loadingPhotoIdSet.add(photoID);
      try {
        const blob = await this.lobbyApi.getPhoto(state.code, session.reconnectToken, photoID);
        const url = URL.createObjectURL(blob);
        this.photoUrlByIdState.update((current) => ({ ...current, [photoID]: url }));
      } catch (error: unknown) {
        console.error('Unable to load a private lobby photo', error);
      } finally {
        this.loadingPhotoIdSet.delete(photoID);
      }
    }
  }

  private async synchronizeRoute(state: LobbyStateResponse): Promise<void> {
    const currentURL = this.router.url.split('?')[0];
    if (!currentURL.startsWith('/lobby/') && !currentURL.startsWith('/game/')) {
      return;
    }
    if (!state.game || !state.status || state.status !== 'in_game' && state.status !== 'completed') {
      const lobbyURL = `/lobby/${state.code}`;
      if (currentURL !== lobbyURL && currentURL.startsWith('/game/')) {
        await this.router.navigateByUrl(lobbyURL);
      }
      return;
    }
    if (state.game.phase === 'between_rounds') {
      const lobbyURL = `/lobby/${state.code}`;
      if (currentURL !== lobbyURL) {
        await this.router.navigateByUrl(lobbyURL);
      }
      return;
    }
    const roundBase = `/game/${state.code}/round/${state.game.roundNumber}`;
    let expectedPrefix = roundBase;
    let destination = `${roundBase}/role`;
    if (state.game.phase === 'reveal_and_vote') {
      expectedPrefix = `${roundBase}/reveal`;
      destination = expectedPrefix;
    } else if (state.game.phase === 'round_results') {
      expectedPrefix = `${roundBase}/scores`;
      destination = expectedPrefix;
    } else if (state.game.phase === 'completed') {
      expectedPrefix = `/game/${state.code}/final`;
      destination = expectedPrefix;
    }
    const collectingRouteIsValid = state.game.phase === 'collecting_submissions' &&
      (currentURL.startsWith(roundBase) && !currentURL.includes('/reveal') && !currentURL.includes('/scores'));
    if (!collectingRouteIsValid && !currentURL.startsWith(expectedPrefix)) {
      await this.router.navigateByUrl(destination);
    }
  }

  private captureError(error: unknown, fallbackMessage: string): void {
    if (error instanceof HttpErrorResponse) {
      const serverMessage = (error.error as { error?: { message?: string } } | null)?.error?.message;
      this.errorMessageState.set(serverMessage ?? fallbackMessage);
      return;
    }
    this.errorMessageState.set(fallbackMessage);
  }
}
