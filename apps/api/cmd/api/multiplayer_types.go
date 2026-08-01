package main

import (
	"encoding/json"
	"time"
)

const (
	minimumPlayerCount    = 2
	maximumPlayerCount    = 10
	reconnectGracePeriod  = 90 * time.Second
	maximumPhotoSizeBytes = 7 << 20
	maximumPhotoDimension = 4096
	bestTaglineBonus      = 10
)

type profileField struct {
	ID      string           `json:"id"`
	Label   string           `json:"label"`
	Options []questionOption `json:"options"`
}

func defaultProfileFields() []profileField {
	return []profileField{
		{ID: "quality", Label: "Qualité", Options: []questionOption{
			{ID: "attentive", Label: "Attentionné·e"},
			{ID: "funny", Label: "Drôle"},
			{ID: "ambitious", Label: "Ambitieux·se"},
			{ID: "spontaneous", Label: "Spontané·e"},
		}},
		{ID: "flaw", Label: "Petit défaut", Options: []questionOption{
			{ID: "stubborn", Label: "Têtu·e"},
			{ID: "impatient", Label: "Impatient·e"},
			{ID: "late", Label: "Toujours en retard"},
			{ID: "perfectionist", Label: "Perfectionniste"},
		}},
		{ID: "passion", Label: "Passion", Options: []questionOption{
			{ID: "music", Label: "Musique"},
			{ID: "cooking", Label: "Cuisine"},
			{ID: "travel", Label: "Voyage"},
			{ID: "photography", Label: "Photographie"},
		}},
		{ID: "lifestyle", Label: "Style de vie", Options: []questionOption{
			{ID: "adventurous", Label: "Aventureux·se"},
			{ID: "homebody", Label: "Casanièr·e"},
			{ID: "sporty", Label: "Sportif·ve"},
			{ID: "zen", Label: "Zen"},
		}},
		{ID: "intention", Label: "Ce qu’il ou elle recherche", Options: []questionOption{
			{ID: "complicity", Label: "De la complicité"},
			{ID: "serious", Label: "Une histoire sérieuse"},
			{ID: "see", Label: "Voir où ça mène"},
			{ID: "light", Label: "Une aventure légère"},
		}},
	}
}

type authenticatedPlayer struct {
	ID          string
	LobbyID     string
	Code        string
	DisplayName string
	IsHost      bool
}

type playerSessionResponse struct {
	ReconnectToken string             `json:"reconnectToken"`
	State          lobbyStateResponse `json:"state"`
}

type lobbyPlayerView struct {
	ID                string     `json:"id"`
	DisplayName       string     `json:"displayName"`
	IsHost            bool       `json:"isHost"`
	ReadyStatus       string     `json:"readyStatus"`
	Connected         bool       `json:"connected"`
	DisconnectedAt    *time.Time `json:"disconnectedAt,omitempty"`
	ReconnectDeadline *time.Time `json:"reconnectDeadline,omitempty"`
	CanExclude        bool       `json:"canExclude"`
	PhotoIDs          []string   `json:"photoIds"`
	JoinedAt          time.Time  `json:"joinedAt"`
}

type lobbyStateResponse struct {
	Revision        int64                  `json:"revision"`
	ServerTime      time.Time              `json:"serverTime"`
	Code            string                 `json:"code"`
	Status          string                 `json:"status"`
	MaxPlayers      int                    `json:"maxPlayers"`
	CurrentPlayerID string                 `json:"currentPlayerId"`
	Players         []lobbyPlayerView      `json:"players"`
	GameConfig      lobbyGameConfigSummary `json:"gameConfig"`
	Questionnaire   questionnaireSnapshot  `json:"questionnaire"`
	Game            *gameStateView         `json:"game,omitempty"`
}

type lobbyGameConfigSummary struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Version       int    `json:"version"`
	QuestionCount int    `json:"questionCount"`
}

type gameStateView struct {
	ID                  string                 `json:"id"`
	Phase               string                 `json:"phase"`
	RoundNumber         int                    `json:"roundNumber"`
	TotalRounds         int                    `json:"totalRounds"`
	Role                string                 `json:"role"`
	SubjectPlayerID     string                 `json:"subjectPlayerId"`
	NextSubjectPlayerID string                 `json:"nextSubjectPlayerId,omitempty"`
	Submitted           bool                   `json:"submitted"`
	SubmittedCount      int                    `json:"submittedCount"`
	RequiredCount       int                    `json:"requiredCount"`
	OfficialSubmission  *roundSubmissionView   `json:"officialSubmission,omitempty"`
	Submissions         []roundSubmissionView  `json:"submissions,omitempty"`
	RoundResults        []roundResultView      `json:"roundResults,omitempty"`
	Leaderboard         []leaderboardEntryView `json:"leaderboard,omitempty"`
}

type roundSubmissionView struct {
	ID              string         `json:"id"`
	PlayerID        string         `json:"playerId,omitempty"`
	AuthorName      string         `json:"authorName,omitempty"`
	Tagline         string         `json:"tagline,omitempty"`
	BioAnswers      map[string]any `json:"bioAnswers"`
	QuestionAnswers map[string]any `json:"questionAnswers"`
	LoverQuestionID string         `json:"loverQuestionId,omitempty"`
	SubmittedAt     time.Time      `json:"submittedAt"`
}

type roundResultView struct {
	PlayerID        string          `json:"playerId"`
	DisplayName     string          `json:"displayName"`
	BaseScore       int             `json:"baseScore"`
	LoverAdjustment int             `json:"loverAdjustment"`
	TaglineBonus    int             `json:"taglineBonus"`
	TotalScore      int             `json:"totalScore"`
	ExactCount      int             `json:"exactCount"`
	ScoreLines      json.RawMessage `json:"scoreLines"`
}

type leaderboardEntryView struct {
	PlayerID          string `json:"playerId"`
	DisplayName       string `json:"displayName"`
	Score             int    `json:"score"`
	RoundScore        int    `json:"roundScore"`
	ExactCount        int    `json:"exactCount"`
	TaglineBonusCount int    `json:"taglineBonusCount"`
}

type roundSubmissionInput struct {
	Tagline         string                     `json:"tagline,omitempty"`
	BioAnswers      map[string]string          `json:"bioAnswers"`
	QuestionAnswers map[string]json.RawMessage `json:"questionAnswers"`
	LoverQuestionID string                     `json:"loverQuestionId,omitempty"`
}

type roundVoteInput struct {
	SubmissionID string `json:"submissionId"`
}
