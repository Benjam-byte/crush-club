package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

const photoOrphanGracePeriod = time.Hour

var storedPhotoFilePattern = regexp.MustCompile(`^[A-Za-z0-9_-]{32}\.(?:jpg|png|webp)$`)

type validatedPhoto struct {
	temporaryPath string
	storageKey    string
	contentType   string
	width         int
	height        int
	size          int64
	id            string
}

type photoHTTPError struct {
	status  int
	code    string
	message string
}

func (e photoHTTPError) Error() string {
	return e.message
}

func (a *api) handleUploadPlayerPhotos(w http.ResponseWriter, r *http.Request) {
	player, ok := a.requirePlayer(w, r)
	if !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maximumPhotoSizeBytes*4+(1<<20))
	photoList, err := a.stagePhotosFromRequest(r, "photos", 4)
	if err != nil {
		a.writePhotoFailure(w, r, err)
		return
	}
	defer removeTemporaryPhotos(photoList)

	tx, err := a.pool.Begin(r.Context())
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	defer tx.Rollback(r.Context())
	if _, _, err := lockPhotoPreparation(r.Context(), tx, player, true); err != nil {
		a.writePhotoFailure(w, r, err)
		return
	}
	rows, err := tx.Query(r.Context(), `SELECT storage_key FROM player_photos WHERE player_id = $1`, player.ID)
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	oldStorageKeys := make([]string, 0, 4)
	for rows.Next() {
		var storageKey string
		if err := rows.Scan(&storageKey); err != nil {
			rows.Close()
			a.internalError(w, r, err)
			return
		}
		oldStorageKeys = append(oldStorageKeys, storageKey)
	}
	rows.Close()
	if _, err := tx.Exec(r.Context(), `DELETE FROM player_photos WHERE player_id = $1`, player.ID); err != nil {
		a.internalError(w, r, err)
		return
	}
	for index := range photoList {
		photo := &photoList[index]
		err := tx.QueryRow(r.Context(), `
			INSERT INTO player_photos (
				player_id, storage_key, position, width, height, content_type, size_bytes
			)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
			RETURNING id
		`, player.ID, photo.storageKey, index+1, photo.width, photo.height, photo.contentType, photo.size).Scan(&photo.id)
		if err != nil {
			a.internalError(w, r, err)
			return
		}
	}
	if err := updatePhotoReadiness(r.Context(), tx, player, "ready"); err != nil {
		a.internalError(w, r, err)
		return
	}

	movedPaths, err := a.moveStagedPhotos(photoList)
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		removePhotoPaths(movedPaths)
		a.internalError(w, r, err)
		return
	}
	for _, storageKey := range oldStorageKeys {
		_ = os.Remove(filepath.Join(a.photoStoragePath, filepath.Base(storageKey)))
	}
	a.writePhotoState(w, r, player, http.StatusOK)
}

func (a *api) handleAddPlayerPhoto(w http.ResponseWriter, r *http.Request) {
	player, ok := a.requirePlayer(w, r)
	if !ok {
		return
	}
	photo, ok := a.stageSinglePhoto(w, r)
	if !ok {
		return
	}
	defer removeTemporaryPhotos([]validatedPhoto{photo})

	tx, err := a.pool.Begin(r.Context())
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	defer tx.Rollback(r.Context())
	if _, _, err := lockPhotoPreparation(r.Context(), tx, player, true); err != nil {
		a.writePhotoFailure(w, r, err)
		return
	}
	var photoCount int
	if err := tx.QueryRow(r.Context(), `SELECT count(*) FROM player_photos WHERE player_id = $1`, player.ID).Scan(&photoCount); err != nil {
		a.internalError(w, r, err)
		return
	}
	if photoCount >= 4 {
		a.writePhotoFailure(w, r, photoHTTPError{http.StatusConflict, "photo_limit_reached", "Four photos have already been added"})
		return
	}
	position := photoCount + 1
	if err := tx.QueryRow(r.Context(), `
		INSERT INTO player_photos (player_id, storage_key, position, width, height, content_type, size_bytes)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id
	`, player.ID, photo.storageKey, position, photo.width, photo.height, photo.contentType, photo.size).Scan(&photo.id); err != nil {
		a.internalError(w, r, err)
		return
	}
	if err := updatePhotoReadiness(r.Context(), tx, player, "preparing_photos"); err != nil {
		a.internalError(w, r, err)
		return
	}
	movedPaths, err := a.moveStagedPhotos([]validatedPhoto{photo})
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		removePhotoPaths(movedPaths)
		a.internalError(w, r, err)
		return
	}
	a.writePhotoState(w, r, player, http.StatusCreated)
}

