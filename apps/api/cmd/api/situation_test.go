package main

import (
	"reflect"
	"testing"
)

func TestSituationDuelsForWaveConvergesToExactlyFourFinalists(t *testing.T) {
	// Simulates repeatedly applying a wave (aliveCount -= duelsThisWave, since
	// each duel removes exactly one loser) until at most situationFinalistCount
	// remain, for a range of starting player counts including ones that are
	// not neatly divisible by 4 or by 2.
	for startCount := situationFinalistCount + 1; startCount <= 37; startCount++ {
		aliveCount := startCount
		waves := 0
		for aliveCount > situationFinalistCount {
			duels := situationDuelsForWave(aliveCount)
			if duels <= 0 {
				t.Fatalf("startCount=%d: situationDuelsForWave(%d) = %d, want > 0 while above the finalist count", startCount, aliveCount, duels)
			}
			aliveCount -= duels
			waves++
			if waves > 10 {
				t.Fatalf("startCount=%d: did not converge after 10 waves, aliveCount=%d", startCount, aliveCount)
			}
		}
		if aliveCount != situationFinalistCount {
			t.Fatalf("startCount=%d: converged to %d survivors, want exactly %d", startCount, aliveCount, situationFinalistCount)
		}
	}
}

func TestSituationDuelsForWaveNeverExceedsHalf(t *testing.T) {
	for aliveCount := 0; aliveCount <= 20; aliveCount++ {
		duels := situationDuelsForWave(aliveCount)
		if duels > aliveCount/2 {
			t.Fatalf("situationDuelsForWave(%d) = %d, exceeds the %d pairs actually available", aliveCount, duels, aliveCount/2)
		}
	}
}

func TestSituationDuelsForWaveSpecificCases(t *testing.T) {
	testCases := []struct {
		aliveCount int
		want       int
	}{
		{aliveCount: 3, want: 0},  // at or below the finalist count already
		{aliveCount: 4, want: 0},
		{aliveCount: 5, want: 1},  // 5 -> 4: one duel, three byes
		{aliveCount: 6, want: 2},  // 6 -> 4: three pairs possible, only need 2
		{aliveCount: 8, want: 4},  // 8 -> 4: exactly one full pairing wave
		{aliveCount: 9, want: 4},  // limited by k/2 = 4, not k-4 = 5; one group byes
		{aliveCount: 10, want: 5}, // 10 -> 5 this wave (not down to 4 yet), needs a second wave
	}
	for _, testCase := range testCases {
		if got := situationDuelsForWave(testCase.aliveCount); got != testCase.want {
			t.Errorf("situationDuelsForWave(%d) = %d, want %d", testCase.aliveCount, got, testCase.want)
		}
	}
}

func TestTallyProposalPointsWeightsByRankPosition(t *testing.T) {
	candidates := []string{"p1", "p2", "p3", "p4"}
	rankings := [][]string{
		{"p1", "p2", "p3", "p4"},
		{"p1", "p3", "p2", "p4"},
	}
	// Weights (4 candidates): 1st=4, 2nd=3, 3rd=2, 4th=1.
	// p1: 4+4=8, p2: 3+2=5, p3: 2+3=5, p4: 1+1=2.
	want := map[string]int{"p1": 8, "p2": 5, "p3": 5, "p4": 2}
	got := tallyProposalPoints(candidates, rankings)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("tallyProposalPoints() = %v, want %v", got, want)
	}
}

func TestTallyProposalPointsIgnoresUnknownEntriesAndMissingVoters(t *testing.T) {
	candidates := []string{"p1", "p2"}
	rankings := [][]string{
		{"p1", "unknown-proposal", "p2"},
	}
	want := map[string]int{"p1": 2, "p2": 0}
	got := tallyProposalPoints(candidates, rankings)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("tallyProposalPoints() = %v, want %v", got, want)
	}
}
