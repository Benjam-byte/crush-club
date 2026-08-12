package main

import "testing"

func TestSelectZeroToHundredNomineesReturnsThreeDistinctPlayers(t *testing.T) {
	playerIDs := []string{"p1", "p2", "p3", "p4", "p5", "p6"}
	for attempt := 0; attempt < 20; attempt++ {
		nominees, err := selectZeroToHundredNominees(playerIDs)
		if err != nil {
			t.Fatalf("selectZeroToHundredNominees() error = %v", err)
		}
		if len(nominees) != zeroToHundredNomineeCount {
			t.Fatalf("selectZeroToHundredNominees() returned %d nominees, want %d", len(nominees), zeroToHundredNomineeCount)
		}
		seen := make(map[string]bool, len(nominees))
		for _, id := range nominees {
			if seen[id] {
				t.Fatalf("selectZeroToHundredNominees() returned a duplicate nominee: %v", nominees)
			}
			seen[id] = true
		}
	}
}

func TestSelectZeroToHundredNomineesRejectsFewerThanThreePlayers(t *testing.T) {
	if _, err := selectZeroToHundredNominees([]string{"p1", "p2"}); err == nil {
		t.Fatal("selectZeroToHundredNominees() expected an error for two players, got nil")
	}
}

func TestSortNomineesByPositionOrdersAscendingWithStableTieBreak(t *testing.T) {
	positions := map[string]int{"b": 50, "a": 50, "c": 10}
	got := sortNomineesByPosition(positions)
	want := []string{"c", "a", "b"} // c=10 first, then a/b tie at 50 broken alphabetically
	if len(got) != len(want) {
		t.Fatalf("sortNomineesByPosition() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("sortNomineesByPosition() = %v, want %v", got, want)
		}
	}
}

func TestGuessOrderMatchesTruth(t *testing.T) {
	truth := map[string]int{"a": 10, "b": 50, "c": 90}
	trueOrder := sortNomineesByPosition(truth)

	testCases := []struct {
		name    string
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
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := guessOrderMatchesTruth(testCase.guesses, truth, trueOrder); got != testCase.want {
				t.Fatalf("guessOrderMatchesTruth() = %v, want %v", got, testCase.want)
			}
		})
	}
}