func (a *api) handleReplacePlayerPhoto(w http.ResponseWriter, r *http.Request) {
	player, ok := a.requirePlayer(w, r)
	if !ok {
		return
	}
	position, err := playerPhotoPosition(r)
	if err != nil {
		a.writePhotoFailure(w, r, err)
		return
	}
	photo, ok := a.stageSinglePhoto(w, r)
	if !ok {
		return
	}
	defer removeTemporaryPhotos([]validatedPhoto{photo})

	tx, err := a.pool.Begin(r.Context())
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	defer tx.Rollback(r.Context())
	if _, _, err := lockPhotoPreparation(r.Context(), tx, player, true); err != nil {
		a.writePhotoFailure(w, r, err)
		return
	}
	var oldStorageKey string
	if err := tx.QueryRow(r.Context(), `
		SELECT storage_key FROM player_photos WHERE player_id = $1 AND position = $2 FOR UPDATE
	`, player.ID, position).Scan(&oldStorageKey); errors.Is(err, pgx.ErrNoRows) {
		a.writePhotoFailure(w, r, photoHTTPError{http.StatusNotFound, "photo_not_found", "Photo not found"})
		return
	} else if err != nil {
		a.internalError(w, r, err)
		return
	}
	if _, err := tx.Exec(r.Context(), `DELETE FROM player_photos WHERE player_id = $1 AND position = $2`, player.ID, position); err != nil {
		a.internalError(w, r, err)
		return
	}
	if err := tx.QueryRow(r.Context(), `
		INSERT INTO player_photos (player_id, storage_key, position, width, height, content_type, size_bytes)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id
	`, player.ID, photo.storageKey, position, photo.width, photo.height, photo.contentType, photo.size).Scan(&photo.id); err != nil {
		a.internalError(w, r, err)
		return
	}
	if err := updatePhotoReadiness(r.Context(), tx, player, "preparing_photos"); err != nil {
		a.internalError(w, r, err)
		return
	}
	movedPaths, err := a.moveStagedPhotos([]validatedPhoto{photo})
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		removePhotoPaths(movedPaths)
		a.internalError(w, r, err)
		return
	}
	_ = os.Remove(filepath.Join(a.photoStoragePath, filepath.Base(oldStorageKey)))
	a.writePhotoState(w, r, player, http.StatusOK)
}

func (a *api) handleDeletePlayerPhoto(w http.ResponseWriter, r *http.Request) {
	player, ok := a.requirePlayer(w, r)
	if !ok {
		return
	}
	position, err := playerPhotoPosition(r)
	if err != nil {
		a.writePhotoFailure(w, r, err)
		return
	}
	tx, err := a.pool.Begin(r.Context())
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	defer tx.Rollback(r.Context())
	if _, _, err := lockPhotoPreparation(r.Context(), tx, player, true); err != nil {
		a.writePhotoFailure(w, r, err)
		return
	}
	var storageKey string
	if err := tx.QueryRow(r.Context(), `
		DELETE FROM player_photos WHERE player_id = $1 AND position = $2 RETURNING storage_key
	`, player.ID, position).Scan(&storageKey); errors.Is(err, pgx.ErrNoRows) {
		a.writePhotoFailure(w, r, photoHTTPError{http.StatusNotFound, "photo_not_found", "Photo not found"})
		return
	} else if err != nil {
		a.internalError(w, r, err)
		return
	}
	for sourcePosition := position + 1; sourcePosition <= 4; sourcePosition++ {
		if _, err := tx.Exec(r.Context(), `
			UPDATE player_photos SET position = $1 WHERE player_id = $2 AND position = $3
		`, sourcePosition-1, player.ID, sourcePosition); err != nil {
			a.internalError(w, r, err)
			return
		}
	}
	if err := updatePhotoReadiness(r.Context(), tx, player, "preparing_photos"); err != nil {
		a.internalError(w, r, err)
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		a.internalError(w, r, err)
		return
	}
	_ = os.Remove(filepath.Join(a.photoStoragePath, filepath.Base(storageKey)))
	a.writePhotoState(w, r, player, http.StatusOK)
}

