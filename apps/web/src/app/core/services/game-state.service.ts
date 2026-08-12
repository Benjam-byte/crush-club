import { HttpErrorResponse } from '@angular/common/http';
import { computed, effect, inject, Injectable, signal } from '@angular/core';
import { Router } from '@angular/router';
import { activeNavigationUrl } from '@core/guards/lobby-route';
import { primaryPhotoQuestionId } from '@core/models/game.models';
import type {
  AnswerValue,
  FastBioStateView,
  LobbyMode,
  LobbyPlayer,
  LobbyStateResponse,
  LobbyStatus,
  PlayerRole,
  QuestionDefinition,
  QuestionnaireSnapshot,
  RoundSubmissionInput,
  ThemeSelectionState,
  ZeroToHundredStateView,
} from '@core/models/game.models';
import { LobbyApiService } from './lobby-api.service';
import { filterBioAnswers, restoredPrimaryPhotoId } from './game-state-draft';
import { canHostExcludePlayer, PlayerExclusionController } from './player-exclusion';
import { RealtimeLobbyService } from './realtime-lobby.service';

interface StoredDraft {
  primaryPhotoId: string | null
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

function isQuestionAnswerValid(question: QuestionDefinition, answer: AnswerValue | undefined): boolean {
  if (question.type === 'integer_range') {
    return typeof answer === 'number' && Number.isInteger(answer) &&
      question.minimum !== undefined && question.maximum !== undefined &&
      answer >= question.minimum && answer <= question.maximum;
  }
  if (question.type === 'single_choice' || question.type === 'binary_choice') {
    return typeof answer === 'string' && question.options?.some((option) => option.id === answer) === true;
  }
  return answer !== undefined;
}

@Injectable({
  providedIn: 'root',
})
export class GameStateService {
  private readonly lobbyApi = inject(LobbyApiService);
  private readonly realtime = inject(RealtimeLobbyService);
  private readonly router = inject(Router);
  private readonly storedSessionState = signal<StoredPlayerSession | null>(this.readStoredSession());
  private readonly primaryPhotoIdState = signal<string | null>(null);
  private readonly taglineState = signal('');
  private readonly answerByQuestionIdState = signal<Record<string, AnswerValue>>({});
  private readonly bioAnswerByCategoryIdState = signal<Record<string, string>>({});
  private readonly loverQuestionIdState = signal<string | null>(null);
  private readonly errorMessageState = signal<string | null>(null);
  private readonly photoUrlByIdState = signal<Readonly<Record<string, string>>>({});
  private readonly loadingPhotoIdSet = new Set<string>();
  private readonly fastBioPhotoUrlByIdState = signal<Readonly<Record<string, string>>>({});
  private readonly loadingFastBioPhotoIdSet = new Set<string>();
  private readonly playerExclusion = new PlayerExclusionController();
  private restoredDraftKey = '';

