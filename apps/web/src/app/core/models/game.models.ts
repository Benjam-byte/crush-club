export type LobbyStatus =
  | 'waiting_for_players'
  | 'preparing_photos'
  | 'ready_to_start'
  | 'in_game'
  | 'completed'
  | 'expired'

export type LobbyMode = 'classic' | 'fast_bio' | 'zero_to_100' | 'situation' | 'wavelength' | 'secret_ranking'

export type FastBioGamePhase = 'collecting_themes' | 'ranking_themes' | 'playing' | 'completed'

export type FastBioRoundPhase = 'submitting' | 'reviewing' | 'completed'

export const fastBioReactionEmojis = ['❤️', '😂', '😐', '🤮'] as const

export type FastBioReactionEmoji = typeof fastBioReactionEmojis[number]

export const fastBioReactionPoints: Readonly<Record<FastBioReactionEmoji, number>> = {
  '❤️': 3,
  '😂': 2,
  '😐': 1,
  '🤮': 0,
}

export type ZeroToHundredGamePhase = 'collecting_themes' | 'ranking_themes' | 'playing' | 'completed'

export type ZeroToHundredRoundPhase = 'guessing' | 'results' | 'completed'

export type SituationGamePhase = 'collecting_themes' | 'ranking_themes' | 'playing' | 'completed'

export type SituationRoundPhase = 'proposing' | 'dueling' | 'ranking' | 'results' | 'completed'

export type WavelengthGamePhase = 'collecting_themes' | 'ranking_themes' | 'playing' | 'completed'

export type WavelengthRoundPhase = 'clue' | 'guessing' | 'results' | 'completed'

export type SecretRankingGamePhase = 'collecting_themes' | 'ranking_themes' | 'playing' | 'completed'

/** secret (bets) → placing (phones numbered on the table) → revealing (domino) → results (debrief). */
export type SecretRankingRoundPhase = 'secret' | 'placing' | 'revealing' | 'results' | 'completed'

/** Shared shape of the theme-collection-and-ranking step, common to every mode that uses it. */
export interface ThemeSelectionState {
  phase: string
  themeSubmitted: boolean
  themeCandidates?: readonly string[]
  themeRanked: boolean
  /** Deadline for whichever theme sub-phase is currently active (submission or ranking). */
  themeDeadline?: string
  /** How many active players have completed the current theme sub-phase so far. */
  themeProgressCount?: number
  /** How many active players are required to complete the current theme sub-phase. */
  themeProgressRequired?: number
}

export type PlayerReadyStatus = 'joining' | 'preparing_photos' | 'ready'

export type PlayerRole = 'lover' | 'cupid'

export type QuestionType =
  | 'single_choice'
  | 'binary_choice'
  | 'multi_choice'
  | 'ranked_choice'
  | 'integer_range'
  | 'number'
  | 'color_choice'
  | 'short_text'

export type AnswerValue = string | number | readonly string[]

export const primaryPhotoQuestionId = '__primary_photo__';

export type PlayerAdmission = 'admitted' | 'pending'

export interface LobbyPlayer {
  id: string
  displayName: string
  isHost: boolean
  isCurrentPlayer: boolean
  readyStatus: PlayerReadyStatus
  connected: boolean
  disconnectedAt?: string
  reconnectDeadline?: string
  canExclude: boolean
  photoIds: readonly string[]
  joinedAt: string
}

export interface QuestionOption {
  id: string
  label: string
  hint?: string
}

export interface QuestionDefinition {
  id: string
  kind?: GameConfigKind
  type: QuestionType
  label: string
  description?: string
  maximumScore: number
  loverEligible: boolean
  options?: readonly QuestionOption[]
  minimum?: number
  maximum?: number
  minimumLabel?: string
  maximumLabel?: string
}

export type GameConfigKind = 'system' | 'personal'

export interface GameConfig {
  id: string
  name: string
  kind: GameConfigKind
  isPublic: boolean
  isOwner: boolean
  version: number
  questionIds: readonly string[]
  questions: readonly QuestionDefinition[]
  createdAt: string
  updatedAt: string
}