func (a *api) handleCompletePlayerPhotos(w http.ResponseWriter, r *http.Request) {
	player, ok := a.requirePlayer(w, r)
	if !ok {
		return
	}
	tx, err := a.pool.Begin(r.Context())
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	defer tx.Rollback(r.Context())
	readyStatus, _, err := lockPhotoPreparation(r.Context(), tx, player, false)
	if err != nil {
		a.writePhotoFailure(w, r, err)
		return
	}
	if readyStatus != "ready" {
		var photoCount int
		if err := tx.QueryRow(r.Context(), `SELECT count(*) FROM player_photos WHERE player_id = $1`, player.ID).Scan(&photoCount); err != nil {
			a.internalError(w, r, err)
			return
		}
		if photoCount != 4 {
			a.writePhotoFailure(w, r, photoHTTPError{http.StatusUnprocessableEntity, "incomplete_photos", "Four photos are required before validation"})
			return
		}
		if err := updatePhotoReadiness(r.Context(), tx, player, "ready"); err != nil {
			a.internalError(w, r, err)
			return
		}
	}
	if err := tx.Commit(r.Context()); err != nil {
		a.internalError(w, r, err)
		return
	}
	a.writePhotoState(w, r, player, http.StatusOK)
}

func (a *api) stageSinglePhoto(w http.ResponseWriter, r *http.Request) (validatedPhoto, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, maximumPhotoSizeBytes+(1<<20))
	photoList, err := a.stagePhotosFromRequest(r, "photo", 1)
	if err != nil {
		a.writePhotoFailure(w, r, err)
		return validatedPhoto{}, false
	}
	return photoList[0], true
}

func (a *api) stagePhotosFromRequest(r *http.Request, fieldName string, expectedCount int) ([]validatedPhoto, error) {
	if err := r.ParseMultipartForm(maximumPhotoSizeBytes * int64(expectedCount)); err != nil {
		return nil, photoHTTPError{http.StatusRequestEntityTooLarge, "photo_too_large", "Each photo must be at most 7 MB"}
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}
	fileHeaders := r.MultipartForm.File[fieldName]
	if len(fileHeaders) != expectedCount {
		return nil, photoHTTPError{http.StatusUnprocessableEntity, "invalid_photo_count", "The expected number of photos is required"}
	}
	if err := os.MkdirAll(a.photoStoragePath, 0o750); err != nil {
		return nil, err
	}
	photoList := make([]validatedPhoto, 0, expectedCount)
	for _, fileHeader := range fileHeaders {
		photo, err := a.stagePhoto(fileHeader)
		if err != nil {
			removeTemporaryPhotos(photoList)
			return nil, err
		}
		photoList = append(photoList, photo)
	}
	return photoList, nil
}

func (a *api) stagePhoto(fileHeader *multipart.FileHeader) (validatedPhoto, error) {
	file, err := fileHeader.Open()
	if err != nil {
		return validatedPhoto{}, err
	}
	data, readErr := io.ReadAll(io.LimitReader(file, maximumPhotoSizeBytes+1))
	closeErr := file.Close()
	if readErr != nil {
		return validatedPhoto{}, readErr
	}
	if closeErr != nil {
		return validatedPhoto{}, closeErr
	}
	if len(data) == 0 {
		return validatedPhoto{}, photoHTTPError{http.StatusUnprocessableEntity, "invalid_photo", "The photo is empty or invalid"}
	}
	if len(data) > maximumPhotoSizeBytes {
		return validatedPhoto{}, photoHTTPError{http.StatusRequestEntityTooLarge, "photo_too_large", "Each photo must be at most 7 MB"}
	}
	contentType := http.DetectContentType(data)
	extension := ""
	switch contentType {
	case "image/jpeg":
		extension = ".jpg"
	case "image/png":
		extension = ".png"
	case "image/webp":
		extension = ".webp"
	default:
		return validatedPhoto{}, photoHTTPError{http.StatusUnsupportedMediaType, "unsupported_photo", "Photos must be JPEG, PNG or WebP images"}
	}
	width, height, err := decodeImageDimensions(data, contentType)
	if err != nil || width <= 0 || height <= 0 || width > maximumPhotoDimension || height > maximumPhotoDimension {
		return validatedPhoto{}, photoHTTPError{http.StatusUnprocessableEntity, "invalid_photo", "Photos must be valid images no larger than 4096 by 4096 pixels"}
	}
	storageToken, err := randomToken(24)
	if err != nil {
		return validatedPhoto{}, err
	}
	temporaryFile, err := os.CreateTemp(a.photoStoragePath, ".photo-upload-*")
	if err != nil {
		return validatedPhoto{}, err
	}
	temporaryPath := temporaryFile.Name()
	if _, err := temporaryFile.Write(data); err != nil {
		temporaryFile.Close()
		_ = os.Remove(temporaryPath)
		return validatedPhoto{}, err
	}
	if err := temporaryFile.Close(); err != nil {
		_ = os.Remove(temporaryPath)
		return validatedPhoto{}, err
	}
	return validatedPhoto{
		temporaryPath: temporaryPath,
		storageKey:    storageToken + extension,
		contentType:   contentType,
		width:         width,
		height:        height,
		size:          int64(len(data)),
	}, nil
}

