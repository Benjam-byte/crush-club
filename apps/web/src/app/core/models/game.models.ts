export interface LocalPhotoCandidate {
  id: string
  kind: 'local'
  source: string
  isObjectUrl: true
}

export type PhotoCandidate = LocalPhotoCandidate

export type LobbyStatus =
  | 'waiting_for_players'
  | 'preparing_photos'
  | 'ready_to_start'
  | 'in_game'
  | 'completed'
  | 'expired'

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
  maxPlayers: number
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
  maxPlayers: number
  currentPlayerId: string
  players: readonly Omit<LobbyPlayer, 'isCurrentPlayer'>[]
  gameConfig: LobbyGameConfigSummary
  questionnaire: QuestionnaireSnapshot
  game?: GameStateView
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
