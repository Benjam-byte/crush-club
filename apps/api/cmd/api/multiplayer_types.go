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
	lobbyModeSituation     = "situation"
	lobbyModeWavelength    = "wavelength"
	lobbyModeSecretRanking = "secret_ranking"

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
	zeroToHundredNomineeCount          = 5 // bounded by zero_to_100_nominees_seat_check (seats 0-4)
	zeroToHundredMinPlayerCount        = 3
	zeroToHundredProximityMaximumScore = 10
	zeroToHundredOrderBonus            = 30
	// zeroToHundredSelfPlacementWeight is the share of a nominee's own placement
	// in their target; the group's average position makes up the rest.
	zeroToHundredSelfPlacementWeight = 1.0 / 3

	situationRoundCount     = 3
	situationProposalWindow = 2 * time.Minute
	situationDuelWindow     = 1 * time.Minute
	situationFinalistCount  = 4

	wavelengthClueWindow  = 90 * time.Second
	wavelengthGuessWindow = 90 * time.Second
	// The secret target is drawn far enough from the edges for the
	// bullseye band to always fit on the dial.
	wavelengthTargetMin = 3
	wavelengthTargetMax = 97
	// Scoring bands, by distance between a needle and the target.
	wavelengthBullseyeRadius = 3
	wavelengthInnerRadius    = 9
	wavelengthOuterRadius    = 15
	wavelengthBullseyePoints = 4
	wavelengthInnerPoints    = 3
	wavelengthOuterPoints    = 2
	wavelengthPoleMaxLength  = 40
	// wavelengthSpectraPerVote is how many of the ranked spectra a vote
	// keeps: they are played one per round, in ranking order, before the
	// group votes again.
	wavelengthSpectraPerVote = 3

	secretRankingMinPlayerCount = 3
	secretRankingMaxPlayerCount = maximumPlayerCount
	secretRankingRoundCount     = 3
	// secretRankingMinParticipants is how many bets a round needs: below it
	// the secret window starts over, and a round left with fewer at the table
	// is cancelled.
	secretRankingMinParticipants = 2
	secretRankingAmountWindow    = 2 * time.Minute
	secretRankingMaxAmount       = 1_000_000_000
	secretRankingThemeMaxLength  = 80
	// Once the phones are numbered 1..N without duplicates, the order locks
	// after secretRankingLockDelay unless someone taps a number again.
	secretRankingLockDelay = 5 * time.Second
	// The domino reveal: a "Silence…" lead, then one phone flips every step,
	// then the verdict plays for secretRankingRevealFinale. The debrief opens
	// once it is over.
	secretRankingRevealLead   = 2 * time.Second
	secretRankingRevealStep   = 1400 * time.Millisecond
	secretRankingRevealFinale = 2500 * time.Millisecond

	// The shared theme/situation selection step (collecting_themes, then
	// ranking_themes) of Fast Bio, 0 à 100, Situation and Wavelength gets the
	// full window the first time a lobby goes through it, and the shorter
	// repeat windows afterwards (Wavelength rounds 2+, replays): by then
	// everyone knows the drill. See themeStepWindow.
	themeStepFirstWindow      = 2 * time.Minute
	themeProposalRepeatWindow = 40 * time.Second
	themeRankingRepeatWindow  = 40 * time.Second
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
	// Admitted is false while a player who joined a game in progress waits
	// for the host's approval: they can follow the lobby but not play.
	Admitted bool
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
	Revision        int64     `json:"revision"`
	ServerTime      time.Time `json:"serverTime"`
	Code            string    `json:"code"`
	Status          string    `json:"status"`
	Mode            string    `json:"mode"`
	MaxPlayers      *int      `json:"maxPlayers"`
	CurrentPlayerID string    `json:"currentPlayerId"`
	// CurrentPlayerAdmission is "pending" while the current player waits for
	// the host to let them into the game in progress, "admitted" otherwise.
	CurrentPlayerAdmission string `json:"currentPlayerAdmission"`
	// Players only lists admitted players; PendingPlayers are those waiting
	// for the host's approval.
	Players        []lobbyPlayerView `json:"players"`
	PendingPlayers []lobbyPlayerView `json:"pendingPlayers"`
	// AwaitingPlayerIDs are the active players the current phase still waits
	// for, so everyone can see who is holding the game up.
	AwaitingPlayerIDs []string                `json:"awaitingPlayerIds"`
	GameConfig        lobbyGameConfigSummary  `json:"gameConfig"`
	Questionnaire     questionnaireSnapshot   `json:"questionnaire"`
	Game              *gameStateView          `json:"game,omitempty"`
	FastBioGame       *fastBioStateView       `json:"fastBioGame,omitempty"`
	ZeroToHundredGame *zeroToHundredStateView `json:"zeroToHundredGame,omitempty"`
	SituationGame     *situationStateView     `json:"situationGame,omitempty"`
	WavelengthGame    *wavelengthStateView    `json:"wavelengthGame,omitempty"`
	SecretRankingGame *secretRankingStateView `json:"secretRankingGame,omitempty"`
}