func lockPhotoPreparation(
	ctx context.Context,
	tx pgx.Tx,
	player authenticatedPlayer,
	requireEditable bool,
) (string, string, error) {
	var readyStatus, lobbyStatus string
	err := tx.QueryRow(ctx, `
		SELECT player.ready_status, lobby.status
		FROM players AS player
		JOIN lobbies AS lobby ON lobby.id = player.lobby_id
		WHERE player.id = $1 AND lobby.id = $2
		FOR UPDATE OF player, lobby
	`, player.ID, player.LobbyID).Scan(&readyStatus, &lobbyStatus)
	if err != nil {
		return "", "", err
	}
	if lobbyStatus != "waiting_for_players" && lobbyStatus != "preparing_photos" &&
		lobbyStatus != "ready_to_start" && lobbyStatus != "in_game" {
		return "", "", photoHTTPError{http.StatusConflict, "photos_locked", "Photos can no longer be changed"}
	}
	if requireEditable && readyStatus == "ready" {
		return "", "", photoHTTPError{http.StatusConflict, "photos_locked", "Photos have already been validated"}
	}
	return readyStatus, lobbyStatus, nil
}

func updatePhotoReadiness(ctx context.Context, tx pgx.Tx, player authenticatedPlayer, readyStatus string) error {
	if _, err := tx.Exec(ctx, `
		UPDATE players SET ready_status = $1, updated_at = now() WHERE id = $2
	`, readyStatus, player.ID); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `
		UPDATE lobbies
		SET status = CASE
		      WHEN status = 'in_game' THEN status
		      WHEN (SELECT count(*) FROM players WHERE lobby_id = $1 AND excluded_at IS NULL) < $2
		        THEN 'waiting_for_players'::lobby_status
		      WHEN NOT (SELECT bool_and(ready_status = 'ready') FROM players WHERE lobby_id = $1 AND excluded_at IS NULL)
		        THEN 'preparing_photos'::lobby_status
		      ELSE 'ready_to_start'::lobby_status
		    END,
		    revision = revision + 1,
		    updated_at = now()
		WHERE id = $1
	`, player.LobbyID, minimumPlayerCount)
	return err
}

func playerPhotoPosition(r *http.Request) (int, error) {
	position, err := strconv.Atoi(r.PathValue("position"))
	if err != nil || position < 1 || position > 4 {
		return 0, photoHTTPError{http.StatusUnprocessableEntity, "invalid_photo_position", "Photo position must be between 1 and 4"}
	}
	return position, nil
}

func removeTemporaryPhotos(photoList []validatedPhoto) {
	for _, photo := range photoList {
		_ = os.Remove(photo.temporaryPath)
	}
}

func removePhotoPaths(pathList []string) {
	for _, path := range pathList {
		_ = os.Remove(path)
	}
}

func (a *api) moveStagedPhotos(photoList []validatedPhoto) ([]string, error) {
	movedPaths := make([]string, 0, len(photoList))
	for _, photo := range photoList {
		finalPath := filepath.Join(a.photoStoragePath, photo.storageKey)
		if err := os.Rename(photo.temporaryPath, finalPath); err != nil {
			removePhotoPaths(movedPaths)
			return nil, err
		}
		movedPaths = append(movedPaths, finalPath)
	}
	return movedPaths, nil
}

