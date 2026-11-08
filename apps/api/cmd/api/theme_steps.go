package main

import (
	"context"
	"time"
)

// themeStep is one of the two sub-phases of the shared theme selection.
type themeStep int

const (
	themeProposalStep themeStep = iota
	themeRankingStep
)

// themeStepWindow is how long players get for one theme step. A lobby's
// first theme selection gets the full window; later ones (Wavelength rounds
// 2+, replays) are shorter, since everyone has already proposed once.
func themeStepWindow(step themeStep, firstThemeSelection bool) time.Duration {
	if firstThemeSelection {
		return themeStepFirstWindow
	}
	if step == themeRankingStep {
		return themeRankingRepeatWindow
	}
	return themeProposalRepeatWindow
}

// themeRankingDeadline is when the ranking step about to open must close.
// gamesTable is always one of the hardcoded per-mode game tables (never user
// input); laterRound is true for a Wavelength round after the first.
func themeRankingDeadline(ctx context.Context, db dbQuerier, gamesTable, lobbyID, gameID string, laterRound bool) (time.Time, error) {
	var hasEarlierGame bool
	if err := db.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM `+gamesTable+` WHERE lobby_id = $1 AND id <> $2)
	`, lobbyID, gameID).Scan(&hasEarlierGame); err != nil {
		return time.Time{}, err
	}
	firstThemeSelection := !hasEarlierGame && !laterRound
	return time.Now().Add(themeStepWindow(themeRankingStep, firstThemeSelection)), nil
}

// phaseTimers collects the deadline timers a transaction opened: they may
// only start once it has committed.
type phaseTimers []func()

func (timers *phaseTimers) add(start func()) {
	*timers = append(*timers, start)
}

func (timers phaseTimers) start() {
	for _, start := range timers {
		start()
	}
}
