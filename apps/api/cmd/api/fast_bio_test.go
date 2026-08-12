package main

import (
	"reflect"
	"testing"
)

func TestTallyFastBioThemeRankingPicksTopThreeByBordaCount(t *testing.T) {
	candidates := []string{"Mystérieux", "Aventurier", "Glamour", "Intello"}
	rankings := [][]string{
		{"Mystérieux", "Aventurier", "Glamour", "Intello"},
		{"Mystérieux", "Glamour", "Aventurier", "Intello"},
		{"Aventurier", "Mystérieux", "Intello", "Glamour"},
	}
	// Borda weights (4 candidates): 1st=4, 2nd=3, 3rd=2, 4th=1.
	// Mystérieux: 4+4+3=11, Aventurier: 3+2+4=9, Glamour: 2+3+1=6, Intello: 1+1+2=4.
	want := []string{"Mystérieux", "Aventurier", "Glamour"}
	got := tallyFastBioThemeRanking(candidates, rankings)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("tallyFastBioThemeRanking() = %v, want %v", got, want)
	}
}

func TestTallyFastBioThemeRankingBreaksTiesByCandidateOrder(t *testing.T) {
	candidates := []string{"A", "B", "C"}
	// No rankings at all: every candidate has score 0, so the original order wins.
	got := tallyFastBioThemeRanking(candidates, nil)
	want := []string{"A", "B", "C"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("tallyFastBioThemeRanking() = %v, want %v", got, want)
	}
}

func TestTallyFastBioThemeRankingIgnoresUnknownEntries(t *testing.T) {
	candidates := []string{"A", "B", "C"}
	rankings := [][]string{
		{"B", "unknown-theme", "A", "C"},
	}
	got := tallyFastBioThemeRanking(candidates, rankings)
	want := []string{"B", "A", "C"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("tallyFastBioThemeRanking() = %v, want %v", got, want)
	}
}

func TestRankingMatchesCandidates(t *testing.T) {
	candidates := []string{"Mystérieux", "Aventurier", "Glamour"}
	testCases := []struct {
		name    string
		ranking []string
		want    bool
	}{
		{name: "exact permutation", ranking: []string{"Aventurier", "Glamour", "Mystérieux"}, want: true},
		{name: "case insensitive match", ranking: []string{"aventurier", "GLAMOUR", "mystérieux"}, want: true},
		{name: "missing an entry", ranking: []string{"Aventurier", "Glamour"}, want: false},
		{name: "duplicate entry", ranking: []string{"Aventurier", "Aventurier", "Glamour"}, want: false},
		{name: "unknown entry", ranking: []string{"Aventurier", "Glamour", "Inconnu"}, want: false},
		{name: "empty ranking against empty candidates", ranking: []string{}, want: false},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := rankingMatchesCandidates(testCase.ranking, candidates); got != testCase.want {
				t.Fatalf("rankingMatchesCandidates() = %v, want %v", got, testCase.want)
			}
		})
	}
}

func TestAssignFastBioTargetsIsADerangement(t *testing.T) {
	playerIDs := []string{"p1", "p2", "p3", "p4", "p5", "p6"}
	for attempt := 0; attempt < 20; attempt++ {
		assignments, err := assignFastBioTargets(playerIDs)
		if err != nil {
			t.Fatalf("assignFastBioTargets() error = %v", err)
		}
		if len(assignments) != len(playerIDs) {
			t.Fatalf("assignFastBioTargets() returned %d assignments, want %d", len(assignments), len(playerIDs))
		}
		targetCount := make(map[string]int, len(playerIDs))
		for author, target := range assignments {
			if author == target {
				t.Fatalf("assignFastBioTargets() assigned %s to themselves", author)
			}
			targetCount[target]++
		}
		for _, playerID := range playerIDs {
			if targetCount[playerID] != 1 {
				t.Fatalf("assignFastBioTargets() player %s is targeted %d times, want exactly 1", playerID, targetCount[playerID])
			}
		}
	}
}

func TestAssignFastBioTargetsRejectsFewerThanTwoPlayers(t *testing.T) {
	if _, err := assignFastBioTargets([]string{"solo"}); err == nil {
		t.Fatal("assignFastBioTargets() expected an error for a single player, got nil")
	}
}