func (a *api) writePhotoFailure(w http.ResponseWriter, r *http.Request, err error) {
	var requestError photoHTTPError
	if errors.As(err, &requestError) {
		writeError(w, requestError.status, requestError.code, requestError.message)
		return
	}
	a.internalError(w, r, err)
}

func (a *api) writePhotoState(w http.ResponseWriter, r *http.Request, player authenticatedPlayer, status int) {
	a.writeLobbyStateResponse(w, r, player, status)
}

// writeLobbyStateResponse reloads the full lobby state, publishes the update
// to every connected client, and writes the state as the HTTP response. It is
// the common tail of any handler that mutates lobby/game state, regardless of
// game mode.
func (a *api) writeLobbyStateResponse(w http.ResponseWriter, r *http.Request, player authenticatedPlayer, status int) {
	state, err := a.loadLobbyState(r.Context(), player)
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	a.hub.publish(player.Code)
	writeJSON(w, status, state)
}

func decodeImageDimensions(data []byte, contentType string) (int, int, error) {
	switch contentType {
	case "image/png":
		if len(data) < 45 || !bytes.Equal(data[:8], []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a}) {
			return 0, 0, errors.New("invalid PNG header")
		}
		width, height := 0, 0
		hasImageData := false
		for offset := 8; offset+12 <= len(data); {
			chunkLength := int(binary.BigEndian.Uint32(data[offset : offset+4]))
			if chunkLength < 0 || chunkLength > len(data)-offset-12 {
				return 0, 0, errors.New("invalid PNG chunk length")
			}
			chunkType := string(data[offset+4 : offset+8])
			chunkEnd := offset + 12 + chunkLength
			expectedCRC := binary.BigEndian.Uint32(data[offset+8+chunkLength : chunkEnd])
			if pngCRC32(data[offset+4:offset+8+chunkLength]) != expectedCRC {
				return 0, 0, errors.New("invalid PNG chunk checksum")
			}
			switch chunkType {
			case "IHDR":
				if offset != 8 || chunkLength != 13 {
					return 0, 0, errors.New("invalid PNG image header")
				}
				width = int(binary.BigEndian.Uint32(data[offset+8 : offset+12]))
				height = int(binary.BigEndian.Uint32(data[offset+12 : offset+16]))
			case "IDAT":
				hasImageData = hasImageData || chunkLength > 0
			case "IEND":
				if chunkLength != 0 || chunkEnd != len(data) || width <= 0 || height <= 0 || !hasImageData {
					return 0, 0, errors.New("incomplete PNG image")
				}
				return width, height, nil
			}
			offset = chunkEnd
		}
		return 0, 0, errors.New("PNG end marker not found")
	case "image/jpeg":
		if len(data) < 4 || data[0] != 0xff || data[1] != 0xd8 {
			return 0, 0, errors.New("invalid JPEG header")
		}
		width, height := 0, 0
		for offset := 2; offset+4 <= len(data); {
			if data[offset] != 0xff {
				return 0, 0, errors.New("invalid JPEG marker")
			}
			for offset < len(data) && data[offset] == 0xff {
				offset++
			}
			if offset >= len(data) {
				break
			}
			marker := data[offset]
			offset++
			if marker == 0xd8 || marker == 0x01 || marker >= 0xd0 && marker <= 0xd7 {
				continue
			}
			if marker == 0xd9 {
				if width > 0 && height > 0 {
					return width, height, nil
				}
				return 0, 0, errors.New("JPEG dimensions not found")
			}
			if offset+2 > len(data) {
				break
			}
			segmentLength := int(binary.BigEndian.Uint16(data[offset : offset+2]))
			if segmentLength < 2 || offset+segmentLength > len(data) {
				break
			}
			if (marker >= 0xc0 && marker <= 0xc3) || (marker >= 0xc5 && marker <= 0xc7) ||
				(marker >= 0xc9 && marker <= 0xcb) || (marker >= 0xcd && marker <= 0xcf) {
				if segmentLength < 7 {
					break
				}
				height = int(binary.BigEndian.Uint16(data[offset+3 : offset+5]))
				width = int(binary.BigEndian.Uint16(data[offset+5 : offset+7]))
			}
			if marker == 0xda {
				if width > 0 && height > 0 && bytes.LastIndex(data[offset+segmentLength:], []byte{0xff, 0xd9}) >= 0 {
					return width, height, nil
				}
				return 0, 0, errors.New("incomplete JPEG image")
			}
			offset += segmentLength
		}
		return 0, 0, errors.New("incomplete JPEG image")
	case "image/webp":
		if len(data) < 28 || string(data[:4]) != "RIFF" || string(data[8:12]) != "WEBP" ||
			int(binary.LittleEndian.Uint32(data[4:8]))+8 != len(data) {
			return 0, 0, errors.New("invalid WebP header")
		}
		width, height := 0, 0
		for offset := 12; offset+8 <= len(data); {
			chunkSize := int(binary.LittleEndian.Uint32(data[offset+4 : offset+8]))
			paddedSize := chunkSize + chunkSize%2
			if chunkSize < 0 || paddedSize > len(data)-offset-8 {
				return 0, 0, errors.New("invalid WebP chunk length")
			}
			chunkData := data[offset+8 : offset+8+chunkSize]
			switch string(data[offset : offset+4]) {
			case "VP8X":
				if len(chunkData) < 10 {
					return 0, 0, errors.New("invalid extended WebP header")
				}
				width = 1 + int(chunkData[4]) + int(chunkData[5])<<8 + int(chunkData[6])<<16
				height = 1 + int(chunkData[7]) + int(chunkData[8])<<8 + int(chunkData[9])<<16
			case "VP8L":
				if len(chunkData) < 5 || chunkData[0] != 0x2f {
					return 0, 0, errors.New("invalid lossless WebP header")
				}
				bits := binary.LittleEndian.Uint32(chunkData[1:5])
				width = int(bits&0x3fff) + 1
				height = int((bits>>14)&0x3fff) + 1
			case "VP8 ":
				if len(chunkData) < 10 || chunkData[3] != 0x9d || chunkData[4] != 0x01 || chunkData[5] != 0x2a {
					return 0, 0, errors.New("invalid lossy WebP header")
				}
				width = int(binary.LittleEndian.Uint16(chunkData[6:8]) & 0x3fff)
				height = int(binary.LittleEndian.Uint16(chunkData[8:10]) & 0x3fff)
			}
			offset += 8 + paddedSize
		}
		if width > 0 && height > 0 {
			return width, height, nil
		}
		return 0, 0, errors.New("WebP dimensions not found")
	}
	return 0, 0, errors.New("unsupported image format")
}

