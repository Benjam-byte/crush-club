package main

import (
	"encoding/json"
	"maps"
	"reflect"
	"slices"
	"testing"
)

func TestSelectZeroToHundredNomineesReturnsDistinctPlayers(t *testing.T) {
	testCases := []struct {
		name      string
		playerIDs []string
		want      int
	}{
		{name: "caps at five", playerIDs: []string{"p1", "p2", "p3", "p4", "p5", "p6"}, want: zeroToHundredNomineeCount},
		{name: "everyone when fewer than five", playerIDs: []string{"p1", "p2", "p3"}, want: 3},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			for attempt := 0; attempt < 20; attempt++ {
				nominees, err := selectZeroToHundredNominees(testCase.playerIDs)
				if err != nil {
					t.Fatalf("selectZeroToHundredNominees() error = %v", err)
				}
				if len(nominees) != testCase.want {
					t.Fatalf("selectZeroToHundredNominees() returned %d nominees, want %d", len(nominees), testCase.want)
				}
				seen := make(map[string]bool, len(nominees))
				for _, id := range nominees {
					if seen[id] {
						t.Fatalf("selectZeroToHundredNominees() returned a duplicate nominee: %v", nominees)
					}
					seen[id] = true
				}
			}
		})
	}
}

func TestSelectZeroToHundredNomineesRejectsFewerThanMinimumPlayers(t *testing.T) {
	if _, err := selectZeroToHundredNominees([]string{"p1", "p2"}); err == nil {
		t.Fatal("selectZeroToHundredNominees() expected an error for two players, got nil")
	}
}

func TestZeroToHundredRanks(t *testing.T) {
	got := zeroToHundredRanks(map[string]float64{"a": 8.1, "b": 5.3, "c": 5.3, "d": 2})
	want := map[string]int{"a": 1, "b": 2, "c": 2, "d": 4}
	if !maps.Equal(got, want) {
		t.Fatalf("zeroToHundredRanks() = %v, want %v", got, want)
	}
}

func TestZeroToHundredTargets(t *testing.T) {
	testCases := []struct {
		name     string
		truth    map[string]int
		averages map[string]float64
		want     map[string]float64
	}{
		{
			name:     "one third self-placement, two thirds group average",
			truth:    map[string]int{"a": 1},
			averages: map[string]float64{"a": 4},
			want:     map[string]float64{"a": 3},
		},
		{
			name:     "rounds to the displayed decimal",
			truth:    map[string]int{"a": 5},
			averages: map[string]float64{"a": 4.5},
			want:     map[string]float64{"a": 4.7},
		},
		{
			name:     "a nominee nobody else placed keeps their self-placement",
			truth:    map[string]int{"a": 7},
			averages: map[string]float64{},
			want:     map[string]float64{"a": 7},
		},
		{
			name:     "a nominee who never placed themselves has no target",
			truth:    map[string]int{},
			averages: map[string]float64{"a": 4},
			want:     map[string]float64{},
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := zeroToHundredTargets(testCase.truth, testCase.averages); !maps.Equal(got, testCase.want) {
				t.Fatalf("zeroToHundredTargets() = %v, want %v", got, testCase.want)
			}
		})
	}
}

