package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	hostSessionCookieName = "crush_club_host"
	defaultConfigID       = "00000000-0000-0000-0000-000000000001"
)

var (
	errNotFound     = errors.New("not found")
	errHostRequired = errors.New("host required")
	errLobbyStarted = errors.New("lobby already started")
)

type validationError struct {
	message string
}

func (e validationError) Error() string {
	return e.message
}

type api struct {
	pool             *pgxpool.Pool
	logger           *slog.Logger
	secureCookies    bool
	hub              *realtimeHub
	photoStoragePath string
	photoRetention   time.Duration
	lobbyEmptyGrace  time.Duration
}

type dbQuerier interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

// bumpLobbyRevision marks a committed lobby mutation as a new realtime
// snapshot. Clients intentionally ignore snapshots whose revision is not
// newer than the state they already hold, so every player-visible mutation
// must call this exactly once in the same transaction as the mutation.
func bumpLobbyRevision(ctx context.Context, db dbQuerier, lobbyID string) error {
	tag, err := db.Exec(ctx, `
		UPDATE lobbies SET revision = revision + 1, updated_at = now() WHERE id = $1
	`, lobbyID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("bump lobby revision: updated %d rows for lobby %s", tag.RowsAffected(), lobbyID)
	}
	return nil
}

type rowScanner interface {
	Scan(...any) error
}

type questionOption struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Hint  string `json:"hint,omitempty"`
}

type question struct {
	ID            string           `json:"id"`
	Kind          string           `json:"kind"`
	Type          string           `json:"type"`
	Label         string           `json:"label"`
	Description   string           `json:"description,omitempty"`
	MaximumScore  int              `json:"maximumScore"`
	LoverEligible bool             `json:"loverEligible"`
	Options       []questionOption `json:"options,omitempty"`
	Minimum       *int             `json:"minimum,omitempty"`
	Maximum       *int             `json:"maximum,omitempty"`
	MinimumLabel  string           `json:"minimumLabel,omitempty"`
	MaximumLabel  string           `json:"maximumLabel,omitempty"`
}

type gameConfig struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Kind        string     `json:"kind"`
	IsPublic    bool       `json:"isPublic"`
	IsOwner     bool       `json:"isOwner"`
	Version     int        `json:"version"`
	QuestionIDs []string   `json:"questionIds"`
	Questions   []question `json:"questions"`
	CreatedAt   time.Time  `json:"createdAt"`
	UpdatedAt   time.Time  `json:"updatedAt"`
	ownerID     *string
}

type questionnaireSnapshot struct {
	SourceConfigID string         `json:"sourceConfigId"`
	SourceVersion  int            `json:"sourceVersion"`
	Name           string         `json:"name"`
	Kind           string         `json:"kind,omitempty"`
	Questions      []question     `json:"questions"`
	ProfileFields  []profileField `json:"profileFields"`
}

type gameConfigInput struct {
	Name            string                `json:"name"`
	IsPublic        *bool                 `json:"isPublic,omitempty"`
	QuestionIDs     []string              `json:"questionIds,omitempty"`
	Questions       []configQuestionInput `json:"questions,omitempty"`
	ExpectedVersion int                   `json:"expectedVersion,omitempty"`
}

type configQuestionInput struct {
	QuestionID string   `json:"questionId,omitempty"`
	ID         string   `json:"id,omitempty"`
	Label      string   `json:"label,omitempty"`
	Type       string   `json:"type,omitempty"`
	Options    []string `json:"options,omitempty"`
	Minimum    *int     `json:"minimum,omitempty"`
	Maximum    *int     `json:"maximum,omitempty"`
}

type lobbyResponse struct {
	Code       string `json:"code"`
	Status     string `json:"status"`
	Mode       string `json:"mode"`
	MaxPlayers *int   `json:"maxPlayers"`
	GameConfig struct {
		ID            string `json:"id"`
		Name          string `json:"name"`
		Version       int    `json:"version"`
		QuestionCount int    `json:"questionCount"`
	} `json:"gameConfig"`
	ReconnectToken string `json:"reconnectToken,omitempty"`
}

func newAPI(pool *pgxpool.Pool, logger *slog.Logger, secureCookies bool) *api {
	a := &api{
		pool:             pool,
		logger:           logger,
		secureCookies:    secureCookies,
		photoStoragePath: envOr("PHOTO_STORAGE_PATH", "./data/photos"),
		photoRetention:   positiveEnvDuration(logger, "PHOTO_RETENTION_HOURS", time.Hour, 24*time.Hour),
		lobbyEmptyGrace:  positiveEnvDuration(logger, "LOBBY_EMPTY_GRACE_MINUTES", time.Minute, 15*time.Minute),
	}
	a.hub = newRealtimeHub(a)
	return a
}