func pngCRC32(data []byte) uint32 {
	crc := ^uint32(0)
	for _, value := range data {
		crc ^= uint32(value)
		for range 8 {
			mask := uint32(0) - (crc & 1)
			crc = crc>>1 ^ (0xedb88320 & mask)
		}
	}
	return ^crc
}

func (a *api) handleGetPlayerPhoto(w http.ResponseWriter, r *http.Request) {
	player, ok := a.requirePlayer(w, r)
	if !ok {
		return
	}
	var storageKey, contentType string
	var createdAt time.Time
	err := a.pool.QueryRow(r.Context(), `
		SELECT photo.storage_key, photo.content_type, photo.created_at
		FROM player_photos AS photo
		JOIN players AS owner ON owner.id = photo.player_id
		JOIN lobbies AS lobby ON lobby.id = owner.lobby_id
		WHERE photo.id = $1 AND owner.lobby_id = $2
		  AND (owner.excluded_at IS NULL OR lobby.status = 'completed')
	`, r.PathValue("photoID"), player.LobbyID).Scan(&storageKey, &contentType, &createdAt)
	if err != nil {
		writeError(w, http.StatusNotFound, "photo_not_found", "Photo not found")
		return
	}
	if filepath.Base(storageKey) != storageKey {
		a.internalError(w, r, errors.New("unsafe photo storage key"))
		return
	}
	file, err := os.Open(filepath.Join(a.photoStoragePath, storageKey))
	if errors.Is(err, os.ErrNotExist) {
		writeError(w, http.StatusNotFound, "photo_not_found", "Photo not found")
		return
	}
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	defer file.Close()
	fileInfo, err := file.Stat()
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "private, max-age=3600")
	http.ServeContent(w, r, "", createdAt, io.NewSectionReader(file, 0, fileInfo.Size()))
}