func TestScoreZeroToHundredGuesser(t *testing.T) {
	// The podium: c first, b second, a third.
	targets := map[string]float64{"a": 1, "b": 5, "c": 9}

	testCases := []struct {
		name        string
		targets     map[string]float64
		positions   map[string]int
		wantCorrect []string
		wantPerfect bool
	}{
		{
			name:        "same order as the podium",
			positions:   map[string]int{"a": 0, "b": 4, "c": 10},
			wantCorrect: []string{"a", "b", "c"},
			wantPerfect: true,
		},
		{
			name:        "only the order counts, not the values",
			positions:   map[string]int{"a": 1, "b": 2, "c": 3},
			wantCorrect: []string{"a", "b", "c"},
			wantPerfect: true,
		},
		{
			name:        "reversed order keeps the middle step only",
			positions:   map[string]int{"a": 9, "b": 5, "c": 1},
			wantCorrect: []string{"b"},
		},
		{
			name:        "everyone shifted by one step",
			positions:   map[string]int{"a": 9, "b": 1, "c": 5},
			wantCorrect: []string{},
		},
		{
			name:        "a tie the guesser made only gets the better rank",
			positions:   map[string]int{"a": 1, "b": 4, "c": 4},
			wantCorrect: []string{"a", "c"},
		},
		{
			name:        "a nominee the guesser did not place shifts the others",
			positions:   map[string]int{"a": 1, "c": 9},
			wantCorrect: []string{"c"},
		},
		{
			name:        "a podium tie accepts one order",
			targets:     map[string]float64{"a": 1, "b": 5, "c": 5},
			positions:   map[string]int{"a": 0, "b": 4, "c": 6},
			wantCorrect: []string{"a", "b", "c"},
			wantPerfect: true,
		},
		{
			name:        "a podium tie accepts the other order",
			targets:     map[string]float64{"a": 1, "b": 5, "c": 5},
			positions:   map[string]int{"a": 0, "b": 6, "c": 4},
			wantCorrect: []string{"a", "b", "c"},
			wantPerfect: true,
		},
		{
			name:        "a nominee without a target is left out of the ranking",
			targets:     map[string]float64{"a": 1, "b": 5},
			positions:   map[string]int{"a": 2, "b": 6, "d": 10},
			wantCorrect: []string{"a", "b"},
			wantPerfect: true,
		},
		{
			name:        "a single target leaves no order to find",
			targets:     map[string]float64{"a": 1},
			positions:   map[string]int{"a": 1, "b": 4},
			wantCorrect: []string{"a"},
		},
		{
			name:        "a player who did not answer has nothing right",
			wantCorrect: []string{},
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			caseTargets := targets
			if testCase.targets != nil {
				caseTargets = testCase.targets
			}
			score := scoreZeroToHundredGuesser(testCase.positions, caseTargets)
			correct := make([]string, 0, len(score.Correct))
			for nominee, isCorrect := range score.Correct {
				if isCorrect {
					correct = append(correct, nominee)
				}
			}
			slices.Sort(correct)
			if !slices.Equal(correct, testCase.wantCorrect) || score.CorrectCount != len(testCase.wantCorrect) {
				t.Fatalf("scoreZeroToHundredGuesser() correct = %v (count %d), want %v", correct, score.CorrectCount, testCase.wantCorrect)
			}
			if score.PerfectOrder != testCase.wantPerfect {
				t.Fatalf("scoreZeroToHundredGuesser() perfect = %v, want %v", score.PerfectOrder, testCase.wantPerfect)
			}
		})
	}
}