func (a *api) routes() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/host-session", a.handleHostSession)
	mux.HandleFunc("GET /api/v1/questions", a.handleListQuestions)
	mux.HandleFunc("GET /api/v1/game-configs", a.handleListConfigs)
	mux.HandleFunc("POST /api/v1/game-configs", a.handleCreateConfig)
	mux.HandleFunc("GET /api/v1/game-configs/{id}", a.handleGetConfig)
	mux.HandleFunc("PUT /api/v1/game-configs/{id}", a.handleUpdateConfig)
	mux.HandleFunc("DELETE /api/v1/game-configs/{id}", a.handleDeleteConfig)
	mux.HandleFunc("POST /api/v1/lobbies", a.handleCreateLobby)
	mux.HandleFunc("GET /api/v1/lobbies/{code}", a.handleGetLobby)
	mux.HandleFunc("POST /api/v1/lobbies/{code}/players", a.handleJoinLobby)
	mux.HandleFunc("GET /api/v1/lobbies/{code}/state", a.handleGetLobbyState)
	mux.HandleFunc("POST /api/v1/lobbies/{code}/players/me/leave", a.handleLeaveLobby)
	mux.HandleFunc("PUT /api/v1/lobbies/{code}/players/me/photos", a.handleUploadPlayerPhotos)
	mux.HandleFunc("POST /api/v1/lobbies/{code}/players/me/photos", a.handleAddPlayerPhoto)
	mux.HandleFunc("PUT /api/v1/lobbies/{code}/players/me/photos/{position}", a.handleReplacePlayerPhoto)
	mux.HandleFunc("DELETE /api/v1/lobbies/{code}/players/me/photos/{position}", a.handleDeletePlayerPhoto)
	mux.HandleFunc("POST /api/v1/lobbies/{code}/players/me/photos/complete", a.handleCompletePlayerPhotos)
	mux.HandleFunc("GET /api/v1/lobbies/{code}/photos/{photoID}", a.handleGetPlayerPhoto)
	mux.HandleFunc("POST /api/v1/lobbies/{code}/players/{playerID}/exclude", a.handleExcludePlayer)
	mux.HandleFunc("PUT /api/v1/lobbies/{code}/game-config", a.handleChangeLobbyConfig)
	mux.HandleFunc("GET /api/v1/lobbies/{code}/questionnaire", a.handleGetQuestionnaire)
	mux.HandleFunc("POST /api/v1/lobbies/{code}/start", a.handleStartMultiplayerGame)
	mux.HandleFunc("PUT /api/v1/lobbies/{code}/rounds/current/submission", a.handleSubmitCurrentRound)
	mux.HandleFunc("PUT /api/v1/lobbies/{code}/rounds/current/vote", a.handleVoteCurrentRound)
	mux.HandleFunc("POST /api/v1/lobbies/{code}/rounds/current/next", a.handleNextRound)
	mux.HandleFunc("POST /api/v1/lobbies/{code}/rounds/next/start", a.handleStartNextRound)
	mux.HandleFunc("POST /api/v1/lobbies/{code}/fast-bio/start", a.handleStartFastBioGame)
	mux.HandleFunc("POST /api/v1/lobbies/{code}/fast-bio/themes", a.handleSubmitFastBioTheme)
	mux.HandleFunc("POST /api/v1/lobbies/{code}/fast-bio/themes/rank", a.handleRankFastBioThemes)
	mux.HandleFunc("POST /api/v1/lobbies/{code}/fast-bio/proposal", a.handleSubmitFastBioProposal)
	mux.HandleFunc("GET /api/v1/lobbies/{code}/fast-bio/photos/{proposalID}", a.handleGetFastBioProposalPhoto)
	mux.HandleFunc("POST /api/v1/lobbies/{code}/fast-bio/review/advance", a.handleAdvanceFastBioReview)
	mux.HandleFunc("POST /api/v1/lobbies/{code}/fast-bio/proposals/{proposalID}/react", a.handleReactToFastBioProposal)
	mux.HandleFunc("POST /api/v1/lobbies/{code}/fast-bio/replay", a.handleReplayFastBio)
	mux.HandleFunc("POST /api/v1/lobbies/{code}/zero-to-100/start", a.handleStartZeroToHundredGame)
	mux.HandleFunc("POST /api/v1/lobbies/{code}/zero-to-100/themes", a.handleSubmitZeroToHundredTheme)
	mux.HandleFunc("POST /api/v1/lobbies/{code}/zero-to-100/themes/rank", a.handleRankZeroToHundredThemes)
	mux.HandleFunc("POST /api/v1/lobbies/{code}/zero-to-100/guesses", a.handleSubmitZeroToHundredGuess)
	mux.HandleFunc("POST /api/v1/lobbies/{code}/zero-to-100/rounds/next", a.handleStartNextZeroToHundredRound)
	mux.HandleFunc("POST /api/v1/lobbies/{code}/zero-to-100/replay", a.handleReplayZeroToHundred)
	mux.HandleFunc("POST /api/v1/lobbies/{code}/situation/start", a.handleStartSituationGame)
	mux.HandleFunc("POST /api/v1/lobbies/{code}/situation/themes", a.handleSubmitSituationTheme)
	mux.HandleFunc("POST /api/v1/lobbies/{code}/situation/themes/rank", a.handleRankSituationThemes)
	mux.HandleFunc("POST /api/v1/lobbies/{code}/situation/proposal", a.handleSubmitSituationProposal)
	mux.HandleFunc("POST /api/v1/lobbies/{code}/situation/duels/{duelID}/vote", a.handleVoteSituationDuel)
	mux.HandleFunc("POST /api/v1/lobbies/{code}/situation/review/advance", a.handleAdvanceSituationReview)
	mux.HandleFunc("POST /api/v1/lobbies/{code}/situation/ranking", a.handleSubmitSituationRanking)
	mux.HandleFunc("POST /api/v1/lobbies/{code}/situation/rounds/next", a.handleStartNextSituationRound)
	mux.HandleFunc("POST /api/v1/lobbies/{code}/situation/replay", a.handleReplaySituation)
	mux.HandleFunc("GET /ws/lobbies/{code}", a.handleLobbyWebSocket)
	return mux
}

func (a *api) handleHostSession(w http.ResponseWriter, r *http.Request) {
	identityID, identityError := a.identityID(r)
	if identityError == nil && identityID != "" {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if identityError != nil && !errors.Is(identityError, errNotFound) {
		a.internalError(w, r, identityError)
		return
	}

	rawToken, err := randomToken(32)
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	tokenHash := sha256.Sum256([]byte(rawToken))
	if _, err := a.pool.Exec(r.Context(), `
		INSERT INTO host_identities (session_token_hash)
		VALUES ($1)
	`, tokenHash[:]); err != nil {
		a.internalError(w, r, err)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     hostSessionCookieName,
		Value:    rawToken,
		Path:     "/",
		MaxAge:   365 * 24 * 60 * 60,
		HttpOnly: true,
		Secure:   a.secureCookies,
		SameSite: http.SameSiteLaxMode,
	})
	w.WriteHeader(http.StatusNoContent)
}