export interface GameConfigQuestionInput {
  questionId?: string
  id?: string
  label?: string
  type?: 'single_choice' | 'binary_choice' | 'integer_range'
  options?: readonly string[]
  minimum?: number
  maximum?: number
}

export interface QuestionnaireSnapshot {
  sourceConfigId: string
  sourceVersion: number
  name: string
  kind?: GameConfigKind
  questions: readonly QuestionDefinition[]
  profileFields: readonly ProfileFieldDefinition[]
}

export interface ProfileFieldDefinition {
  id: string
  label: string
  options: readonly QuestionOption[]
}

export interface LobbyGameConfigSummary {
  id: string
  name: string
  version: number
  questionCount: number
}

export interface LobbyResponse {
  code: string
  status: LobbyStatus
  mode: LobbyMode
  maxPlayers: number | null
  gameConfig: LobbyGameConfigSummary
}

export type GamePhase =
  | 'collecting_submissions'
  | 'reveal_and_vote'
  | 'round_results'
  | 'between_rounds'
  | 'completed'

export interface RoundSubmissionView {
  id: string
  playerId?: string
  authorName?: string
  tagline?: string
  bioAnswers: Readonly<Record<string, string>>
  questionAnswers: Readonly<Record<string, AnswerValue>>
  loverQuestionId?: string
  submittedAt: string
}

export interface ScoreLineView {
  id: string
  label: string
  officialAnswer: AnswerValue
  predictedAnswer: AnswerValue
  baseScore: number
  maximumScore: number
  finalScore: number
  exact: boolean
  isLoverApplied: boolean
}

export interface RoundResultView {
  playerId: string
  displayName: string
  baseScore: number
  loverAdjustment: number
  taglineBonus: number
  totalScore: number
  exactCount: number
  scoreLines: readonly ScoreLineView[]
}

export interface LeaderboardEntryView {
  playerId: string
  displayName: string
  score: number
  roundScore: number
  exactCount: number
  taglineBonusCount: number
}

export interface GameStateView {
  id: string
  phase: GamePhase
  roundNumber: number
  totalRounds: number
  role: 'subject' | 'cupid'
  isParticipant: boolean
  subjectPlayerId: string
  nextSubjectPlayerId?: string
  submitted: boolean
  submittedCount: number
  requiredCount: number
  officialSubmission?: RoundSubmissionView
  submissions?: readonly RoundSubmissionView[]
  roundResults?: readonly RoundResultView[]
  leaderboard?: readonly LeaderboardEntryView[]
}

export interface LobbyStateResponse {
  revision: number
  serverTime: string
  code: string
  status: LobbyStatus
  mode: LobbyMode
  maxPlayers: number | null
  currentPlayerId: string
  /** "pending" while the current player waits for the host to let them into the game in progress. */
  currentPlayerAdmission?: PlayerAdmission
  /** Admitted players only; see pendingPlayers. */
  players: readonly Omit<LobbyPlayer, 'isCurrentPlayer'>[]
  /** Players who joined the game in progress and wait for the host's approval. */
  pendingPlayers?: readonly Omit<LobbyPlayer, 'isCurrentPlayer'>[]
  /** Active players the current phase is still waiting for. */
  awaitingPlayerIds?: readonly string[]
  gameConfig: LobbyGameConfigSummary
  questionnaire: QuestionnaireSnapshot
  game?: GameStateView
  fastBioGame?: FastBioStateView
  zeroToHundredGame?: ZeroToHundredStateView
  situationGame?: SituationStateView
  wavelengthGame?: WavelengthStateView
  secretRankingGame?: SecretRankingStateView
}

export interface FastBioProposalView {
  id: string
  authorPlayerId: string
  authorDisplayName: string
  targetPlayerId: string
  targetDisplayName: string
  photoId: string
  bio: string
  reactions: readonly FastBioReactionCountView[]
  totalPoints: number
}

export interface FastBioReactionCountView {
  emoji: string
  count: number
}

export interface FastBioLeaderboardEntryView {
  playerId: string
  displayName: string
  score: number
  roundScore: number
}