func (a *api) startMaintenance(ctx context.Context) {
	if _, err := a.pool.Exec(ctx, `
		WITH disconnected AS (
		  UPDATE players AS player
		  SET disconnected_at = now(), updated_at = now()
		  FROM lobbies AS lobby
		  WHERE player.lobby_id = lobby.id
		    AND player.excluded_at IS NULL
		    AND player.disconnected_at IS NULL
		    AND lobby.status <> 'expired'
		    AND lobby.expires_at > now()
		  RETURNING player.lobby_id
		)
		UPDATE lobbies
		SET revision = revision + 1, updated_at = now()
		WHERE id IN (SELECT lobby_id FROM disconnected)
	`); err != nil {
		a.logger.Error("unable to reset presence after startup", "error", err)
	}
	go func() {
		a.cleanupExpiredLobbies(ctx)
		a.cleanupOrphanedPhotoFiles(ctx)
		cleanupTicker := time.NewTicker(time.Minute)
		orphanTicker := time.NewTicker(time.Hour)
		presenceTicker := time.NewTicker(5 * time.Second)
		defer cleanupTicker.Stop()
		defer orphanTicker.Stop()
		defer presenceTicker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-cleanupTicker.C:
				a.cleanupExpiredLobbies(ctx)
			case <-orphanTicker.C:
				a.cleanupOrphanedPhotoFiles(ctx)
			case <-presenceTicker.C:
				a.transferEligibleHosts(ctx)
			}
		}
	}()
}

func (a *api) transferEligibleHosts(ctx context.Context) {
	rows, err := a.pool.Query(ctx, `
		SELECT lobby.code, player.id, player.disconnected_at
		FROM players AS player
		JOIN lobbies AS lobby ON lobby.id = player.lobby_id
		WHERE player.is_host
		  AND player.excluded_at IS NULL
		  AND player.disconnected_at <= now() - interval '90 seconds'
		  AND lobby.status NOT IN ('completed', 'expired')
		  AND lobby.expires_at > now()
	`)
	if err != nil {
		a.logger.Error("unable to inspect disconnected hosts", "error", err)
		return
	}
	type disconnectedHost struct {
		code           string
		playerID       string
		disconnectedAt time.Time
	}
	hostList := make([]disconnectedHost, 0)
	for rows.Next() {
		var host disconnectedHost
		if err := rows.Scan(&host.code, &host.playerID, &host.disconnectedAt); err != nil {
			rows.Close()
			a.logger.Error("unable to scan disconnected host", "error", err)
			return
		}
		hostList = append(hostList, host)
	}
	rows.Close()
	for _, host := range hostList {
		a.hub.transferDisconnectedHost(host.code, host.playerID, host.disconnectedAt)
	}
}

func (a *api) cleanupExpiredLobbies(ctx context.Context) {
	disconnectedCutoff := time.Now().UTC().Add(-a.lobbyEmptyGrace)
	expiredTag, err := a.pool.Exec(ctx, `
		UPDATE lobbies AS lobby
		SET status = 'expired', expires_at = LEAST(expires_at, now()),
		    revision = revision + 1, updated_at = now()
		WHERE lobby.status <> 'expired'
		  AND (
		    lobby.expires_at <= now()
		    OR NOT EXISTS (
		      SELECT 1
		      FROM players AS player
		      WHERE player.lobby_id = lobby.id
		        AND player.excluded_at IS NULL
		        AND (player.disconnected_at IS NULL OR player.disconnected_at > $1)
		    )
		  )
	`, disconnectedCutoff)
	if err != nil {
		a.logger.Error("lobby expiration scan failed", "error", err)
		return
	}
	rows, err := a.pool.Query(ctx, `SELECT id::text FROM lobbies WHERE status = 'expired'`)
	if err != nil {
		a.logger.Error("expired lobby lookup failed", "error", err)
		return
	}
	lobbyIDs := make([]string, 0)
	for rows.Next() {
		var lobbyID string
		if err := rows.Scan(&lobbyID); err != nil {
			rows.Close()
			a.logger.Error("expired lobby scan failed", "error", err)
			return
		}
		lobbyIDs = append(lobbyIDs, lobbyID)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		a.logger.Error("expired lobby iteration failed", "error", err)
		return
	}
	rows.Close()

	deletedCount := 0
	for _, lobbyID := range lobbyIDs {
		if a.purgeLobby(ctx, lobbyID) {
			deletedCount++
		}
	}
	if expiredTag.RowsAffected() > 0 || deletedCount > 0 {
		a.logger.Info("lobby cleanup completed", "expired", expiredTag.RowsAffected(), "deleted", deletedCount)
	}
}