type lobbyGameConfigSummary struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Version       int    `json:"version"`
	QuestionCount int    `json:"questionCount"`
}

type gameStateView struct {
	// AwaitingPlayerIDs is lifted into lobbyStateResponse.AwaitingPlayerIDs.
	AwaitingPlayerIDs   []string               `json:"-"`
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
	// AwaitingPlayerIDs is lifted into lobbyStateResponse.AwaitingPlayerIDs.
	AwaitingPlayerIDs          []string                      `json:"-"`
	ID                         string                        `json:"id"`
	Phase                      string                        `json:"phase"`
	ThemeSubmitted             bool                          `json:"themeSubmitted"`
	ThemeCandidates            []string                      `json:"themeCandidates,omitempty"`
	ThemeRanked                bool                          `json:"themeRanked"`
	ThemeDeadline              *time.Time                    `json:"themeDeadline,omitempty"`
	ThemeProgressCount         int                           `json:"themeProgressCount"`
	ThemeProgressRequired      int                           `json:"themeProgressRequired"`
	SelectedThemes             []string                      `json:"selectedThemes,omitempty"`
	RoundNumber                int                           `json:"roundNumber,omitempty"`
	TotalRounds                int                           `json:"totalRounds,omitempty"`
	RoundPhase                 string                        `json:"roundPhase,omitempty"`
	ThemeLabel                 string                        `json:"themeLabel,omitempty"`
	SubmissionDeadline         *time.Time                    `json:"submissionDeadline,omitempty"`
	TargetPlayerID             string                        `json:"targetPlayerId,omitempty"`
	TargetDisplayName          string                        `json:"targetDisplayName,omitempty"`
	Submitted                  bool                          `json:"submitted"`
	SubmissionProgressCount    int                           `json:"submissionProgressCount"`
	SubmissionProgressRequired int                           `json:"submissionProgressRequired"`
	ProposalCount              int                           `json:"proposalCount,omitempty"`
	ReviewIndex                int                           `json:"reviewIndex,omitempty"`
	IsHostReview               bool                          `json:"isHostReview,omitempty"`
	CurrentProposal            *fastBioProposalView          `json:"currentProposal,omitempty"`
	MyReactionEmoji            string                        `json:"myReactionEmoji,omitempty"`
	ReactionProgressCount      int                           `json:"reactionProgressCount"`
	ReactionProgressRequired   int                           `json:"reactionProgressRequired"`
	Leaderboard                []fastBioLeaderboardEntryView `json:"leaderboard,omitempty"`
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
	// AwaitingPlayerIDs is lifted into lobbyStateResponse.AwaitingPlayerIDs.
	AwaitingPlayerIDs          []string                       `json:"-"`
	ID                         string                         `json:"id"`
	Phase                      string                         `json:"phase"`
	ThemeSubmitted             bool                           `json:"themeSubmitted"`
	ThemeCandidates            []string                       `json:"themeCandidates,omitempty"`
	ThemeRanked                bool                           `json:"themeRanked"`
	ThemeDeadline              *time.Time                     `json:"themeDeadline,omitempty"`
	ThemeProgressCount         int                            `json:"themeProgressCount"`
	ThemeProgressRequired      int                            `json:"themeProgressRequired"`
	SelectedThemes             []string                       `json:"selectedThemes,omitempty"`
	RoundNumber                int                            `json:"roundNumber,omitempty"`
	TotalRounds                int                            `json:"totalRounds,omitempty"`
	RoundPhase                 string                         `json:"roundPhase,omitempty"`
	ThemeLabel                 string                         `json:"themeLabel,omitempty"`
	SubmissionDeadline         *time.Time                     `json:"submissionDeadline,omitempty"`
	Nominees                   []zeroToHundredNomineeView     `json:"nominees,omitempty"`
	IsNominee                  bool                           `json:"isNominee,omitempty"`
	Submitted                  bool                           `json:"submitted"`
	SubmissionProgressCount    int                            `json:"submissionProgressCount"`
	SubmissionProgressRequired int                            `json:"submissionProgressRequired"`
	Reveal                     []zeroToHundredRevealEntryView `json:"reveal,omitempty"`
	RoundScore                 int                            `json:"roundScore,omitempty"`
	OrderBonus                 int                            `json:"orderBonus,omitempty"`
	Leaderboard                []fastBioLeaderboardEntryView  `json:"leaderboard,omitempty"`
}