func TestBuildZeroToHundredReveal(t *testing.T) {
	nominees := []zeroToHundredNomineeRow{
		{PlayerID: "a", DisplayName: "Alice"},
		{PlayerID: "b", DisplayName: "Bob"},
		{PlayerID: "c", DisplayName: "Chloé"},
	}
	// Alice gives herself 1 but the group gives her 8.5, so her target (6) ends
	// up above Bob's (5.7): the podium differs from the self-placements. "c"
	// never answered, so they have no target; "g" is not a nominee.
	guesses := []zeroToHundredGuessRow{
		{Guesser: "a", GuesserName: "Alice", Nominee: "a", Position: 1},
		{Guesser: "a", GuesserName: "Alice", Nominee: "b", Position: 4},
		{Guesser: "a", GuesserName: "Alice", Nominee: "c", Position: 7},
		{Guesser: "b", GuesserName: "Bob", Nominee: "a", Position: 9},
		{Guesser: "b", GuesserName: "Bob", Nominee: "b", Position: 8},
		{Guesser: "b", GuesserName: "Bob", Nominee: "c", Position: 8},
		{Guesser: "g", GuesserName: "Gus", Nominee: "a", Position: 8},
		{Guesser: "g", GuesserName: "Gus", Nominee: "b", Position: 5},
		{Guesser: "g", GuesserName: "Gus", Nominee: "c", Position: 9},
	}

	reveal, guessers := buildZeroToHundredReveal(nominees, guesses, "g")

	wantReveal := []zeroToHundredRevealEntryView{
		{
			PlayerID: "a", DisplayName: "Alice",
			TruePosition: pointerTo(1), AveragePosition: pointerTo(8.5), TargetPosition: pointerTo(6.0), TargetRank: pointerTo(1),
		},
		{
			PlayerID: "b", DisplayName: "Bob",
			TruePosition: pointerTo(8), AveragePosition: pointerTo(4.5), TargetPosition: pointerTo(5.7), TargetRank: pointerTo(2),
		},
		{PlayerID: "c", DisplayName: "Chloé", AveragePosition: pointerTo(8.0)},
	}
	if !reflect.DeepEqual(reveal, wantReveal) {
		got, _ := json.Marshal(reveal)
		expected, _ := json.Marshal(wantReveal)
		t.Fatalf("buildZeroToHundredReveal() reveal = %s, want %s", got, expected)
	}

	// Gus and Bob both put Alice above Bob, like the podium; Alice put herself
	// below Bob. Chloé, who has no target, is left out of every ranking.
	wantGuessers := []zeroToHundredGuesserView{
		{
			PlayerID: "b", DisplayName: "Bob",
			Placements: []zeroToHundredPlacementView{
				{NomineeID: "a", Position: 9, Rank: 1, Correct: true},
				{NomineeID: "b", Position: 8, Rank: 2, Correct: true},
			},
			CorrectCount: 2, PerfectOrder: true,
		},
		{
			PlayerID: "g", DisplayName: "Gus", IsCurrentPlayer: true,
			Placements: []zeroToHundredPlacementView{
				{NomineeID: "a", Position: 8, Rank: 1, Correct: true},
				{NomineeID: "b", Position: 5, Rank: 2, Correct: true},
			},
			CorrectCount: 2, PerfectOrder: true,
		},
		{
			PlayerID: "a", DisplayName: "Alice",
			Placements: []zeroToHundredPlacementView{
				{NomineeID: "b", Position: 4, Rank: 1},
				{NomineeID: "a", Position: 1, Rank: 2},
			},
		},
	}
	if !reflect.DeepEqual(guessers, wantGuessers) {
		got, _ := json.Marshal(guessers)
		expected, _ := json.Marshal(wantGuessers)
		t.Fatalf("buildZeroToHundredReveal() guessers = %s, want %s", got, expected)
	}

	t.Run("flags the current player", func(t *testing.T) {
		reveal, guessers := buildZeroToHundredReveal(nominees, guesses, "a")
		if !reveal[0].IsCurrentPlayer || reveal[1].IsCurrentPlayer || !guessers[2].IsCurrentPlayer || guessers[1].IsCurrentPlayer {
			t.Fatalf("buildZeroToHundredReveal() did not flag only Alice as the current player")
		}
	})
}

func TestBuildZeroToHundredLeaderboard(t *testing.T) {
	players := []zeroToHundredNomineeRow{
		{PlayerID: "a", DisplayName: "Alice"},
		{PlayerID: "b", DisplayName: "Bob"},
		{PlayerID: "c", DisplayName: "Chloé"},
		{PlayerID: "d", DisplayName: "Dan"},
	}
	guessesByRound := map[string][]zeroToHundredGuessRow{
		// Targets: a 5.3, b 4.7. Alice and Bob find the order, Chloé reverses it.
		"round-1": {
			{Guesser: "a", Nominee: "a", Position: 8},
			{Guesser: "a", Nominee: "b", Position: 2},
			{Guesser: "b", Nominee: "a", Position: 7},
			{Guesser: "b", Nominee: "b", Position: 3},
			{Guesser: "c", Nominee: "a", Position: 1},
			{Guesser: "c", Nominee: "b", Position: 9},
		},
		// Targets: a 3.3, c 6. Alice and Chloé find the order; Bob's tie only
		// gets Chloé's first place right.
		"round-2": {
			{Guesser: "a", Nominee: "a", Position: 2},
			{Guesser: "a", Nominee: "c", Position: 6},
			{Guesser: "b", Nominee: "a", Position: 5},
			{Guesser: "b", Nominee: "c", Position: 5},
			{Guesser: "c", Nominee: "a", Position: 3},
			{Guesser: "c", Nominee: "c", Position: 7},
		},
		// "x" left the lobby: their answers are not listed.
		"round-3": {
			{Guesser: "x", Nominee: "x", Position: 5},
		},
	}
	got := buildZeroToHundredLeaderboard(players, guessesByRound)
	want := []zeroToHundredLeaderboardEntryView{
		{PlayerID: "a", DisplayName: "Alice", Score: 4, PerfectOrders: 2},
		{PlayerID: "b", DisplayName: "Bob", Score: 3, PerfectOrders: 1},
		{PlayerID: "c", DisplayName: "Chloé", Score: 2, PerfectOrders: 1},
		{PlayerID: "d", DisplayName: "Dan"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("buildZeroToHundredLeaderboard() = %+v, want %+v", got, want)
	}
}

func pointerTo[T any](value T) *T {
	return &value
}
