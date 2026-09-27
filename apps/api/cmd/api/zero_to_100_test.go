package main

import (
	"encoding/json"
	"maps"
	"reflect"
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

func TestGuessOrderMatches(t *testing.T) {
	reference := map[string]int{"a": 10, "b": 50, "c": 90}

	testCases := []struct {
		name      string
		reference map[string]int
		guesses   map[string]int
		want      bool
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
			name:    "tied guesses where the reference is strict",
			guesses: map[string]int{"a": 5, "b": 40, "c": 40},
			want:    false,
		},
		{
			name:    "guess for a nominee outside the reference is ignored",
			guesses: map[string]int{"a": 5, "b": 40, "c": 99, "d": 1},
			want:    true,
		},
		{
			name:      "reference tie accepts one guessed order",
			reference: map[string]int{"a": 10, "b": 50, "c": 50},
			guesses:   map[string]int{"a": 5, "b": 40, "c": 60},
			want:      true,
		},
		{
			name:      "reference tie accepts the other guessed order",
			reference: map[string]int{"a": 10, "b": 50, "c": 50},
			guesses:   map[string]int{"a": 5, "b": 60, "c": 40},
			want:      true,
		},
		{
			name:      "a single reference has no order to guess",
			reference: map[string]int{"a": 10},
			guesses:   map[string]int{"a": 10, "b": 40},
			want:      false,
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			caseReference := reference
			if testCase.reference != nil {
				caseReference = testCase.reference
			}
			if got := guessOrderMatches(testCase.guesses, caseReference); got != testCase.want {
				t.Fatalf("guessOrderMatches() = %v, want %v", got, testCase.want)
			}
		})
	}
}