type zeroToHundredNomineeView struct {
	PlayerID        string `json:"playerId"`
	DisplayName     string `json:"displayName"`
	IsCurrentPlayer bool   `json:"isCurrentPlayer"`
}

// zeroToHundredRevealEntryView describes one nominee from the current
// player's point of view. TruePosition (the nominee's own placement) and
// TargetPosition (what everyone is scored against) are nil when the nominee
// did not place themselves; AveragePosition is nil when no other player
// placed them. MyPosition includes the current player's own self-placement;
// Points are the proximity points it earned, order bonus excluded.
type zeroToHundredRevealEntryView struct {
	PlayerID        string   `json:"playerId"`
	DisplayName     string   `json:"displayName"`
	IsCurrentPlayer bool     `json:"isCurrentPlayer"`
	TruePosition    *int     `json:"truePosition,omitempty"`
	AveragePosition *float64 `json:"averagePosition,omitempty"`
	TargetPosition  *int     `json:"targetPosition,omitempty"`
	MyPosition      *int     `json:"myPosition,omitempty"`
	Points          int      `json:"points"`
}

type situationStateView struct {
	// AwaitingPlayerIDs is lifted into lobbyStateResponse.AwaitingPlayerIDs.
	AwaitingPlayerIDs          []string                      `json:"-"`
	ID                         string                        `json:"id"`
	Phase                      string                        `json:"phase"`
	ThemeSubmitted             bool                          `json:"themeSubmitted"`
	ThemeCandidates            []string                      `json:"themeCandidates,omitempty"`
	ThemeRanked                bool                          `json:"themeRanked"`
	ThemeDeadline              *time.Time                    `json:"themeDeadline,omitempty"`
	ThemeProgressCount         int                           `json:"themeProgressCount"`
	ThemeProgressRequired      int                           `json:"themeProgressRequired"`
	SelectedThemes             []string                      `json:"selectedThemes,omitempty"`
	RoundNumber                int                           `json:"roundNumber,omitempty"`
	TotalRounds                int                           `json:"totalRounds,omitempty"`
	RoundPhase                 string                        `json:"roundPhase,omitempty"`
	ThemeLabel                 string                        `json:"themeLabel,omitempty"`
	ProposalDeadline           *time.Time                    `json:"proposalDeadline,omitempty"`
	Submitted                  bool                          `json:"submitted"`
	SubmissionProgressCount    int                           `json:"submissionProgressCount"`
	SubmissionProgressRequired int                           `json:"submissionProgressRequired"`
	CurrentDuel                *situationDuelView            `json:"currentDuel,omitempty"`
	RankingCandidates          []situationProposalView       `json:"rankingCandidates,omitempty"`
	RankingSubmitted           bool                          `json:"rankingSubmitted,omitempty"`
	RoundScore                 int                           `json:"roundScore,omitempty"`
	RoundResults               []situationRoundResultView    `json:"roundResults,omitempty"`
	Leaderboard                []fastBioLeaderboardEntryView `json:"leaderboard,omitempty"`
}

// situationRoundResultView is one finalist's line on the results screen:
// every member of its group earns its Points.
type situationRoundResultView struct {
	Proposal situationProposalView      `json:"proposal"`
	Rank     int                        `json:"rank"`
	Points   int                        `json:"points"`
	Members  []situationGroupMemberView `json:"members"`
	IsMine   bool                       `json:"isMine"`
}