func (a *api) handleListQuestions(w http.ResponseWriter, r *http.Request) {
	questionList, err := loadCatalog(r.Context(), a.pool)
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, questionList)
}

func (a *api) handleListConfigs(w http.ResponseWriter, r *http.Request) {
	identityID, ok := a.requireIdentity(w, r)
	if !ok {
		return
	}

	rows, err := a.pool.Query(r.Context(), `
		SELECT id, owner_identity_id, name, is_system, is_public, version, created_at, updated_at
		FROM game_configs
		WHERE is_system OR owner_identity_id = $1 OR is_public
		ORDER BY is_system DESC, (owner_identity_id = $1) DESC, updated_at DESC, name ASC
	`, identityID)
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	defer rows.Close()

	configList := make([]gameConfig, 0)
	for rows.Next() {
		config, err := scanConfig(rows)
		if err != nil {
			a.internalError(w, r, err)
			return
		}
		config.IsOwner = config.ownerID != nil && *config.ownerID == identityID
		config.Questions, err = loadConfigQuestions(r.Context(), a.pool, config.ID)
		if err != nil {
			a.internalError(w, r, err)
			return
		}
		config.QuestionIDs = questionIDs(config.Questions)
		configList = append(configList, config)
	}
	if err := rows.Err(); err != nil {
		a.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, configList)
}

func (a *api) handleGetConfig(w http.ResponseWriter, r *http.Request) {
	identityID, ok := a.requireIdentity(w, r)
	if !ok {
		return
	}
	config, err := loadVisibleConfig(r.Context(), a.pool, r.PathValue("id"), identityID)
	if errors.Is(err, errNotFound) {
		writeError(w, http.StatusNotFound, "config_not_found", "Configuration not found")
		return
	}
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, config)
}

func (a *api) handleCreateConfig(w http.ResponseWriter, r *http.Request) {
	identityID, ok := a.requireIdentity(w, r)
	if !ok {
		return
	}
	var input gameConfigInput
	if !decodeRequest(w, r, &input) {
		return
	}
	input.Name = strings.TrimSpace(input.Name)
	if inputError := validateConfigEnvelope(input); inputError != nil {
		writeError(w, http.StatusUnprocessableEntity, "invalid_config", inputError.Error())
		return
	}

	tx, err := a.pool.Begin(r.Context())
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	defer tx.Rollback(r.Context())

	var configID string
	isPublic := input.IsPublic != nil && *input.IsPublic
	if err := tx.QueryRow(r.Context(), `
		INSERT INTO game_configs (owner_identity_id, name, is_public)
		VALUES ($1, $2, $3)
		RETURNING id
	`, identityID, input.Name, isPublic).Scan(&configID); err != nil {
		a.internalError(w, r, err)
		return
	}
	if _, err := persistConfigQuestions(
		r.Context(), tx, configID, identityID, normalizedQuestionInputs(input),
	); err != nil {
		var invalidInput validationError
		if errors.As(err, &invalidInput) {
			writeError(w, http.StatusUnprocessableEntity, "invalid_config", err.Error())
			return
		}
		a.internalError(w, r, err)
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		a.internalError(w, r, err)
		return
	}

	config, err := loadVisibleConfig(r.Context(), a.pool, configID, identityID)
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, config)
}

func (a *api) handleUpdateConfig(w http.ResponseWriter, r *http.Request) {
	identityID, ok := a.requireIdentity(w, r)
	if !ok {
		return
	}
	var input gameConfigInput
	if !decodeRequest(w, r, &input) {
		return
	}
	input.Name = strings.TrimSpace(input.Name)
	if input.ExpectedVersion <= 0 {
		writeError(w, http.StatusUnprocessableEntity, "invalid_version", "expectedVersion must be positive")
		return
	}
	if inputError := validateConfigEnvelope(input); inputError != nil {
		writeError(w, http.StatusUnprocessableEntity, "invalid_config", inputError.Error())
		return
	}

	configID := r.PathValue("id")
	var ownerID *string
	var isSystem bool
	var currentIsPublic bool
	var currentVersion int
	err := a.pool.QueryRow(r.Context(), `
		SELECT owner_identity_id, is_system, is_public, version FROM game_configs WHERE id = $1
	`, configID).Scan(&ownerID, &isSystem, &currentIsPublic, &currentVersion)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "config_not_found", "Configuration not found")
		return
	}
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	if isSystem {
		writeError(w, http.StatusForbidden, "system_config_read_only", "System configurations are read-only")
		return
	}
	if ownerID == nil || *ownerID != identityID {
		writeError(w, http.StatusNotFound, "config_not_found", "Configuration not found")
		return
	}
	if currentVersion != input.ExpectedVersion {
		writeError(w, http.StatusConflict, "version_conflict", "Configuration has changed; reload before saving")
		return
	}
	nextIsPublic := currentIsPublic
	if input.IsPublic != nil {
		nextIsPublic = *input.IsPublic
	}

	tx, err := a.pool.Begin(r.Context())
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	defer tx.Rollback(r.Context())
	commandTag, err := tx.Exec(r.Context(), `
		UPDATE game_configs
		SET name = $1, is_public = $2, version = version + 1, updated_at = now()
		WHERE id = $3 AND owner_identity_id = $4 AND version = $5
	`, input.Name, nextIsPublic, configID, identityID, input.ExpectedVersion)
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	if commandTag.RowsAffected() != 1 {
		writeError(w, http.StatusConflict, "version_conflict", "Configuration has changed; reload before saving")
		return
	}
	if _, err := persistConfigQuestions(
		r.Context(), tx, configID, identityID, normalizedQuestionInputs(input),
	); err != nil {
		var invalidInput validationError
		if errors.As(err, &invalidInput) {
			writeError(w, http.StatusUnprocessableEntity, "invalid_config", err.Error())
			return
		}
		a.internalError(w, r, err)
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		a.internalError(w, r, err)
		return
	}
	config, err := loadVisibleConfig(r.Context(), a.pool, configID, identityID)
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, config)
}

