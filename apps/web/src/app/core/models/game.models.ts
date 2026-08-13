export type LobbyStatus =
  | 'waiting_for_players'
  | 'preparing_photos'
  | 'ready_to_start'
  | 'in_game'
  | 'completed'
  | 'expired'

export type LobbyMode = 'classic' | 'fast_bio' | 'zero_to_100' | 'situation'

export type FastBioGamePhase = 'collecting_themes' | 'ranking_themes' | 'playing' | 'completed'

export type FastBioRoundPhase = 'submitting' | 'reviewing' | 'completed'

export const fastBioReactionEmojis = ['❤️', '😂', '😐', '🤮'] as const

export type FastBioReactionEmoji = typeof fastBioReactionEmojis[number]

export type ZeroToHundredGamePhase = 'collecting_themes' | 'ranking_themes' | 'playing' | 'completed'

export type ZeroToHundredRoundPhase = 'guessing' | 'results' | 'completed'

export type SituationGamePhase = 'collecting_themes' | 'ranking_themes' | 'playing' | 'completed'

export type SituationRoundPhase = 'proposing' | 'dueling' | 'revealing' | 'ranking' | 'results' | 'completed'

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
  players: readonly Omit<LobbyPlayer, 'isCurrentPlayer'>[]
  gameConfig: LobbyGameConfigSummary
  questionnaire: QuestionnaireSnapshot
  game?: GameStateView
  fastBioGame?: FastBioStateView
  zeroToHundredGame?: ZeroToHundredStateView
  situationGame?: SituationStateView
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
  leaderboard?: readonly FastBioLeaderboardEntryView[]
}

export interface ZeroToHundredNomineeView {
  playerId: string
  displayName: string
  isCurrentPlayer: boolean
}

export interface ZeroToHundredRevealEntryView {
  playerId: string
  displayName: string
  truePosition: number
  averagePosition: number
  myGuess?: number
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
  roundScore?: number
  leaderboard?: readonly FastBioLeaderboardEntryView[]
}

export interface SituationProposalView {
  id: string
  authorPlayerId?: string
  authorDisplayName?: string
  chosenPlayerId: string
  chosenDisplayName: string
  reason: string
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
  submitted: boolean
  submissionProgressCount?: number
  submissionProgressRequired?: number
  currentDuel?: SituationDuelView
  proposalCount?: number
  reviewIndex?: number
  isHostReview?: boolean
  currentProposal?: SituationProposalView
  rankingCandidates?: readonly SituationProposalView[]
  rankingSubmitted?: boolean
  roundScore?: number
  leaderboard?: readonly FastBioLeaderboardEntryView[]
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