export interface FastBioStateView {
  id: string
  phase: FastBioGamePhase
  themeSubmitted: boolean
  themeCandidates?: readonly string[]
  themeRanked: boolean
  themeDeadline?: string
  themeProgressCount?: number
  themeProgressRequired?: number
  selectedThemes?: readonly string[]
  roundNumber?: number
  totalRounds?: number
  roundPhase?: FastBioRoundPhase
  themeLabel?: string
  submissionDeadline?: string
  targetPlayerId?: string
  targetDisplayName?: string
  submitted: boolean
  submissionProgressCount?: number
  submissionProgressRequired?: number
  proposalCount?: number
  reviewIndex?: number
  isHostReview?: boolean
  currentProposal?: FastBioProposalView
  myReactionEmoji?: string
  reactionProgressCount?: number
  reactionProgressRequired?: number
  leaderboard?: readonly FastBioLeaderboardEntryView[]
}

export interface ZeroToHundredNomineeView {
  playerId: string
  displayName: string
  isCurrentPlayer: boolean
}

/** One nominee of the round, in seat order (the seat picks their color). Values are on the 0 à 10 scale. */
export interface ZeroToHundredRevealEntryView {
  playerId: string
  displayName: string
  isCurrentPlayer: boolean
  /** Absent when the nominee did not place themselves. */
  truePosition?: number
  /** Absent when no other player placed this nominee. Rounded to one decimal. */
  averagePosition?: number
  /** ⅓ self-placement + ⅔ group average, rounded to one decimal. Absent when the nominee did not place themselves. */
  targetPosition?: number
  /** Step on the podium, 1 for the highest target; ties share a rank. Absent with targetPosition. */
  targetRank?: number
}

/** One nominee as a player placed them, among the nominees that have a target. */
export interface ZeroToHundredPlacementView {
  nomineeId: string
  position: number
  /** 1 for the highest position; ties share a rank. */
  rank: number
  /** Whether this rank matches the nominee's step on the podium. */
  correct: boolean
}

/** One player's answer for the round, sorted by the server from the best score down. */
export interface ZeroToHundredGuesserView {
  playerId: string
  displayName: string
  isCurrentPlayer: boolean
  /** Sorted by rank, then seat. */
  placements: readonly ZeroToHundredPlacementView[]
  correctCount: number
  perfectOrder: boolean
}

export interface ZeroToHundredLeaderboardEntryView {
  playerId: string
  displayName: string
  /** Well-placed nominees over every round. */
  score: number
  perfectOrders: number
}

export interface ZeroToHundredStateView extends ThemeSelectionState {
  id: string
  phase: ZeroToHundredGamePhase
  selectedThemes?: readonly string[]
  roundNumber?: number
  totalRounds?: number
  roundPhase?: ZeroToHundredRoundPhase
  themeLabel?: string
  submissionDeadline?: string
  nominees?: readonly ZeroToHundredNomineeView[]
  isNominee?: boolean
  submitted: boolean
  submissionProgressCount?: number
  submissionProgressRequired?: number
  reveal?: readonly ZeroToHundredRevealEntryView[]
  guessers?: readonly ZeroToHundredGuesserView[]
  leaderboard?: readonly ZeroToHundredLeaderboardEntryView[]
}

export interface SituationProposalView {
  id: string
  authorPlayerId?: string
  authorDisplayName?: string
  chosenPlayerId: string
  chosenDisplayName: string
  reason: string
}

export interface SituationGroupMemberView {
  playerId: string
  displayName: string
}

/** One finalist on the results screen: every member of its group earns its points. */
export interface SituationRoundResultView {
  proposal: SituationProposalView
  rank: number
  points: number
  members: readonly SituationGroupMemberView[]
  isMine: boolean
}

export interface SituationDuelView {
  id: string
  opponentDisplayName: string
  proposalA: SituationProposalView
  proposalB: SituationProposalView
  myVoteProposalId?: string
  opponentHasVoted: boolean
  deadline: string
}

export interface SituationStateView extends ThemeSelectionState {
  id: string
  phase: SituationGamePhase
  selectedThemes?: readonly string[]
  roundNumber?: number
  totalRounds?: number
  roundPhase?: SituationRoundPhase
  themeLabel?: string
  proposalDeadline?: string
  rankingDeadline?: string
  submitted: boolean
  submissionProgressCount?: number
  submissionProgressRequired?: number
  currentDuel?: SituationDuelView
  rankingCandidates?: readonly SituationProposalView[]
  rankingSubmitted?: boolean
  roundScore?: number
  roundResults?: readonly SituationRoundResultView[]
  leaderboard?: readonly FastBioLeaderboardEntryView[]
}