  readonly state = this.realtime.state;
  readonly connectionStatus = this.realtime.connectionStatus;
  readonly isRealtimeConnected = computed(() => this.connectionStatus() === 'connected');
  readonly lobbyCode = computed(() => this.state()?.code ?? this.storedSessionState()?.lobbyCode ?? '');
  readonly lobbyStatus = computed<LobbyStatus>(() => this.state()?.status ?? 'waiting_for_players');
  readonly mode = computed<LobbyMode>(() => this.state()?.mode ?? 'classic');
  readonly isFastBioMode = computed(() => this.mode() === 'fast_bio');
  readonly isZeroToHundredMode = computed(() => this.mode() === 'zero_to_100');
  readonly isUncappedMode = computed(() => this.isFastBioMode() || this.isZeroToHundredMode());
  readonly fastBioGame = computed<FastBioStateView | null>(() => this.state()?.fastBioGame ?? null);
  readonly zeroToHundredGame = computed<ZeroToHundredStateView | null>(() => this.state()?.zeroToHundredGame ?? null);
  /** Shared theme-collection-and-ranking state, whichever uncapped mode is active. */
  readonly themeSelectionState = computed<ThemeSelectionState | null>(() => this.fastBioGame() ?? this.zeroToHundredGame());
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
  readonly primaryPhotoId = this.primaryPhotoIdState.asReadonly();
  readonly tagline = this.taglineState.asReadonly();
  readonly answerByQuestionId = this.answerByQuestionIdState.asReadonly();
  readonly bioAnswerByCategoryId = this.bioAnswerByCategoryIdState.asReadonly();
  readonly loverQuestionId = this.loverQuestionIdState.asReadonly();
  readonly isProfileLocked = computed(() => this.game()?.submitted ?? false);
  readonly isQuestionnaireLoading = computed(() => this.state() === null);
  readonly errorMessage = this.errorMessageState.asReadonly();
  readonly firstIncompleteDraftStep = computed<number | null>(() => {
    if (!this.primaryPhotoId() || !this.subjectPlayer().photoIds.includes(this.primaryPhotoId() ?? '')) {
      return 0;
    }
    let stepIndex = 1;
    if (this.role() !== 'lover') {
      const taglineLength = this.tagline().trim().length;
      if (taglineLength < 1 || taglineLength > 100) {
        return stepIndex;
      }
      stepIndex++;
    }
    const bioAnswers = this.bioAnswerByCategoryId();
    for (const field of this.profileFieldList()) {
      if (!field.options.some((option) => option.id === bioAnswers[field.id])) {
        return stepIndex;
      }
      stepIndex++;
    }
    const questionAnswers = this.answerByQuestionId();
    for (const question of this.activeQuestionList()) {
      if (!isQuestionAnswerValid(question, questionAnswers[question.id])) {
        return stepIndex;
      }
      stepIndex++;
    }
    return null;
  });
  readonly isDraftComplete = computed(() => this.firstIncompleteDraftStep() === null);
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
  readonly canStartFastBioGame = computed(() => {
    const players = this.playerList();
    return this.isFastBioMode() &&
      this.fastBioGame() === null &&
      this.isHost() &&
      this.isRealtimeConnected() &&
      players.length >= 2 &&
      players.every((player) => player.connected);
  });
  readonly canStartZeroToHundredGame = computed(() => {
    const players = this.playerList();
    return this.isZeroToHundredMode() &&
      this.zeroToHundredGame() === null &&
      this.isHost() &&
      this.isRealtimeConnected() &&
      players.length >= 3 &&
      players.every((player) => player.connected);
  });

  constructor() {
    effect(() => {
      const state = this.state();
      if (state) {
        this.pruneRemovedPhotoUrls(state);
        this.pruneRemovedFastBioPhotoUrls(state);
        void this.loadMissingPhotos(state);
        void this.loadMissingFastBioPhoto(state);
        void this.synchronizeRoute(state);
      }
    });
  }