func (a *api) handleDeleteConfig(w http.ResponseWriter, r *http.Request) {
	identityID, ok := a.requireIdentity(w, r)
	if !ok {
		return
	}
	configID := r.PathValue("id")
	var ownerID *string
	var isSystem bool
	err := a.pool.QueryRow(r.Context(), `SELECT owner_identity_id, is_system FROM game_configs WHERE id = $1`, configID).Scan(&ownerID, &isSystem)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "config_not_found", "Configuration not found")
		return
	}
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	if isSystem {
		writeError(w, http.StatusForbidden, "system_config_read_only", "System configurations are read-only")
		return
	}
	if ownerID == nil || *ownerID != identityID {
		writeError(w, http.StatusNotFound, "config_not_found", "Configuration not found")
		return
	}
	tx, err := a.pool.Begin(r.Context())
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	defer tx.Rollback(r.Context())
	rows, err := tx.Query(r.Context(), `
		SELECT q.id
		FROM game_config_questions item
		JOIN questions q ON q.id = item.question_id
		WHERE item.game_config_id = $1 AND NOT q.is_system AND q.owner_identity_id = $2
	`, configID, identityID)
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	customQuestionIDs := make([]string, 0)
	for rows.Next() {
		var questionID string
		if err := rows.Scan(&questionID); err != nil {
			rows.Close()
			a.internalError(w, r, err)
			return
		}
		customQuestionIDs = append(customQuestionIDs, questionID)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		a.internalError(w, r, err)
		return
	}
	rows.Close()
	if _, err := tx.Exec(r.Context(), `DELETE FROM game_configs WHERE id = $1 AND owner_identity_id = $2`, configID, identityID); err != nil {
		a.internalError(w, r, err)
		return
	}
	if len(customQuestionIDs) > 0 {
		if _, err := tx.Exec(r.Context(), `
			DELETE FROM questions q
			WHERE q.id = ANY($1::text[]) AND q.owner_identity_id = $2 AND NOT q.is_system
			  AND NOT EXISTS (SELECT 1 FROM game_config_questions item WHERE item.question_id = q.id)
		`, customQuestionIDs, identityID); err != nil {
			a.internalError(w, r, err)
			return
		}
	}
	if err := tx.Commit(r.Context()); err != nil {
		a.internalError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *api) handleCreateLobby(w http.ResponseWriter, r *http.Request) {
	identityID, ok := a.requireIdentity(w, r)
	if !ok {
		return
	}
	var input struct {
		DisplayName  string `json:"displayName"`
		Mode         string `json:"mode"`
		MaxPlayers   int    `json:"maxPlayers"`
		GameConfigID string `json:"gameConfigId"`
	}
	if !decodeRequest(w, r, &input) {
		return
	}
	input.DisplayName = strings.TrimSpace(input.DisplayName)
	if len([]rune(input.DisplayName)) < 2 || len([]rune(input.DisplayName)) > 24 {
		writeError(w, http.StatusUnprocessableEntity, "invalid_lobby", "A valid display name is required")
		return
	}
	if input.Mode == "" {
		input.Mode = lobbyModeClassic
	}
	if input.Mode != lobbyModeClassic && input.Mode != lobbyModeFastBio &&
		input.Mode != lobbyModeZeroToHundred && input.Mode != lobbyModeSituation {
		writeError(w, http.StatusUnprocessableEntity, "invalid_lobby", "mode must be classic, fast_bio, zero_to_100, or situation")
		return
	}
	var maxPlayers *int
	if input.Mode == lobbyModeClassic {
		if input.MaxPlayers == 0 {
			input.MaxPlayers = classicLobbyMaxPlayers
		}
		if input.MaxPlayers < classicLobbyMinPlayers || input.MaxPlayers > classicLobbyMaxPlayers {
			writeError(w, http.StatusUnprocessableEntity, "invalid_lobby", "maxPlayers must be between 2 and 5 for the classic mode")
			return
		}
		maxPlayers = &input.MaxPlayers
	}
	// fast_bio and zero_to_100 have no player cap: maxPlayers stays nil (NULL in the database).
	if input.GameConfigID == "" {
		input.GameConfigID = defaultConfigID
	}
	config, err := loadVisibleConfig(r.Context(), a.pool, input.GameConfigID, identityID)
	if errors.Is(err, errNotFound) {
		writeError(w, http.StatusNotFound, "config_not_found", "Configuration not found")
		return
	}
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	snapshot, err := json.Marshal(snapshotFromConfig(config))
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	lobbyCode, err := randomLobbyCode()
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	reconnectToken, err := randomToken(32)
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	reconnectHash := sha256.Sum256([]byte(reconnectToken))

	tx, err := a.pool.Begin(r.Context())
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	defer tx.Rollback(r.Context())
	var lobbyID string
	if err := tx.QueryRow(r.Context(), `
		INSERT INTO lobbies (
			code, mode, max_players, settings, expires_at, owner_identity_id,
			game_config_id, game_config_version, game_config_snapshot
		)
		VALUES ($1, $2, $3, '{}'::jsonb, $4, $5, $6, $7, $8)
		RETURNING id
	`, lobbyCode, input.Mode, maxPlayers, time.Now().Add(a.photoRetention), identityID, config.ID, config.Version, snapshot).Scan(&lobbyID); err != nil {
		a.internalError(w, r, err)
		return
	}
	var playerID string
	if err := tx.QueryRow(r.Context(), `
		INSERT INTO players (
			lobby_id, display_name, is_host, adult_confirmed, reconnect_token_hash
		)
		VALUES ($1, $2, true, true, $3)
		RETURNING id
	`, lobbyID, input.DisplayName, hex.EncodeToString(reconnectHash[:])).Scan(&playerID); err != nil {
		a.internalError(w, r, err)
		return
	}
	if _, err := tx.Exec(r.Context(), `UPDATE lobbies SET host_player_id = $1 WHERE id = $2`, playerID, lobbyID); err != nil {
		a.internalError(w, r, err)
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		a.internalError(w, r, err)
		return
	}

	player := authenticatedPlayer{
		ID:          playerID,
		LobbyID:     lobbyID,
		Code:        lobbyCode,
		DisplayName: input.DisplayName,
		IsHost:      true,
	}
	state, err := a.loadLobbyState(r.Context(), player)
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, playerSessionResponse{
		ReconnectToken: reconnectToken,
		State:          state,
	})
}

