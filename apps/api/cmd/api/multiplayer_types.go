package main

import (
	"encoding/json"
	"time"
)

const (
	minimumPlayerCount       = 2
	maximumPlayerCount       = 10
	reconnectGracePeriod     = 90 * time.Second
	maximumPhotoSizeBytes    = 7 << 20
	maximumPhotoDimension    = 4096
	bestTaglineBonus         = 10
	primaryPhotoQuestionID   = "__primary_photo__"
	primaryPhotoMaximumScore = 10

	lobbyModeClassic       = "classic"
	lobbyModeFastBio       = "fast_bio"
	lobbyModeZeroToHundred = "zero_to_100"

	// classicLobbyMinPlayers matches the system-wide floor (minimumPlayerCount)
	// rather than a stricter 3: the create-lobby UI only ever offers 3-5, this
	// is just a permissive backend safety net consistent with the rest of the
	// classic engine (which already treats 2 as a valid game size).
	classicLobbyMinPlayers  = minimumPlayerCount
	classicLobbyMaxPlayers  = 5
	fastBioRoundCount       = 3
	fastBioSubmissionWindow = 2 * time.Minute

	fastBioReactionHeart   = "❤️"
	fastBioReactionLaugh   = "😂"
	fastBioReactionNeutral = "😐"
	fastBioReactionSick    = "🤮"

	zeroToHundredRoundCount            = 3
	zeroToHundredGuessWindow           = 2 * time.Minute
	zeroToHundredNomineeCount          = 3
	zeroToHundredProximityMaximumScore = 10
	zeroToHundredOrderBonus            = 15
)

var fastBioReactionPoints = map[string]int{
	fastBioReactionHeart:   3,
	fastBioReactionLaugh:   2,
	fastBioReactionNeutral: 1,
	fastBioReactionSick:    0,
}

func defaultFastBioThemes() []string {
	return []string{
		"Mode mystérieux",
		"Mode aventurier",
		"Mode intello",
		"Mode glamour",
		"Mode sportif",
		"Mode artiste",
	}
}

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
	Revision          int64                   `json:"revision"`
	ServerTime        time.Time               `json:"serverTime"`
	Code              string                  `json:"code"`
	Status            string                  `json:"status"`
	Mode              string                  `json:"mode"`
	MaxPlayers        *int                    `json:"maxPlayers"`
	CurrentPlayerID   string                  `json:"currentPlayerId"`
	Players           []lobbyPlayerView       `json:"players"`
	GameConfig        lobbyGameConfigSummary  `json:"gameConfig"`
	Questionnaire     questionnaireSnapshot   `json:"questionnaire"`
	Game              *gameStateView          `json:"game,omitempty"`
	FastBioGame       *fastBioStateView       `json:"fastBioGame,omitempty"`
	ZeroToHundredGame *zeroToHundredStateView `json:"zeroToHundredGame,omitempty"`
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
	IsParticipant       bool                   `json:"isParticipant"`
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

type fastBioStateView struct {
	ID                 string                        `json:"id"`
	Phase              string                        `json:"phase"`
	ThemeSubmitted     bool                          `json:"themeSubmitted"`
	ThemeCandidates    []string                      `json:"themeCandidates,omitempty"`
	ThemeRanked        bool                          `json:"themeRanked"`
	SelectedThemes     []string                      `json:"selectedThemes,omitempty"`
	RoundNumber        int                           `json:"roundNumber,omitempty"`
	TotalRounds        int                           `json:"totalRounds,omitempty"`
	RoundPhase         string                        `json:"roundPhase,omitempty"`
	ThemeLabel         string                        `json:"themeLabel,omitempty"`
	SubmissionDeadline *time.Time                    `json:"submissionDeadline,omitempty"`
	TargetPlayerID     string                        `json:"targetPlayerId,omitempty"`
	TargetDisplayName  string                        `json:"targetDisplayName,omitempty"`
	Submitted          bool                          `json:"submitted"`
	ProposalCount      int                           `json:"proposalCount,omitempty"`
	ReviewIndex        int                           `json:"reviewIndex,omitempty"`
	IsHostReview       bool                          `json:"isHostReview,omitempty"`
	CurrentProposal    *fastBioProposalView          `json:"currentProposal,omitempty"`
	MyReactionEmoji    string                        `json:"myReactionEmoji,omitempty"`
	Leaderboard        []fastBioLeaderboardEntryView `json:"leaderboard,omitempty"`
}

type fastBioProposalView struct {
	ID                string                     `json:"id"`
	AuthorPlayerID    string                     `json:"authorPlayerId"`
	AuthorDisplayName string                     `json:"authorDisplayName"`
	TargetPlayerID    string                     `json:"targetPlayerId"`
	TargetDisplayName string                     `json:"targetDisplayName"`
	PhotoID           string                     `json:"photoId"`
	Bio               string                     `json:"bio"`
	Reactions         []fastBioReactionCountView `json:"reactions"`
	TotalPoints       int                        `json:"totalPoints"`
}

type fastBioReactionCountView struct {
	Emoji string `json:"emoji"`
	Count int    `json:"count"`
}

type fastBioLeaderboardEntryView struct {
	PlayerID    string `json:"playerId"`
	DisplayName string `json:"displayName"`
	Score       int    `json:"score"`
	RoundScore  int    `json:"roundScore"`
}

type zeroToHundredStateView struct {
	ID                 string                         `json:"id"`
	Phase              string                         `json:"phase"`
	ThemeSubmitted     bool                           `json:"themeSubmitted"`
	ThemeCandidates    []string                       `json:"themeCandidates,omitempty"`
	ThemeRanked        bool                           `json:"themeRanked"`
	SelectedThemes     []string                       `json:"selectedThemes,omitempty"`
	RoundNumber        int                            `json:"roundNumber,omitempty"`
	TotalRounds        int                            `json:"totalRounds,omitempty"`
	RoundPhase         string                         `json:"roundPhase,omitempty"`
	ThemeLabel         string                         `json:"themeLabel,omitempty"`
	SubmissionDeadline *time.Time                     `json:"submissionDeadline,omitempty"`
	Nominees           []zeroToHundredNomineeView     `json:"nominees,omitempty"`
	IsNominee          bool                           `json:"isNominee,omitempty"`
	Submitted          bool                           `json:"submitted"`
	Reveal             []zeroToHundredRevealEntryView `json:"reveal,omitempty"`
	RoundScore         int                            `json:"roundScore,omitempty"`
	Leaderboard        []fastBioLeaderboardEntryView  `json:"leaderboard,omitempty"`
}

type zeroToHundredNomineeView struct {
	PlayerID        string `json:"playerId"`
	DisplayName     string `json:"displayName"`
	IsCurrentPlayer bool   `json:"isCurrentPlayer"`
}

type zeroToHundredRevealEntryView struct {
	PlayerID        string  `json:"playerId"`
	DisplayName     string  `json:"displayName"`
	TruePosition    int     `json:"truePosition"`
	AveragePosition float64 `json:"averagePosition"`
	MyGuess         *int    `json:"myGuess,omitempty"`
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