type situationGroupMemberView struct {
	PlayerID    string `json:"playerId"`
	DisplayName string `json:"displayName"`
}

type situationProposalView struct {
	ID                string `json:"id"`
	AuthorPlayerID    string `json:"authorPlayerId,omitempty"`
	AuthorDisplayName string `json:"authorDisplayName,omitempty"`
	ChosenPlayerID    string `json:"chosenPlayerId"`
	ChosenDisplayName string `json:"chosenDisplayName"`
	Reason            string `json:"reason"`
}

type situationDuelView struct {
	ID                  string                `json:"id"`
	OpponentDisplayName string                `json:"opponentDisplayName"`
	ProposalA           situationProposalView `json:"proposalA"`
	ProposalB           situationProposalView `json:"proposalB"`
	MyVoteProposalID    string                `json:"myVoteProposalId,omitempty"`
	OpponentHasVoted    bool                  `json:"opponentHasVoted"`
	Deadline            time.Time             `json:"deadline"`
}

// wavelengthStateView is personalized: Target is only filled in for the
// psychic until the round reaches its results, and the other players'
// needles (Reveal) are only sent once the round is scored.
type wavelengthStateView struct {
	// AwaitingPlayerIDs is lifted into lobbyStateResponse.AwaitingPlayerIDs.
	AwaitingPlayerIDs          []string                      `json:"-"`
	ID                         string                        `json:"id"`
	Phase                      string                        `json:"phase"`
	ThemeSubmitted             bool                          `json:"themeSubmitted"`
	ThemeCandidates            []string                      `json:"themeCandidates,omitempty"`
	ThemeRanked                bool                          `json:"themeRanked"`
	ThemeDeadline              *time.Time                    `json:"themeDeadline,omitempty"`
	ThemeProgressCount         int                           `json:"themeProgressCount"`
	ThemeProgressRequired      int                           `json:"themeProgressRequired"`
	RoundNumber                int                           `json:"roundNumber,omitempty"`
	TotalRounds                int                           `json:"totalRounds,omitempty"`
	IsLastRound                bool                          `json:"isLastRound,omitempty"`
	RoundPhase                 string                        `json:"roundPhase,omitempty"`
	LeftLabel                  string                        `json:"leftLabel,omitempty"`
	RightLabel                 string                        `json:"rightLabel,omitempty"`
	PsychicPlayerID            string                        `json:"psychicPlayerId,omitempty"`
	PsychicDisplayName         string                        `json:"psychicDisplayName,omitempty"`
	IsPsychic                  bool                          `json:"isPsychic,omitempty"`
	Target                     *int                          `json:"target,omitempty"`
	PhaseDeadline              *time.Time                    `json:"phaseDeadline,omitempty"`
	Submitted                  bool                          `json:"submitted"`
	MyPosition                 *int                          `json:"myPosition,omitempty"`
	SubmissionProgressCount    int                           `json:"submissionProgressCount"`
	SubmissionProgressRequired int                           `json:"submissionProgressRequired"`
	Reveal                     []wavelengthRevealEntryView   `json:"reveal,omitempty"`
	PsychicPoints              int                           `json:"psychicPoints,omitempty"`
	RoundScore                 int                           `json:"roundScore,omitempty"`
	Leaderboard                []fastBioLeaderboardEntryView `json:"leaderboard,omitempty"`
	// SpectraToPlay is, during a vote, how many of the ranked spectra will be
	// played over the next rounds.
	SpectraToPlay int `json:"spectraToPlay,omitempty"`
	// NextSpectrumLabel is, on a round's results, the spectrum already voted
	// for the next round; empty when the group votes again next.
	NextSpectrumLabel string `json:"nextSpectrumLabel,omitempty"`
}

type wavelengthRevealEntryView struct {
	PlayerID    string `json:"playerId"`
	DisplayName string `json:"displayName"`
	Position    int    `json:"position"`
	Points      int    `json:"points"`
}