func (a *api) handleGetLobby(w http.ResponseWriter, r *http.Request) {
	response, err := loadLobbyResponse(r.Context(), a.pool, strings.ToUpper(r.PathValue("code")))
	if errors.Is(err, errNotFound) {
		writeError(w, http.StatusNotFound, "lobby_not_found", "Lobby not found")
		return
	}
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, response)
}

func (a *api) handleChangeLobbyConfig(w http.ResponseWriter, r *http.Request) {
	player, playerOK := a.requirePlayer(w, r)
	if !playerOK {
		return
	}
	if !player.IsHost {
		writeError(w, http.StatusForbidden, "host_required", "Only the lobby host can change its configuration")
		return
	}
	identityID, ok := a.requireIdentity(w, r)
	if !ok {
		return
	}
	var input struct {
		GameConfigID string `json:"gameConfigId"`
	}
	if !decodeRequest(w, r, &input) {
		return
	}
	if input.GameConfigID == "" {
		writeError(w, http.StatusUnprocessableEntity, "invalid_config", "gameConfigId is required")
		return
	}

	tx, err := a.pool.Begin(r.Context())
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	defer tx.Rollback(r.Context())
	code := strings.ToUpper(r.PathValue("code"))
	var ownerID *string
	var status string
	var maxPlayers int
	err = tx.QueryRow(r.Context(), `
		SELECT owner_identity_id, status, max_players FROM lobbies WHERE code = $1 FOR UPDATE
	`, code).Scan(&ownerID, &status, &maxPlayers)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "lobby_not_found", "Lobby not found")
		return
	}
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	if authorizationError := validateLobbyConfigChange(ownerID, identityID, status); errors.Is(authorizationError, errHostRequired) {
		writeError(w, http.StatusForbidden, "host_required", "Only the lobby host can change its configuration")
		return
	} else if errors.Is(authorizationError, errLobbyStarted) {
		writeError(w, http.StatusConflict, "lobby_already_started", "The configuration cannot change after the game starts")
		return
	}
	config, err := loadVisibleConfig(r.Context(), tx, input.GameConfigID, identityID)
	if errors.Is(err, errNotFound) {
		writeError(w, http.StatusNotFound, "config_not_found", "Configuration not found")
		return
	}
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	snapshotValue := snapshotFromConfig(config)
	snapshotJSON, err := json.Marshal(snapshotValue)
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	if _, err := tx.Exec(r.Context(), `
		UPDATE lobbies
		SET game_config_id = $1, game_config_version = $2, game_config_snapshot = $3,
		    revision = revision + 1, updated_at = now()
		WHERE code = $4
	`, config.ID, config.Version, snapshotJSON, code); err != nil {
		a.internalError(w, r, err)
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		a.internalError(w, r, err)
		return
	}
	state, err := a.loadLobbyState(r.Context(), player)
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	a.hub.publish(code)
	writeJSON(w, http.StatusOK, state)
}

func (a *api) handleGetQuestionnaire(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.requirePlayer(w, r); !ok {
		return
	}
	var snapshotJSON []byte
	err := a.pool.QueryRow(r.Context(), `
		SELECT game_config_snapshot
		FROM lobbies
		WHERE code = $1 AND status <> 'expired' AND expires_at > now()
	`, strings.ToUpper(r.PathValue("code"))).Scan(&snapshotJSON)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "lobby_not_found", "Lobby not found")
		return
	}
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	var snapshot questionnaireSnapshot
	if err := json.Unmarshal(snapshotJSON, &snapshot); err != nil {
		a.internalError(w, r, err)
		return
	}
	hydrateQuestionnaireSnapshot(&snapshot)
	writeJSON(w, http.StatusOK, snapshot)
}

func (a *api) handleStartLobby(w http.ResponseWriter, r *http.Request) {
	identityID, ok := a.requireIdentity(w, r)
	if !ok {
		return
	}
	code := strings.ToUpper(r.PathValue("code"))
	commandTag, err := a.pool.Exec(r.Context(), `
		UPDATE lobbies
		SET status = 'in_game', updated_at = now()
		WHERE code = $1
		  AND owner_identity_id = $2
		  AND status IN ('waiting_for_players', 'preparing_photos', 'ready_to_start')
	`, code, identityID)
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	if commandTag.RowsAffected() != 1 {
		var ownerID *string
		var status string
		err := a.pool.QueryRow(r.Context(), `SELECT owner_identity_id, status FROM lobbies WHERE code = $1`, code).Scan(&ownerID, &status)
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			writeError(w, http.StatusNotFound, "lobby_not_found", "Lobby not found")
		case err != nil:
			a.internalError(w, r, err)
		case ownerID == nil || *ownerID != identityID:
			writeError(w, http.StatusForbidden, "host_required", "Only the lobby host can start the game")
		default:
			writeError(w, http.StatusConflict, "invalid_lobby_status", "Lobby cannot be started from its current status")
		}
		return
	}
	response, err := loadLobbyResponse(r.Context(), a.pool, code)
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, response)
}