export interface WavelengthRevealEntryView {
  playerId: string
  displayName: string
  position: number
  points: number
}

/**
 * A vote selects up to three spectra, played one per round before the group votes again: `roundNumber`
 * is the round being played, or the first round the current vote prepares.
 */
export interface WavelengthStateView extends ThemeSelectionState {
  id: string
  phase: WavelengthGamePhase
  roundNumber?: number
  totalRounds?: number
  isLastRound?: boolean
  roundPhase?: WavelengthRoundPhase
  leftLabel?: string
  rightLabel?: string
  psychicPlayerId?: string
  psychicDisplayName?: string
  isPsychic?: boolean
  /** Only sent to the psychic until the round is scored. */
  target?: number
  phaseDeadline?: string
  submitted: boolean
  myPosition?: number
  submissionProgressCount?: number
  submissionProgressRequired?: number
  reveal?: readonly WavelengthRevealEntryView[]
  psychicPoints?: number
  roundScore?: number
  leaderboard?: readonly FastBioLeaderboardEntryView[]
  /** During a vote: how many of the ranked spectra the next rounds will play. */
  spectraToPlay?: number
  /** On a round's results: the spectrum already voted for the next round, absent when a new vote comes next. */
  nextSpectrumLabel?: string
}

/** One participant at the table. `colorIndex` is their phone's color, stable for the whole lobby. */
export interface SecretRankingSeatView {
  playerId: string
  displayName: string
  colorIndex: number
  position?: number
}

/** One phone of the line, in position order. Equal amounts share a true rank range. */
export interface SecretRankingRevealEntryView {
  playerId: string
  displayName: string
  colorIndex: number
  position: number
  amount: number
  wellPlaced: boolean
  trueRankMin: number
  trueRankMax: number
}

export interface SecretRankingRoundSummaryView {
  roundNumber: number
  themeLabel: string
  wellPlacedCount: number
  participantCount: number
  cancelled: boolean
  smallestPlayerIds: readonly string[]
  largestPlayerIds: readonly string[]
  smallestNames: readonly string[]
  largestNames: readonly string[]
}

/**
 * « Le classement secret ». `themeLabel` completes "Pour combien d'argent accepterais-tu de … ?". Nobody
 * else's amount is sent before the reveal: `myAmount` is the current player's own bet.
 */
export interface SecretRankingStateView extends ThemeSelectionState {
  id: string
  phase: SecretRankingGamePhase
  selectedThemes?: readonly string[]
  roundNumber?: number
  totalRounds?: number
  isLastRound?: boolean
  roundPhase?: SecretRankingRoundPhase
  themeLabel?: string
  secretDeadline?: string
  submitted: boolean
  submissionProgressCount?: number
  submissionProgressRequired?: number
  /** False for a player who did not bet this round: their phone is not on the table. */
  isParticipant: boolean
  myAmount?: number
  myPosition?: number
  participantCount: number
  numberedCount: number
  seats?: readonly SecretRankingSeatView[]
  /** Set while the phones are numbered 1..N without duplicates: the order locks at this time. */
  lockAt?: string
  /** Set when a departure wiped every number. */
  positionsResetAt?: string
  /** When phone #1 flips; phone #n flips (n − 1) × revealStepMs later. */
  revealAt?: string
  revealStepMs?: number
  /** When the reveal and its verdict are over: the debrief can open. */
  revealEndsAt?: string
  reveal?: readonly SecretRankingRevealEntryView[]
  wellPlacedCount: number
  cancelled?: boolean
  rounds?: readonly SecretRankingRoundSummaryView[]
}

export interface PlayerSessionResponse {
  reconnectToken: string
  state: LobbyStateResponse
}

export interface RoundSubmissionInput {
  tagline?: string
  bioAnswers: Readonly<Record<string, string>>
  questionAnswers: Readonly<Record<string, AnswerValue>>
  loverQuestionId?: string
}

export interface BioCategory {
  id: string
  label: string
  optionList: readonly QuestionOption[]
}
