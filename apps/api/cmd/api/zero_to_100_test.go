package main

import "testing"

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

func TestGuessOrderMatchesTruth(t *testing.T) {
	truth := map[string]int{"a": 10, "b": 50, "c": 90}

	testCases := []struct {
		name    string
		truth   map[string]int
		guesses map[string]int
		want    bool
	}{
		{
			name:    "exact relative order",
			guesses: map[string]int{"a": 5, "b": 40, "c": 99},
			want:    true,
		},
		{
			name:    "different absolute values but same order",
			guesses: map[string]int{"a": 1, "b": 2, "c": 3},
			want:    true,
		},
		{
			name:    "wrong order",
			guesses: map[string]int{"a": 90, "b": 50, "c": 10},
			want:    false,
		},
		{
			name:    "missing a nominee",
			guesses: map[string]int{"a": 5, "b": 40},
			want:    false,
		},
		{
			name:    "tied guesses where the truth is strict",
			guesses: map[string]int{"a": 5, "b": 40, "c": 40},
			want:    false,
		},
		{
			name:    "nominee's own self-placement counts toward the order",
			guesses: map[string]int{"a": 10, "b": 30, "c": 70}, // guesser "a" is a nominee: their own position equals the truth
			want:    true,
		},
		{
			name:    "guess for a nominee who never placed themselves is ignored",
			guesses: map[string]int{"a": 5, "b": 40, "c": 99, "d": 1},
			want:    true,
		},
		{
			name:    "truth tie accepts one guessed order",
			truth:   map[string]int{"a": 10, "b": 50, "c": 50},
			guesses: map[string]int{"a": 5, "b": 40, "c": 60},
			want:    true,
		},
		{
			name:    "truth tie accepts the other guessed order",
			truth:   map[string]int{"a": 10, "b": 50, "c": 50},
			guesses: map[string]int{"a": 5, "b": 60, "c": 40},
			want:    true,
		},
		{
			name:    "a single self-placement has no order to guess",
			truth:   map[string]int{"a": 10},
			guesses: map[string]int{"a": 10, "b": 40},
			want:    false,
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			caseTruth := truth
			if testCase.truth != nil {
				caseTruth = testCase.truth
			}
			if got := guessOrderMatchesTruth(testCase.guesses, caseTruth); got != testCase.want {
				t.Fatalf("guessOrderMatchesTruth() = %v, want %v", got, testCase.want)
			}
		})
	}
}