func (a *api) identityID(r *http.Request) (string, error) {
	cookie, err := r.Cookie(hostSessionCookieName)
	if err != nil || cookie.Value == "" {
		return "", errNotFound
	}
	tokenHash := sha256.Sum256([]byte(cookie.Value))
	var identityID string
	err = a.pool.QueryRow(r.Context(), `
		UPDATE host_identities SET last_seen_at = now()
		WHERE session_token_hash = $1
		RETURNING id
	`, tokenHash[:]).Scan(&identityID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", errNotFound
	}
	return identityID, err
}

func (a *api) requireIdentity(w http.ResponseWriter, r *http.Request) (string, bool) {
	identityID, err := a.identityID(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "host_session_required", "Create a host session first")
		return "", false
	}
	return identityID, true
}

func loadCatalog(ctx context.Context, db dbQuerier) ([]question, error) {
	rows, err := db.Query(ctx, `
		SELECT id, type, label, description, maximum_score, lover_eligible, options,
		       minimum, maximum, minimum_label, maximum_label, is_system
		FROM questions
		WHERE is_active AND is_system
		ORDER BY created_at, id
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]question, 0)
	for rows.Next() {
		item, err := scanQuestion(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func loadConfigQuestions(ctx context.Context, db dbQuerier, configID string) ([]question, error) {
	rows, err := db.Query(ctx, `
		SELECT q.id, q.type, q.label, q.description, q.maximum_score, q.lover_eligible, q.options,
		       q.minimum, q.maximum, q.minimum_label, q.maximum_label, q.is_system
		FROM game_config_questions item
		JOIN questions q ON q.id = item.question_id
		WHERE item.game_config_id = $1
		ORDER BY item.position
	`, configID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]question, 0)
	for rows.Next() {
		item, err := scanQuestion(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func scanQuestion(row rowScanner) (question, error) {
	var item question
	var description, minimumLabel, maximumLabel *string
	var optionsJSON []byte
	var isSystem bool
	err := row.Scan(
		&item.ID, &item.Type, &item.Label, &description, &item.MaximumScore,
		&item.LoverEligible, &optionsJSON, &item.Minimum, &item.Maximum,
		&minimumLabel, &maximumLabel, &isSystem,
	)
	if err != nil {
		return question{}, err
	}
	if description != nil {
		item.Description = *description
	}
	if minimumLabel != nil {
		item.MinimumLabel = *minimumLabel
	}
	if maximumLabel != nil {
		item.MaximumLabel = *maximumLabel
	}
	if len(optionsJSON) > 0 {
		if err := json.Unmarshal(optionsJSON, &item.Options); err != nil {
			return question{}, err
		}
	}
	if isSystem {
		item.Kind = "system"
	} else {
		item.Kind = "personal"
	}
	return item, nil
}

func scanConfig(row rowScanner) (gameConfig, error) {
	var config gameConfig
	var isSystem bool
	err := row.Scan(
		&config.ID, &config.ownerID, &config.Name, &isSystem, &config.IsPublic,
		&config.Version, &config.CreatedAt, &config.UpdatedAt,
	)
	if err != nil {
		return gameConfig{}, err
	}
	if isSystem {
		config.Kind = "system"
	} else {
		config.Kind = "personal"
	}
	return config, nil
}

func loadVisibleConfig(ctx context.Context, db dbQuerier, configID, identityID string) (gameConfig, error) {
	config, err := scanConfig(db.QueryRow(ctx, `
		SELECT id, owner_identity_id, name, is_system, is_public, version, created_at, updated_at
		FROM game_configs
		WHERE id = $1 AND (is_system OR owner_identity_id = $2 OR is_public)
	`, configID, identityID))
	if errors.Is(err, pgx.ErrNoRows) {
		return gameConfig{}, errNotFound
	}
	if err != nil {
		return gameConfig{}, err
	}
	config.IsOwner = config.ownerID != nil && *config.ownerID == identityID
	config.Questions, err = loadConfigQuestions(ctx, db, config.ID)
	if err != nil {
		return gameConfig{}, err
	}
	config.QuestionIDs = questionIDs(config.Questions)
	return config, nil
}

func normalizedQuestionInputs(input gameConfigInput) []configQuestionInput {
	if len(input.Questions) > 0 {
		return input.Questions
	}
	result := make([]configQuestionInput, 0, len(input.QuestionIDs))
	for _, questionID := range input.QuestionIDs {
		result = append(result, configQuestionInput{QuestionID: questionID})
	}
	return result
}

func validateConfigEnvelope(input gameConfigInput) error {
	if len([]rune(input.Name)) < 1 || len([]rune(input.Name)) > 80 {
		return validationError{"name must contain between 1 and 80 characters"}
	}
	questionInputs := normalizedQuestionInputs(input)
	if len(questionInputs) < 1 || len(questionInputs) > 50 {
		return validationError{"select between 1 and 50 questions"}
	}
	seen := make(map[string]struct{}, len(questionInputs))
	for _, questionInput := range questionInputs {
		if questionInput.QuestionID != "" {
			key := "reference:" + questionInput.QuestionID
			if _, exists := seen[key]; exists {
				return validationError{"questions must be unique"}
			}
			seen[key] = struct{}{}
			continue
		}
		if questionInput.ID != "" {
			key := "custom:" + questionInput.ID
			if _, exists := seen[key]; exists {
				return validationError{"questions must be unique"}
			}
			seen[key] = struct{}{}
		}
		if _, err := normalizeCustomQuestion(questionInput); err != nil {
			return err
		}
	}
	return nil
}

func normalizeCustomQuestion(input configQuestionInput) (question, error) {
	label := strings.TrimSpace(input.Label)
	if len([]rune(label)) < 1 || len([]rune(label)) > 160 {
		return question{}, validationError{"each custom question needs a label between 1 and 160 characters"}
	}
	result := question{
		ID: input.ID, Kind: "personal", Type: input.Type, Label: label,
		MaximumScore: 10, LoverEligible: true,
	}
	switch input.Type {
	case "integer_range":
		if input.Minimum == nil || input.Maximum == nil || *input.Minimum >= *input.Maximum {
			return question{}, validationError{"number questions require a minimum lower than the maximum"}
		}
		minimum := *input.Minimum
		maximum := *input.Maximum
		result.Minimum = &minimum
		result.Maximum = &maximum
		result.MinimumLabel = strconv.Itoa(minimum)
		result.MaximumLabel = strconv.Itoa(maximum)
	case "single_choice":
		if len(input.Options) < 2 || len(input.Options) > 20 {
			return question{}, validationError{"list questions require between 2 and 20 options"}
		}
		result.Options = make([]questionOption, 0, len(input.Options))
		for index, optionLabel := range input.Options {
			optionLabel = strings.TrimSpace(optionLabel)
			if len([]rune(optionLabel)) < 1 || len([]rune(optionLabel)) > 80 {
				return question{}, validationError{"each list option needs text between 1 and 80 characters"}
			}
			result.Options = append(result.Options, questionOption{
				ID: fmt.Sprintf("option-%d", index+1), Label: optionLabel,
			})
		}
	case "binary_choice":
		result.Options = []questionOption{
			{ID: "yes", Label: "Oui"},
			{ID: "no", Label: "Non"},
		}
	default:
		return question{}, validationError{"question type must be integer_range, single_choice or binary_choice"}
	}
	return result, nil
}

func orderAndValidateQuestions(questionIDList []string, catalog []question) ([]question, error) {
	questionByID := make(map[string]question, len(catalog))
	for _, item := range catalog {
		questionByID[item.ID] = item
	}
	selected := make([]question, 0, len(questionIDList))
	hasLoverEligible := false
	for _, questionID := range questionIDList {
		item, exists := questionByID[questionID]
		if !exists {
			return nil, validationError{fmt.Sprintf("unknown or inactive question: %s", questionID)}
		}
		selected = append(selected, item)
		hasLoverEligible = hasLoverEligible || item.LoverEligible
	}
	if !hasLoverEligible {
		return nil, validationError{"select at least one LOVER-eligible question"}
	}
	return selected, nil
}

func validateLobbyConfigChange(ownerID *string, identityID, status string) error {
	if ownerID == nil || *ownerID != identityID {
		return errHostRequired
	}
	if status == "in_game" || status == "completed" || status == "expired" {
		return errLobbyStarted
	}
	return nil
}

func persistConfigQuestions(
	ctx context.Context,
	tx pgx.Tx,
	configID string,
	identityID string,
	questionInputs []configQuestionInput,
) ([]string, error) {
	rows, err := tx.Query(ctx, `
		SELECT q.id
		FROM game_config_questions item
		JOIN questions q ON q.id = item.question_id
		WHERE item.game_config_id = $1 AND NOT q.is_system AND q.owner_identity_id = $2
	`, configID, identityID)
	if err != nil {
		return nil, err
	}
	previousCustomIDs := make([]string, 0)
	for rows.Next() {
		var questionID string
		if err := rows.Scan(&questionID); err != nil {
			rows.Close()
			return nil, err
		}
		previousCustomIDs = append(previousCustomIDs, questionID)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()

	questionIDList := make([]string, 0, len(questionInputs))
	seenIDs := make(map[string]struct{}, len(questionInputs))
	hasLoverEligible := false
	for _, questionInput := range questionInputs {
		questionID := questionInput.QuestionID
		if questionID != "" {
			referencedQuestionID := questionID
			var loverEligible bool
			questionID, loverEligible, err = cloneQuestionReference(ctx, tx, questionID, identityID)
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, validationError{fmt.Sprintf("unknown or inaccessible question: %s", referencedQuestionID)}
			}
			if err != nil {
				return nil, err
			}
			hasLoverEligible = hasLoverEligible || loverEligible
		} else {
			normalized, err := normalizeCustomQuestion(questionInput)
			if err != nil {
				return nil, err
			}
			optionsJSON, err := marshalQuestionOptions(normalized.Options)
			if err != nil {
				return nil, err
			}
			if questionInput.ID == "" {
				err = tx.QueryRow(ctx, `
					INSERT INTO questions (
						id, type, label, maximum_score, lover_eligible, options,
						minimum, maximum, minimum_label, maximum_label,
						owner_identity_id, is_system
					)
					VALUES (
						gen_random_uuid()::text, $1, $2, 10, true, $3,
						$4, $5, $6, $7, $8, false
					)
					RETURNING id
				`, normalized.Type, normalized.Label, optionsJSON,
					normalized.Minimum, normalized.Maximum,
					nullableString(normalized.MinimumLabel), nullableString(normalized.MaximumLabel),
					identityID,
				).Scan(&questionID)
			} else {
				questionID = questionInput.ID
				commandTag, updateErr := tx.Exec(ctx, `
					UPDATE questions AS q
					SET type = $1, label = $2, options = $3,
					    minimum = $4, maximum = $5, minimum_label = $6, maximum_label = $7,
					    updated_at = now()
					WHERE q.id = $8 AND q.owner_identity_id = $9 AND NOT q.is_system
					  AND EXISTS (
					    SELECT 1 FROM game_config_questions item
					    WHERE item.game_config_id = $10 AND item.question_id = q.id
					  )
				`, normalized.Type, normalized.Label, optionsJSON,
					normalized.Minimum, normalized.Maximum,
					nullableString(normalized.MinimumLabel), nullableString(normalized.MaximumLabel),
					questionID, identityID, configID,
				)
				err = updateErr
				if err == nil && commandTag.RowsAffected() != 1 {
					return nil, validationError{"custom question does not belong to this configuration"}
				}
			}
			if err != nil {
				return nil, err
			}
			hasLoverEligible = true
		}
		if _, exists := seenIDs[questionID]; exists {
			return nil, validationError{"questions must be unique"}
		}
		seenIDs[questionID] = struct{}{}
		questionIDList = append(questionIDList, questionID)
	}
	if !hasLoverEligible {
		return nil, validationError{"select at least one LOVER-eligible question"}
	}

	if _, err := tx.Exec(ctx, `DELETE FROM game_config_questions WHERE game_config_id = $1`, configID); err != nil {
		return nil, err
	}
	for position, questionID := range questionIDList {
		if _, err := tx.Exec(ctx, `
			INSERT INTO game_config_questions (game_config_id, question_id, position)
			VALUES ($1, $2, $3)
		`, configID, questionID, position); err != nil {
			return nil, err
		}
	}
	if len(previousCustomIDs) > 0 {
		if _, err := tx.Exec(ctx, `
			DELETE FROM questions q
			WHERE q.id = ANY($1::text[])
			  AND q.owner_identity_id = $2
			  AND NOT q.is_system
			  AND NOT EXISTS (
			    SELECT 1 FROM game_config_questions item WHERE item.question_id = q.id
			  )
		`, previousCustomIDs, identityID); err != nil {
			return nil, err
		}
	}
	return questionIDList, nil
}

func cloneQuestionReference(
	ctx context.Context,
	tx pgx.Tx,
	questionID string,
	identityID string,
) (string, bool, error) {
	var clonedID string
	var loverEligible bool
	err := tx.QueryRow(ctx, `
		INSERT INTO questions (
			id, type, label, description, maximum_score, lover_eligible, options,
			minimum, maximum, minimum_label, maximum_label, is_active,
			owner_identity_id, is_system
		)
		SELECT
			gen_random_uuid()::text, type, label, description, maximum_score, lover_eligible, options,
			minimum, maximum, minimum_label, maximum_label, is_active,
			$2, false
		FROM questions
		WHERE id = $1 AND is_active
		  AND (is_system OR owner_identity_id = $2)
		RETURNING id, lover_eligible
	`, questionID, identityID).Scan(&clonedID, &loverEligible)
	return clonedID, loverEligible, err
}

func marshalQuestionOptions(options []questionOption) (any, error) {
	if len(options) == 0 {
		return nil, nil
	}
	return json.Marshal(options)
}

func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func questionIDs(questionList []question) []string {
	ids := make([]string, 0, len(questionList))
	for _, item := range questionList {
		ids = append(ids, item.ID)
	}
	return ids
}

func snapshotFromConfig(config gameConfig) questionnaireSnapshot {
	questions := append([]question(nil), config.Questions...)
	profileFields := make([]profileField, 0)
	if config.Kind == "system" {
		profileFields = defaultProfileFields()
	}
	return questionnaireSnapshot{
		SourceConfigID: config.ID,
		SourceVersion:  config.Version,
		Name:           config.Name,
		Kind:           config.Kind,
		Questions:      questions,
		ProfileFields:  profileFields,
	}
}

func hydrateQuestionnaireSnapshot(snapshot *questionnaireSnapshot) {
	if snapshot.Kind == "personal" {
		if snapshot.ProfileFields == nil {
			snapshot.ProfileFields = make([]profileField, 0)
		}
		return
	}
	if len(snapshot.ProfileFields) == 0 {
		snapshot.ProfileFields = defaultProfileFields()
	}
}

func lobbyFromSnapshot(code, status, mode string, maxPlayers *int, snapshot questionnaireSnapshot) lobbyResponse {
	response := lobbyResponse{Code: code, Status: status, Mode: mode, MaxPlayers: maxPlayers}
	response.GameConfig.ID = snapshot.SourceConfigID
	response.GameConfig.Name = snapshot.Name
	response.GameConfig.Version = snapshot.SourceVersion
	response.GameConfig.QuestionCount = len(snapshot.Questions)
	return response
}

func loadLobbyResponse(ctx context.Context, db dbQuerier, code string) (lobbyResponse, error) {
	var status, mode string
	var maxPlayers *int
	var snapshotJSON []byte
	err := db.QueryRow(ctx, `
		SELECT status, mode, max_players, game_config_snapshot
		FROM lobbies
		WHERE code = $1 AND status <> 'expired' AND expires_at > now()
	`, code).Scan(&status, &mode, &maxPlayers, &snapshotJSON)
	if errors.Is(err, pgx.ErrNoRows) {
		return lobbyResponse{}, errNotFound
	}
	if err != nil {
		return lobbyResponse{}, err
	}
	var snapshot questionnaireSnapshot
	if err := json.Unmarshal(snapshotJSON, &snapshot); err != nil {
		return lobbyResponse{}, err
	}
	return lobbyFromSnapshot(code, status, mode, maxPlayers, snapshot), nil
}

func randomToken(byteCount int) (string, error) {
	buffer := make([]byte, byteCount)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buffer), nil
}

func positiveEnvDuration(logger *slog.Logger, key string, unit, fallback time.Duration) time.Duration {
	rawValue := strings.TrimSpace(os.Getenv(key))
	if rawValue == "" {
		return fallback
	}
	value, err := strconv.Atoi(rawValue)
	if err != nil || value <= 0 {
		logger.Warn("invalid duration configuration; using default", "key", key, "default", fallback.String())
		return fallback
	}
	return time.Duration(value) * unit
}

func randomLobbyCode() (string, error) {
	const alphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	buffer := make([]byte, 6)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	for index := range buffer {
		buffer[index] = alphabet[int(buffer[index])%len(alphabet)]
	}
	return string(buffer), nil
}

func decodeRequest(w http.ResponseWriter, r *http.Request, destination any) bool {
	decoder := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "Request body is invalid")
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{
		"error": map[string]string{"code": code, "message": message},
	})
}

func (a *api) internalError(w http.ResponseWriter, r *http.Request, err error) {
	a.logger.Error("request failed", "method", r.Method, "path", r.URL.Path, "error", err)
	writeError(w, http.StatusInternalServerError, "internal_error", "An unexpected error occurred")
}