func TestZeroToHundredTargets(t *testing.T) {
	testCases := []struct {
		name     string
		truth    map[string]int
		averages map[string]float64
		want     map[string]int
	}{
		{
			name:     "one third self-placement, two thirds group average",
			truth:    map[string]int{"a": 10},
			averages: map[string]float64{"a": 40},
			want:     map[string]int{"a": 30},
		},
		{
			name:     "rounds to the displayed integer",
			truth:    map[string]int{"a": 52},
			averages: map[string]float64{"a": 45.5},
			want:     map[string]int{"a": 48},
		},
		{
			name:     "a nominee nobody else placed keeps their self-placement",
			truth:    map[string]int{"a": 70},
			averages: map[string]float64{},
			want:     map[string]int{"a": 70},
		},
		{
			name:     "a nominee who never placed themselves has no target",
			truth:    map[string]int{},
			averages: map[string]float64{"a": 40},
			want:     map[string]int{},
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

func TestZeroToHundredGuesserPoints(t *testing.T) {
	targets := map[string]int{"a": 10, "b": 50, "c": 90}

	testCases := []struct {
		name          string
		guesser       string
		targets       map[string]int
		positions     map[string]int
		wantProximity map[string]int
		wantBonus     int
	}{
		{
			name:          "right order earns proximity and the bonus",
			guesser:       "g",
			positions:     map[string]int{"a": 20, "b": 50, "c": 70},
			wantProximity: map[string]int{"a": 9, "b": 10, "c": 8},
			wantBonus:     zeroToHundredOrderBonus,
		},
		{
			name:          "wrong order keeps proximity but loses the bonus",
			guesser:       "g",
			positions:     map[string]int{"a": 60, "b": 50, "c": 90},
			wantProximity: map[string]int{"a": 5, "b": 10, "c": 10},
		},
		{
			name:          "a nominee earns nothing on themselves but their self-placement counts toward the order",
			guesser:       "a",
			positions:     map[string]int{"a": 10, "b": 40, "c": 99},
			wantProximity: map[string]int{"b": 9, "c": 9},
			wantBonus:     zeroToHundredOrderBonus,
		},
		{
			name:          "a nominee without a target earns nothing and is ignored for the order",
			guesser:       "g",
			targets:       map[string]int{"a": 10, "b": 50},
			positions:     map[string]int{"a": 20, "b": 60, "d": 5},
			wantProximity: map[string]int{"a": 9, "b": 9},
			wantBonus:     zeroToHundredOrderBonus,
		},
		{
			name:          "a single target leaves no order bonus",
			guesser:       "g",
			targets:       map[string]int{"a": 10},
			positions:     map[string]int{"a": 10, "b": 40},
			wantProximity: map[string]int{"a": 10},
		},
		{
			name:          "a player who did not answer earns nothing",
			guesser:       "g",
			wantProximity: map[string]int{},
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			caseTargets := targets
			if testCase.targets != nil {
				caseTargets = testCase.targets
			}
			proximity, orderBonus := zeroToHundredGuesserPoints(testCase.guesser, testCase.positions, caseTargets)
			if !maps.Equal(proximity, testCase.wantProximity) {
				t.Fatalf("zeroToHundredGuesserPoints() proximity = %v, want %v", proximity, testCase.wantProximity)
			}
			if orderBonus != testCase.wantBonus {
				t.Fatalf("zeroToHundredGuesserPoints() orderBonus = %d, want %d", orderBonus, testCase.wantBonus)
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
	// Alice sees herself at 10 but the group puts her at 85, so her target (60)
	// ends up above Bob's (57): the targets' order differs from the
	// self-placements'. "c" never answered, so they have no target; "g" is not a
	// nominee.
	guesses := []zeroToHundredGuessRow{
		{Guesser: "a", Nominee: "a", Position: 10},
		{Guesser: "a", Nominee: "b", Position: 40},
		{Guesser: "a", Nominee: "c", Position: 70},
		{Guesser: "b", Nominee: "a", Position: 90},
		{Guesser: "b", Nominee: "b", Position: 80},
		{Guesser: "b", Nominee: "c", Position: 80},
		{Guesser: "g", Nominee: "a", Position: 80},
		{Guesser: "g", Nominee: "b", Position: 50},
		{Guesser: "g", Nominee: "c", Position: 90},
	}

	t.Run("scores against the targets", func(t *testing.T) {
		reveal, orderBonus := buildZeroToHundredReveal(nominees, guesses, "g")
		want := []zeroToHundredRevealEntryView{
			{
				PlayerID: "a", DisplayName: "Alice",
				TruePosition: pointerTo(10), AveragePosition: pointerTo(85.0), TargetPosition: pointerTo(60),
				MyPosition: pointerTo(80), Points: 8,
			},
			{
				PlayerID: "b", DisplayName: "Bob",
				TruePosition: pointerTo(80), AveragePosition: pointerTo(45.0), TargetPosition: pointerTo(57),
				MyPosition: pointerTo(50), Points: 9,
			},
			{
				PlayerID: "c", DisplayName: "Chloé",
				AveragePosition: pointerTo(80.0), MyPosition: pointerTo(90),
			},
		}
		if !reflect.DeepEqual(reveal, want) {
			got, _ := json.Marshal(reveal)
			expected, _ := json.Marshal(want)
			t.Fatalf("buildZeroToHundredReveal() reveal = %s, want %s", got, expected)
		}
		if orderBonus != zeroToHundredOrderBonus {
			t.Fatalf("buildZeroToHundredReveal() orderBonus = %d, want %d (Bob below Alice, like the targets)", orderBonus, zeroToHundredOrderBonus)
		}
	})

	t.Run("a nominee earns nothing on themselves", func(t *testing.T) {
		reveal, orderBonus := buildZeroToHundredReveal(nominees, guesses, "a")
		if !reveal[0].IsCurrentPlayer || reveal[0].Points != 0 || reveal[1].Points != 8 {
			got, _ := json.Marshal(reveal)
			t.Fatalf("buildZeroToHundredReveal() reveal = %s, want no points on Alice and 8 on Bob", got)
		}
		// Alice placed herself (10) below Bob (40), unlike the targets.
		if orderBonus != 0 {
			t.Fatalf("buildZeroToHundredReveal() orderBonus = %d, want 0", orderBonus)
		}
	})

	t.Run("player who did not answer", func(t *testing.T) {
		reveal, orderBonus := buildZeroToHundredReveal(nominees, guesses, "x")
		for _, entry := range reveal {
			if entry.MyPosition != nil || entry.Points != 0 || entry.IsCurrentPlayer {
				got, _ := json.Marshal(entry)
				t.Fatalf("buildZeroToHundredReveal() entry for a silent player = %s", got)
			}
		}
		if orderBonus != 0 {
			t.Fatalf("buildZeroToHundredReveal() orderBonus = %d, want 0", orderBonus)
		}
	})
}

func pointerTo[T any](value T) *T {
	return &value
}