func (a *api) purgeLobby(ctx context.Context, lobbyID string) bool {
	rows, err := a.pool.Query(ctx, `
		SELECT photo.storage_key
		FROM player_photos AS photo
		JOIN players AS player ON player.id = photo.player_id
		JOIN lobbies AS lobby ON lobby.id = player.lobby_id
		WHERE lobby.id = $1 AND lobby.status = 'expired'
	`, lobbyID)
	if err != nil {
		a.logger.Error("expired photo lookup failed", "error", err)
		return false
	}
	storageKeys := make([]string, 0)
	for rows.Next() {
		var storageKey string
		if err := rows.Scan(&storageKey); err != nil {
			rows.Close()
			a.logger.Error("expired photo scan failed", "error", err)
			return false
		}
		storageKeys = append(storageKeys, storageKey)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		a.logger.Error("expired photo iteration failed", "error", err)
		return false
	}
	rows.Close()
	if err := removeStoredPhotoFiles(a.photoStoragePath, storageKeys, os.Remove); err != nil {
		a.logger.Error("expired photo file removal failed")
		return false
	}
	tag, err := a.pool.Exec(ctx, `DELETE FROM lobbies WHERE id = $1 AND status = 'expired'`, lobbyID)
	if err != nil {
		a.logger.Error("expired lobby deletion failed", "error", err)
		return false
	}
	return tag.RowsAffected() > 0
}

func removeStoredPhotoFiles(storagePath string, storageKeys []string, removeFile func(string) error) error {
	for _, storageKey := range storageKeys {
		if filepath.Base(storageKey) != storageKey || !storedPhotoFilePattern.MatchString(storageKey) {
			return errors.New("invalid photo storage key")
		}
		if err := removeFile(filepath.Join(storagePath, storageKey)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return errors.New("photo file removal failed")
		}
	}
	return nil
}

func (a *api) cleanupOrphanedPhotoFiles(ctx context.Context) {
	rows, err := a.pool.Query(ctx, `SELECT storage_key FROM player_photos`)
	if err != nil {
		a.logger.Error("photo reference lookup failed", "error", err)
		return
	}
	referencedKeys := make(map[string]struct{})
	for rows.Next() {
		var storageKey string
		if err := rows.Scan(&storageKey); err != nil {
			rows.Close()
			a.logger.Error("photo reference scan failed", "error", err)
			return
		}
		referencedKeys[storageKey] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		a.logger.Error("photo reference iteration failed", "error", err)
		return
	}
	rows.Close()

	removedCount, failureCount, err := removeStaleOrphanPhotoFiles(
		a.photoStoragePath,
		referencedKeys,
		time.Now().Add(-photoOrphanGracePeriod),
		os.Remove,
	)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		a.logger.Error("photo storage scan failed", "error", err)
		return
	}
	if removedCount > 0 || failureCount > 0 {
		a.logger.Info("orphan photo cleanup completed", "deleted", removedCount, "failures", failureCount)
	}
}

func removeStaleOrphanPhotoFiles(
	storagePath string,
	referencedKeys map[string]struct{},
	cutoff time.Time,
	removeFile func(string) error,
) (int, int, error) {
	entries, err := os.ReadDir(storagePath)
	if err != nil {
		return 0, 0, err
	}
	removedCount := 0
	failureCount := 0
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		isTemporary := strings.HasPrefix(name, ".photo-upload-")
		isStoredPhoto := storedPhotoFilePattern.MatchString(name)
		if !isTemporary && !isStoredPhoto {
			continue
		}
		if isStoredPhoto {
			if _, referenced := referencedKeys[name]; referenced {
				continue
			}
		}
		info, err := entry.Info()
		if err != nil {
			failureCount++
			continue
		}
		if info.ModTime().After(cutoff) {
			continue
		}
		if err := removeFile(filepath.Join(storagePath, name)); err != nil && !errors.Is(err, os.ErrNotExist) {
			failureCount++
			continue
		}
		removedCount++
	}
	return removedCount, failureCount, nil
}
