package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

type validatedPhoto struct {
	temporaryPath string
	storageKey    string
	contentType   string
	width         int
	height        int
	size          int64
	id            string
}

func (a *api) handleUploadPlayerPhotos(w http.ResponseWriter, r *http.Request) {
	player, ok := a.requirePlayer(w, r)
	if !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maximumPhotoSizeBytes*4+(1<<20))
	if err := r.ParseMultipartForm(maximumPhotoSizeBytes * 4); err != nil {
		writeError(w, http.StatusRequestEntityTooLarge, "photos_too_large", "The four photos must each be at most 7 MB")
		return
	}
	fileHeaders := r.MultipartForm.File["photos"]
	if len(fileHeaders) != 4 {
		writeError(w, http.StatusUnprocessableEntity, "invalid_photo_count", "Exactly four photos are required")
		return
	}
	if err := os.MkdirAll(a.photoStoragePath, 0o750); err != nil {
		a.internalError(w, r, err)
		return
	}

	photoList := make([]validatedPhoto, 0, 4)
	cleanupTemporary := func() {
		for _, photo := range photoList {
			_ = os.Remove(photo.temporaryPath)
		}
	}
	for _, fileHeader := range fileHeaders {
		file, err := fileHeader.Open()
		if err != nil {
			cleanupTemporary()
			a.internalError(w, r, err)
			return
		}
		data, readErr := io.ReadAll(io.LimitReader(file, maximumPhotoSizeBytes+1))
		file.Close()
		if readErr != nil {
			cleanupTemporary()
			a.internalError(w, r, readErr)
			return
		}
		if len(data) == 0 || len(data) > maximumPhotoSizeBytes {
			cleanupTemporary()
			writeError(w, http.StatusRequestEntityTooLarge, "photo_too_large", "Each photo must be at most 7 MB")
			return
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
			cleanupTemporary()
			writeError(w, http.StatusUnsupportedMediaType, "unsupported_photo", "Photos must be JPEG, PNG or WebP images")
			return
		}
		width, height, err := decodeImageDimensions(data, contentType)
		if err != nil || width <= 0 || height <= 0 ||
			width > maximumPhotoDimension || height > maximumPhotoDimension {
			cleanupTemporary()
			writeError(w, http.StatusUnprocessableEntity, "invalid_photo", "Photos must be valid images no larger than 4096 by 4096 pixels")
			return
		}
		storageToken, err := randomToken(24)
		if err != nil {
			cleanupTemporary()
			a.internalError(w, r, err)
			return
		}
		temporaryFile, err := os.CreateTemp(a.photoStoragePath, ".photo-upload-*")
		if err != nil {
			cleanupTemporary()
			a.internalError(w, r, err)
			return
		}
		if _, err := temporaryFile.Write(data); err != nil {
			temporaryFile.Close()
			_ = os.Remove(temporaryFile.Name())
			cleanupTemporary()
			a.internalError(w, r, err)
			return
		}
		if err := temporaryFile.Close(); err != nil {
			_ = os.Remove(temporaryFile.Name())
			cleanupTemporary()
			a.internalError(w, r, err)
			return
		}
		photoList = append(photoList, validatedPhoto{
			temporaryPath: temporaryFile.Name(),
			storageKey:    storageToken + extension,
			contentType:   contentType,
			width:         width,
			height:        height,
			size:          int64(len(data)),
		})
	}

	tx, err := a.pool.Begin(r.Context())
	if err != nil {
		cleanupTemporary()
		a.internalError(w, r, err)
		return
	}
	defer tx.Rollback(r.Context())
	rows, err := tx.Query(r.Context(), `SELECT storage_key FROM player_photos WHERE player_id = $1`, player.ID)
	if err != nil {
		cleanupTemporary()
		a.internalError(w, r, err)
		return
	}
	oldStorageKeys := make([]string, 0, 4)
	for rows.Next() {
		var storageKey string
		if err := rows.Scan(&storageKey); err != nil {
			rows.Close()
			cleanupTemporary()
			a.internalError(w, r, err)
			return
		}
		oldStorageKeys = append(oldStorageKeys, storageKey)
	}
	rows.Close()
	if _, err := tx.Exec(r.Context(), `DELETE FROM player_photos WHERE player_id = $1`, player.ID); err != nil {
		cleanupTemporary()
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
			cleanupTemporary()
			a.internalError(w, r, err)
			return
		}
	}
	if _, err := tx.Exec(r.Context(), `
		UPDATE players SET ready_status = 'ready', updated_at = now() WHERE id = $1
	`, player.ID); err != nil {
		cleanupTemporary()
		a.internalError(w, r, err)
		return
	}
	if _, err := tx.Exec(r.Context(), `
		UPDATE lobbies
		SET status = CASE
		      WHEN (SELECT count(*) FROM players WHERE lobby_id = $1 AND excluded_at IS NULL) < $2
		        THEN 'waiting_for_players'::lobby_status
		      WHEN NOT (SELECT bool_and(ready_status = 'ready') FROM players WHERE lobby_id = $1 AND excluded_at IS NULL)
		        THEN 'preparing_photos'::lobby_status
		      ELSE 'ready_to_start'::lobby_status
		    END,
		    revision = revision + 1,
		    updated_at = now()
		WHERE id = $1 AND status IN ('waiting_for_players', 'preparing_photos', 'ready_to_start')
	`, player.LobbyID, minimumPlayerCount); err != nil {
		cleanupTemporary()
		a.internalError(w, r, err)
		return
	}

	movedPaths := make([]string, 0, 4)
	for _, photo := range photoList {
		finalPath := filepath.Join(a.photoStoragePath, photo.storageKey)
		if err := os.Rename(photo.temporaryPath, finalPath); err != nil {
			for _, movedPath := range movedPaths {
				_ = os.Remove(movedPath)
			}
			cleanupTemporary()
			a.internalError(w, r, err)
			return
		}
		movedPaths = append(movedPaths, finalPath)
	}
	if err := tx.Commit(r.Context()); err != nil {
		for _, movedPath := range movedPaths {
			_ = os.Remove(movedPath)
		}
		a.internalError(w, r, err)
		return
	}
	for _, storageKey := range oldStorageKeys {
		_ = os.Remove(filepath.Join(a.photoStoragePath, filepath.Base(storageKey)))
	}
	state, err := a.loadLobbyState(r.Context(), player)
	if err != nil {
		a.internalError(w, r, err)
		return
	}
	a.hub.publish(player.Code)
	writeJSON(w, http.StatusOK, state)
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
		WHERE photo.id = $1 AND owner.lobby_id = $2 AND owner.excluded_at IS NULL
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
		cleanupTicker := time.NewTicker(time.Hour)
		presenceTicker := time.NewTicker(5 * time.Second)
		defer cleanupTicker.Stop()
		defer presenceTicker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-cleanupTicker.C:
				a.cleanupExpiredLobbies(ctx)
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
	rows, err := a.pool.Query(ctx, `
		SELECT photo.storage_key
		FROM player_photos AS photo
		JOIN players AS player ON player.id = photo.player_id
		JOIN lobbies AS lobby ON lobby.id = player.lobby_id
		WHERE lobby.expires_at <= now()
	`)
	if err != nil {
		a.logger.Error("expired photo lookup failed", "error", err)
		return
	}
	storageKeys := make([]string, 0)
	for rows.Next() {
		var storageKey string
		if err := rows.Scan(&storageKey); err == nil {
			storageKeys = append(storageKeys, storageKey)
		}
	}
	rows.Close()
	if _, err := a.pool.Exec(ctx, `DELETE FROM lobbies WHERE expires_at <= now()`); err != nil {
		a.logger.Error("expired lobby cleanup failed", "error", err)
		return
	}
	for _, storageKey := range storageKeys {
		if filepath.Base(storageKey) == storageKey {
			_ = os.Remove(filepath.Join(a.photoStoragePath, storageKey))
		}
	}
}
