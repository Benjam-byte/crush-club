package main

import (
	"slices"
	"testing"
	"time"
)

func TestSecretRankingRankRanges(t *testing.T) {
	testCases := []struct {
		name    string
		amounts []int64
		want    []secretRankingRankRange
	}{
		{
			name:    "distinct amounts",
			amounts: []int64{500, 10, 2000},
			want:    []secretRankingRankRange{{2, 2}, {1, 1}, {3, 3}},
		},
		{
			name:    "a tie shares its places",
			amounts: []int64{100, 50, 100, 1000},
			want:    []secretRankingRankRange{{2, 3}, {1, 1}, {2, 3}, {4, 4}},
		},
		{
			name:    "everyone equal",
			amounts: []int64{0, 0, 0},
			want:    []secretRankingRankRange{{1, 3}, {1, 3}, {1, 3}},
		},
		{
			name:    "no amounts",
			amounts: nil,
			want:    []secretRankingRankRange{},
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := secretRankingRankRanges(testCase.amounts); !slices.Equal(got, testCase.want) {
				t.Fatalf("secretRankingRankRanges(%v) = %v, want %v", testCase.amounts, got, testCase.want)
			}
		})
	}
}

func TestSecretRankingRankRangeContains(t *testing.T) {
	tie := secretRankingRankRange{min: 2, max: 3}
	for position, want := range map[int]bool{0: false, 1: false, 2: true, 3: true, 4: false} {
		if got := tie.contains(position); got != want {
			t.Errorf("contains(%d) = %v, want %v", position, got, want)
		}
	}
}

func TestIsPositionPermutation(t *testing.T) {
	testCases := []struct {
		name      string
		positions []int
		want      bool
	}{
		{name: "in order", positions: []int{1, 2, 3}, want: true},
		{name: "shuffled", positions: []int{3, 1, 4, 2}, want: true},
		{name: "duplicate", positions: []int{1, 2, 2}, want: false},
		{name: "phone not numbered", positions: []int{1, 0, 2}, want: false},
		{name: "number above the count", positions: []int{1, 2, 4}, want: false},
		{name: "empty table", positions: nil, want: false},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := isPositionPermutation(testCase.positions); got != testCase.want {
				t.Fatalf("isPositionPermutation(%v) = %v, want %v", testCase.positions, got, testCase.want)
			}
		})
	}
}

func TestSecretRankingRevealEndsAt(t *testing.T) {
	revealAt := time.Date(2026, 11, 8, 20, 0, 0, 0, time.UTC)
	// Phone #4 flips three steps after phone #1, then the verdict plays.
	want := revealAt.Add(3*secretRankingRevealStep + secretRankingRevealFinale)
	if got := secretRankingRevealEndsAt(revealAt, 4); !got.Equal(want) {
		t.Fatalf("secretRankingRevealEndsAt(4) = %v, want %v", got, want)
	}
	if got := secretRankingRevealEndsAt(revealAt, 0); !got.Equal(revealAt.Add(secretRankingRevealFinale)) {
		t.Fatalf("secretRankingRevealEndsAt(0) = %v", got)
	}
}

func TestSecretRankingExtremesKeepTies(t *testing.T) {
	entries := []secretRankingEntryRow{
		{PlayerID: "ana", Amount: 50},
		{PlayerID: "tom", Amount: 9000},
		{PlayerID: "lea", Amount: 50},
		{PlayerID: "max", Amount: 9000},
		{PlayerID: "eve", Amount: 300},
	}
	smallest, largest := secretRankingExtremes(entries)
	ids := func(rows []secretRankingEntryRow) []string {
		result := make([]string, 0, len(rows))
		for _, row := range rows {
			result = append(result, row.PlayerID)
		}
		return result
	}
	if got := ids(smallest); !slices.Equal(got, []string{"ana", "lea"}) {
		t.Fatalf("smallest = %v", got)
	}
	if got := ids(largest); !slices.Equal(got, []string{"tom", "max"}) {
		t.Fatalf("largest = %v", got)
	}
}

func TestMergeSecretRankingCandidates(t *testing.T) {
	defaults := []string{"manger un insecte", "vivre sans internet", "démissionner", "chanter"}
	testCases := []struct {
		name      string
		proposals []string
		want      []string
	}{
		{
			name:      "no proposals takes three defaults",
			proposals: nil,
			want:      []string{"manger un insecte", "vivre sans internet", "démissionner"},
		},
		{
			name:      "duplicates are merged and defaults top up",
			proposals: []string{"Sauter en parachute", "sauter en parachute ", "manger un insecte"},
			want:      []string{"Sauter en parachute", "manger un insecte", "vivre sans internet"},
		},
		{
			name:      "every distinct proposal is kept",
			proposals: []string{"a", "b", "c", "d"},
			want:      []string{"a", "b", "c", "d"},
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := mergeSecretRankingCandidates(testCase.proposals, defaults); !slices.Equal(got, testCase.want) {
				t.Fatalf("mergeSecretRankingCandidates = %v, want %v", got, testCase.want)
			}
		})
	}
}

func TestNormalizeSecretRankingTheme(t *testing.T) {
	for input, want := range map[string]string{
		"  manger un insecte vivant ?  ":                          "manger un insecte vivant",
		"Pour combien d'argent accepterais-tu de chanter ?":       "chanter",
		"pour combien d’argent accepterais-tu d’aller à Lille ??": "aller à Lille",
		"démissionner": "démissionner",
		"":             "",
	} {
		if got := normalizeSecretRankingTheme(input); got != want {
			t.Errorf("normalizeSecretRankingTheme(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestDefaultSecretRankingThemesFitTheLimit(t *testing.T) {
	themes := defaultSecretRankingThemes()
	if len(themes) < 3 {
		t.Fatalf("need at least three default themes, got %d", len(themes))
	}
	for _, theme := range themes {
		if len([]rune(theme)) > secretRankingThemeMaxLength {
			t.Errorf("default theme %q is longer than %d characters", theme, secretRankingThemeMaxLength)
		}
		if normalizeSecretRankingTheme(theme) != theme {
			t.Errorf("default theme %q is not normalized", theme)
		}
	}
}