// secretRankingStateView is personalized: MyAmount is the current player's
// own bet, and nobody else's amount is sent before the round reaches its
// reveal. Positions are public: they are on the phones lying on the table.
type secretRankingStateView struct {
	// AwaitingPlayerIDs is lifted into lobbyStateResponse.AwaitingPlayerIDs.
	AwaitingPlayerIDs          []string                        `json:"-"`
	ID                         string                          `json:"id"`
	Phase                      string                          `json:"phase"`
	ThemeSubmitted             bool                            `json:"themeSubmitted"`
	ThemeCandidates            []string                        `json:"themeCandidates,omitempty"`
	ThemeRanked                bool                            `json:"themeRanked"`
	ThemeDeadline              *time.Time                      `json:"themeDeadline,omitempty"`
	ThemeProgressCount         int                             `json:"themeProgressCount"`
	ThemeProgressRequired      int                             `json:"themeProgressRequired"`
	SelectedThemes             []string                        `json:"selectedThemes,omitempty"`
	RoundNumber                int                             `json:"roundNumber,omitempty"`
	TotalRounds                int                             `json:"totalRounds,omitempty"`
	IsLastRound                bool                            `json:"isLastRound,omitempty"`
	RoundPhase                 string                          `json:"roundPhase,omitempty"`
	ThemeLabel                 string                          `json:"themeLabel,omitempty"`
	SecretDeadline             *time.Time                      `json:"secretDeadline,omitempty"`
	Submitted                  bool                            `json:"submitted"`
	SubmissionProgressCount    int                             `json:"submissionProgressCount"`
	SubmissionProgressRequired int                             `json:"submissionProgressRequired"`
	IsParticipant              bool                            `json:"isParticipant"`
	MyAmount                   *int64                          `json:"myAmount,omitempty"`
	MyPosition                 *int                            `json:"myPosition,omitempty"`
	ParticipantCount           int                             `json:"participantCount"`
	NumberedCount              int                             `json:"numberedCount"`
	Seats                      []secretRankingSeatView         `json:"seats,omitempty"`
	LockAt                     *time.Time                      `json:"lockAt,omitempty"`
	PositionsResetAt           *time.Time                      `json:"positionsResetAt,omitempty"`
	RevealAt                   *time.Time                      `json:"revealAt,omitempty"`
	RevealStepMs               int                             `json:"revealStepMs,omitempty"`
	RevealEndsAt               *time.Time                      `json:"revealEndsAt,omitempty"`
	Reveal                     []secretRankingRevealEntryView  `json:"reveal,omitempty"`
	WellPlacedCount            int                             `json:"wellPlacedCount"`
	Cancelled                  bool                            `json:"cancelled,omitempty"`
	Rounds                     []secretRankingRoundSummaryView `json:"rounds,omitempty"`
}

// secretRankingSeatView is one participant at the table. ColorIndex is the
// participant's rank by arrival in the lobby: their phone's color.
type secretRankingSeatView struct {
	PlayerID    string `json:"playerId"`
	DisplayName string `json:"displayName"`
	ColorIndex  int    `json:"colorIndex"`
	Position    *int   `json:"position,omitempty"`
}

// secretRankingRevealEntryView is one phone of the line, in position order.
// A player is well placed when their position lies within their true rank
// range; equal amounts share a range, so they are interchangeable.
type secretRankingRevealEntryView struct {
	PlayerID    string `json:"playerId"`
	DisplayName string `json:"displayName"`
	ColorIndex  int    `json:"colorIndex"`
	Position    int    `json:"position"`
	Amount      int64  `json:"amount"`
	WellPlaced  bool   `json:"wellPlaced"`
	TrueRankMin int    `json:"trueRankMin"`
	TrueRankMax int    `json:"trueRankMax"`
}

// secretRankingRoundSummaryView sums up a played round for the final screen.
type secretRankingRoundSummaryView struct {
	RoundNumber       int      `json:"roundNumber"`
	ThemeLabel        string   `json:"themeLabel"`
	WellPlacedCount   int      `json:"wellPlacedCount"`
	ParticipantCount  int      `json:"participantCount"`
	Cancelled         bool     `json:"cancelled"`
	SmallestPlayerIDs []string `json:"smallestPlayerIds"`
	LargestPlayerIDs  []string `json:"largestPlayerIds"`
	SmallestNames     []string `json:"smallestNames"`
	LargestNames      []string `json:"largestNames"`
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
