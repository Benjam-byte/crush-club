export interface LocalPhotoCandidate {
  id: string
  kind: 'local'
  source: string
  isObjectUrl: true
}

export interface DefaultPhotoCandidate {
  id: string
  kind: 'default'
  atlas: 'camille'
  avatarIndex: number
  isObjectUrl: false
}

export type PhotoCandidate = LocalPhotoCandidate | DefaultPhotoCandidate

export interface ProfileVersion {
  id: string
  authorName: string
  tagline: string
  bio: string
  matchPercentage: number
  avatarIndex: number
}

export interface BadgeDefinition {
  icon: string
  label: string
  owner: string
  tone: 'brand' | 'purple' | 'ink' | 'gold'
}

export type LobbyStatus =
  | 'waiting_for_players'
  | 'preparing_photos'
  | 'ready_to_start'
  | 'in_game'
  | 'completed'
  | 'expired'

export type PlayerReadyStatus = 'joining' | 'preparing_photos' | 'ready'

export type PlayerRole = 'lover' | 'cupid' | 'spectator'

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
  avatarIndex: number
  isHost: boolean
  isCurrentPlayer: boolean
  readyStatus: PlayerReadyStatus
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
  reconnectToken?: string
}

export interface BioCategory {
  id: string
  label: string
  optionList: readonly QuestionOption[]
}

export type ScoreStatus =
  | 'exact'
  | 'close'
  | 'wrong'
  | 'lover_success'
  | 'lover_failed'

export interface ScoreLine {
  id: string
  label: string
  officialAnswer: string
  predictedAnswer: string
  normalScore: number
  maximumScore: number
  finalScore: number
  status: ScoreStatus
  isLoverApplied: boolean
}

export interface LeaderboardEntry {
  id: string
  displayName: string
  avatarIndex: number
  score: number
  exactCount: number
  badge?: string
}