  async createLobby(
    displayName: string,
    mode: LobbyMode,
    gameConfigId: string,
    maxPlayers?: number,
  ): Promise<string> {
    this.errorMessageState.set(null);
    try {
      const session = await this.lobbyApi.create({
        displayName,
        mode,
        maxPlayers: mode === 'classic' ? (maxPlayers ?? 5) : undefined,
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

  async addPhoto(photo: File): Promise<void> {
    await this.runStateCommand(
      (code, token) => this.lobbyApi.addPhoto(code, token, photo),
      'Cette photo n’a pas pu être envoyée.',
    );
  }

  async replacePhoto(position: number, photo: File): Promise<void> {
    await this.runStateCommand(
      (code, token) => this.lobbyApi.replacePhoto(code, token, position, photo),
      'Cette photo n’a pas pu être remplacée.',
    );
  }

  async deletePhoto(position: number): Promise<void> {
    await this.runStateCommand(
      (code, token) => this.lobbyApi.deletePhoto(code, token, position),
      'Cette photo n’a pas pu être supprimée.',
    );
  }

  async completePhotos(): Promise<void> {
    await this.runStateCommand(
      (code, token) => this.lobbyApi.completePhotos(code, token),
      'Tes photos n’ont pas pu être validées.',
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

  async startFastBioGame(): Promise<void> {
    if (!this.canStartFastBioGame()) {
      return;
    }
    await this.runStateCommand(
      (code, token) => this.lobbyApi.startFastBio(code, token),
      'La partie n’a pas pu démarrer.',
    );
  }

  /** Submits (or passes on, via an empty string) a theme proposal for whichever uncapped mode is active. */
  async submitTheme(theme: string): Promise<void> {
    await this.runStateCommand(
      (code, token) => this.isZeroToHundredMode()
        ? this.lobbyApi.submitZeroToHundredTheme(code, token, theme)
        : this.lobbyApi.submitFastBioTheme(code, token, theme),
      'Ta proposition de thème n’a pas pu être envoyée.',
    );
  }

  /** Submits the player's ranking of theme candidates for whichever uncapped mode is active. */
  async rankThemes(ranking: readonly string[]): Promise<void> {
    await this.runStateCommand(
      (code, token) => this.isZeroToHundredMode()
        ? this.lobbyApi.rankZeroToHundredThemes(code, token, ranking)
        : this.lobbyApi.rankFastBioThemes(code, token, ranking),
      'Ton classement n’a pas pu être envoyé.',
    );
  }

  async startZeroToHundredGame(): Promise<void> {
    if (!this.canStartZeroToHundredGame()) {
      return;
    }
    await this.runStateCommand(
      (code, token) => this.lobbyApi.startZeroToHundred(code, token),
      'La partie n’a pas pu démarrer.',
    );
  }

  async submitZeroToHundredGuesses(positions: Readonly<Record<string, number>>): Promise<void> {
    await this.runStateCommand(
      (code, token) => this.lobbyApi.submitZeroToHundredGuesses(code, token, positions),
      'Tes positions n’ont pas pu être envoyées.',
    );
  }

  async startNextZeroToHundredRound(): Promise<void> {
    await this.runStateCommand(
      (code, token) => this.lobbyApi.startNextZeroToHundredRound(code, token),
      'La manche suivante n’a pas pu démarrer.',
    );
  }

  async replayZeroToHundred(): Promise<void> {
    await this.runStateCommand(
      (code, token) => this.lobbyApi.replayZeroToHundred(code, token),
      'Impossible de relancer une nouvelle manche.',
    );
  }

  async submitFastBioProposal(photo: File, bio: string): Promise<void> {
    await this.runStateCommand(
      (code, token) => this.lobbyApi.submitFastBioProposal(code, token, photo, bio),
      'Ta proposition n’a pas pu être envoyée.',
    );
  }

  async advanceFastBioReview(direction: 'next' | 'previous'): Promise<void> {
    await this.runStateCommand(
      (code, token) => this.lobbyApi.advanceFastBioReview(code, token, direction),
      'Impossible de changer de proposition.',
    );
  }

  async reactToFastBioProposal(proposalId: string, emoji: string): Promise<void> {
    const session = this.storedSessionState();
    if (!session) {
      return;
    }
    try {
      await this.lobbyApi.reactToFastBioProposal(session.lobbyCode, session.reconnectToken, proposalId, emoji);
    } catch (error: unknown) {
      this.captureError(error, 'Ta réaction n’a pas pu être envoyée.');
    }
  }

  async replayFastBio(): Promise<void> {
    await this.runStateCommand(
      (code, token) => this.lobbyApi.replayFastBio(code, token),
      'Impossible de relancer une nouvelle manche.',
    );
  }

  fastBioPhotoUrl(photoId: string | undefined): string | null {
    return photoId ? this.fastBioPhotoUrlByIdState()[photoId] ?? null : null;
  }

  saveTagline(tagline: string): void {
    this.taglineState.set(tagline);
    this.persistDraft();
  }

  savePrimaryPhoto(photoId: string): void {
    if (!this.subjectPlayer().photoIds.includes(photoId)) {
      return;
    }
    this.primaryPhotoIdState.set(photoId);
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
      bioAnswers: filterBioAnswers(this.profileFieldList(), this.bioAnswerByCategoryId()),
      questionAnswers: {
        ...this.answerByQuestionId(),
        [primaryPhotoQuestionId]: this.primaryPhotoId() ?? '',
      },
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
    const target = this.playerList().find((player) => player.id === playerId);
    if (!target || !canHostExcludePlayer(this.isHost(), target) || !this.isRealtimeConnected()) {
      return;
    }
    await this.playerExclusion.run(playerId, async () => {
      await this.runStateCommand(
        (code, token) => this.lobbyApi.excludePlayer(code, token, playerId),
        'Impossible d’exclure ce joueur. Vérifie ta connexion puis réessaie.',
      );
    });
  }

  canExcludePlayer(player: LobbyPlayer): boolean {
    return canHostExcludePlayer(this.isHost(), player);
  }

  isExcludingPlayer(playerId: string): boolean {
    return this.playerExclusion.isPending(playerId);
  }

  /** Clears the current error message, e.g. once the global error toast has been read/dismissed. */
  clearError(): void {
    this.errorMessageState.set(null);
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
    for (const url of Object.values(this.fastBioPhotoUrlByIdState())) {
      URL.revokeObjectURL(url);
    }
    this.fastBioPhotoUrlByIdState.set({});
    this.primaryPhotoIdState.set(null);
    this.taglineState.set('');
    this.answerByQuestionIdState.set({});
    this.bioAnswerByCategoryIdState.set({});
    this.loverQuestionIdState.set(null);
    this.errorMessageState.set(null);
    this.playerExclusion.clear();
    this.restoredDraftKey = '';
  }

  async leaveGame(): Promise<void> {
    const session = this.storedSessionState();
    try {
      if (session) {
        await this.lobbyApi.leave(session.lobbyCode, session.reconnectToken);
      }
    } catch {
      // Closing the realtime connection lets the automatic inactivity cleanup take over.
    } finally {
      this.resetGame();
    }
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
      primaryPhotoId: null,
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
    const storedQuestionAnswers = draft.answerByQuestionId ?? {};
    const storedBioAnswers = draft.bioAnswerByCategoryId ?? {};
    const validQuestionIDs = new Set(this.activeQuestionList().map((question) => question.id));
    this.primaryPhotoIdState.set(restoredPrimaryPhotoId(
      draft.primaryPhotoId,
      storedQuestionAnswers,
      this.subjectPlayer().photoIds,
    ));
    this.taglineState.set(typeof draft.tagline === 'string' ? draft.tagline : '');
    this.answerByQuestionIdState.set(Object.fromEntries(
      Object.entries(storedQuestionAnswers).filter(([questionId]) => validQuestionIDs.has(questionId)),
    ));
    this.bioAnswerByCategoryIdState.set(filterBioAnswers(this.profileFieldList(), storedBioAnswers));
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
      primaryPhotoId: this.primaryPhotoId(),
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

  private async loadMissingFastBioPhoto(state: LobbyStateResponse): Promise<void> {
    const session = this.storedSessionState();
    const photoId = state.fastBioGame?.currentProposal?.photoId;
    if (!session || session.lobbyCode !== state.code || !photoId) {
      return;
    }
    if (this.fastBioPhotoUrlByIdState()[photoId] || this.loadingFastBioPhotoIdSet.has(photoId)) {
      return;
    }
    this.loadingFastBioPhotoIdSet.add(photoId);
    try {
      const blob = await this.lobbyApi.getFastBioProposalPhoto(state.code, session.reconnectToken, photoId);
      const url = URL.createObjectURL(blob);
      this.fastBioPhotoUrlByIdState.update((current) => ({ ...current, [photoId]: url }));
    } catch (error: unknown) {
      console.error('Unable to load a Fast Bio proposal photo', error);
    } finally {
      this.loadingFastBioPhotoIdSet.delete(photoId);
    }
  }

  private pruneRemovedFastBioPhotoUrls(state: LobbyStateResponse): void {
    const activePhotoId = state.fastBioGame?.currentProposal?.photoId;
    const currentUrlById = this.fastBioPhotoUrlByIdState();
    const nextUrlById: Record<string, string> = {};
    let hasRemovedPhoto = false;
    for (const [photoId, url] of Object.entries(currentUrlById)) {
      if (photoId === activePhotoId) {
        nextUrlById[photoId] = url;
        continue;
      }
      URL.revokeObjectURL(url);
      hasRemovedPhoto = true;
    }
    if (hasRemovedPhoto) {
      this.fastBioPhotoUrlByIdState.set(nextUrlById);
    }
  }

  private pruneRemovedPhotoUrls(state: LobbyStateResponse): void {
    const activePhotoIdSet = new Set(state.players.flatMap((player) => player.photoIds));
    const currentUrlById = this.photoUrlByIdState();
    const nextUrlById: Record<string, string> = {};
    let hasRemovedPhoto = false;
    for (const [photoId, url] of Object.entries(currentUrlById)) {
      if (activePhotoIdSet.has(photoId)) {
        nextUrlById[photoId] = url;
        continue;
      }
      URL.revokeObjectURL(url);
      hasRemovedPhoto = true;
    }
    if (hasRemovedPhoto) {
      this.photoUrlByIdState.set(nextUrlById);
    }
  }

  private async synchronizeRoute(state: LobbyStateResponse): Promise<void> {
    const pendingNavigation = this.router.currentNavigation();
    const pendingURL = (pendingNavigation?.finalUrl ?? pendingNavigation?.extractedUrl)?.toString();
    const currentURL = activeNavigationUrl(this.router.url, pendingURL);
    if (!currentURL.startsWith('/lobby/') && !currentURL.startsWith('/game/')) {
      return;
    }
    if (state.mode === 'fast_bio') {
      await this.synchronizeFastBioRoute(state, currentURL);
      return;
    }
    if (state.mode === 'zero_to_100') {
      await this.synchronizeZeroToHundredRoute(state, currentURL);
      return;
    }
    if (
      !state.game || !state.status || state.status !== 'in_game' && state.status !== 'completed' ||
      !state.game.isParticipant
    ) {
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

  private async synchronizeFastBioRoute(state: LobbyStateResponse, currentURL: string): Promise<void> {
    const lobbyURL = `/lobby/${state.code}`;
    const fastBio = state.fastBioGame;
    if (!fastBio || state.status !== 'in_game') {
      if (currentURL !== lobbyURL && currentURL.startsWith('/game/')) {
        await this.router.navigateByUrl(lobbyURL);
      }
      return;
    }
    let destination = lobbyURL;
    if (fastBio.phase === 'collecting_themes' || fastBio.phase === 'ranking_themes') {
      destination = `/game/${state.code}/theme-selection`;
    } else if (fastBio.phase === 'playing' && fastBio.roundNumber) {
      const roundBase = `/game/${state.code}/fast-bio/${fastBio.roundNumber}`;
      destination = fastBio.roundPhase === 'submitting' ? `${roundBase}/assignment` : `${roundBase}/review`;
    } else if (fastBio.phase === 'completed') {
      destination = `/game/${state.code}/fast-bio/final`;
    }
    if (currentURL !== destination) {
      await this.router.navigateByUrl(destination);
    }
  }

  private async synchronizeZeroToHundredRoute(state: LobbyStateResponse, currentURL: string): Promise<void> {
    const lobbyURL = `/lobby/${state.code}`;
    const zeroToHundred = state.zeroToHundredGame;
    if (!zeroToHundred || state.status !== 'in_game') {
      if (currentURL !== lobbyURL && currentURL.startsWith('/game/')) {
        await this.router.navigateByUrl(lobbyURL);
      }
      return;
    }
    let destination = lobbyURL;
    if (zeroToHundred.phase === 'collecting_themes' || zeroToHundred.phase === 'ranking_themes') {
      destination = `/game/${state.code}/theme-selection`;
    } else if (zeroToHundred.phase === 'playing' && zeroToHundred.roundNumber) {
      const roundBase = `/game/${state.code}/zero-to-100/${zeroToHundred.roundNumber}`;
      destination = zeroToHundred.roundPhase === 'guessing' ? `${roundBase}/guess` : `${roundBase}/results`;
    } else if (zeroToHundred.phase === 'completed') {
      destination = `/game/${state.code}/zero-to-100/final`;
    }
    if (currentURL !== destination) {
      await this.router.navigateByUrl(destination);
    }
  }

  private captureError(error: unknown, fallbackMessage: string): void {
    if (error instanceof HttpErrorResponse) {
      const serverError = (error.error as {
        error?: { code?: string; message?: string }
      } | null)?.error;
      const errorMessageByCode: Readonly<Record<string, string>> = {
        photo_too_large: 'Cette photo est trop volumineuse, même après compression.',
        photos_too_large: 'Les photos sont trop volumineuses pour être envoyées.',
        unsupported_photo: 'Le format reçu n’est pas pris en charge.',
        invalid_photo: 'Cette image est illisible ou ses dimensions ne sont pas valides.',
        invalid_photo_count: 'Sélectionne une seule photo à la fois.',
        invalid_photo_position: 'Cet emplacement photo n’est pas valide.',
        photo_limit_reached: 'Les quatre emplacements sont déjà remplis.',
        photo_not_found: 'Cette photo n’existe plus. La liste va être actualisée.',
        incomplete_photos: 'Ajoute quatre photos avant de valider.',
        photos_locked: 'Tes photos ont déjà été validées et ne peuvent plus être modifiées.',
        lobby_already_started: 'Cette partie est terminée.',
        display_name_taken: 'Ce pseudo est déjà utilisé par un joueur actuellement en ligne.',
        player_online: 'Ce joueur vient de se reconnecter et ne peut plus être exclu.',
        host_required: 'Seul l’hôte du lobby peut exclure un joueur.',
        player_not_found: 'Ce joueur ne fait plus partie du lobby.',
        cannot_exclude_self: 'Tu ne peux pas t’exclure toi-même du lobby.',
        cannot_exclude_host: 'L’hôte du lobby ne peut pas être exclu.',
        invalid_player_count: 'Il n’y a pas assez de joueurs actifs pour lancer la partie.',
        players_offline: 'Tout le monde doit être connecté avant de lancer la partie.',
        wrong_mode: 'Ce lobby n’est pas configuré en mode Fast Bio.',
        invalid_theme: 'Ce thème est trop long.',
        themes_locked: 'La collecte des thèmes est terminée.',
        ranking_locked: 'Le classement des thèmes n’est pas ouvert.',
        invalid_ranking: 'Ton classement doit contenir chaque thème proposé une seule fois.',
        fast_bio_not_started: 'La partie Fast Bio n’a pas encore démarré.',
        fast_bio_not_playing: 'Aucune manche Fast Bio n’est ouverte pour le moment.',
        fast_bio_round_locked: 'Cette manche n’accepte plus de propositions.',
        not_assigned: 'Tu n’as pas de cible pour cette manche.',
        already_submitted: 'Tu as déjà envoyé ta proposition pour cette manche.',
        invalid_bio: 'La bio doit contenir entre 1 et 280 caractères.',
        invalid_direction: 'Navigation invalide dans la revue des propositions.',
        invalid_emoji: 'Choisis un des quatre emojis proposés.',
        cannot_react_to_own_proposal: 'Tu ne peux pas réagir à ta propre proposition.',
        proposal_not_active: 'Cette proposition n’est plus celle affichée par l’hôte.',
        proposal_not_found: 'Cette proposition n’existe plus.',
        fast_bio_in_progress: 'Le cycle Fast Bio en cours n’est pas encore terminé.',
        invalid_positions: 'Place chaque nominé entre 0 et 100.',
        zero_to_100_not_started: 'La partie 0 à 100 n’a pas encore démarré.',
        zero_to_100_not_playing: 'Aucune manche 0 à 100 n’est ouverte pour le moment.',
        zero_to_100_round_locked: 'Cette manche n’accepte plus de réponses.',
        zero_to_100_in_progress: 'Le cycle 0 à 100 en cours n’est pas encore terminé.',
      };
      this.errorMessageState.set(
        serverError?.code ? errorMessageByCode[serverError.code] ?? serverError.message ?? fallbackMessage :
          serverError?.message ?? fallbackMessage,
      );
      return;
    }
    this.errorMessageState.set(fallbackMessage);
  }
}
