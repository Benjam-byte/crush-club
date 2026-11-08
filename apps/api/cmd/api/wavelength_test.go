package main

import "testing"

func TestWavelengthPoints(t *testing.T) {
	testCases := []struct {
		name     string
		target   int
		position int
		want     int
	}{
		{name: "exact hit", target: 50, position: 50, want: 4},
		{name: "bullseye edge", target: 50, position: 53, want: 4},
		{name: "inner band", target: 50, position: 46, want: 3},
		{name: "inner band edge", target: 50, position: 59, want: 3},
		{name: "outer band", target: 50, position: 60, want: 2},
		{name: "outer band edge", target: 50, position: 35, want: 2},
		{name: "missed", target: 50, position: 34, want: 0},
		{name: "far missed", target: 3, position: 100, want: 0},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := wavelengthPoints(testCase.target, testCase.position); got != testCase.want {
				t.Fatalf("wavelengthPoints(%d, %d) = %d, want %d", testCase.target, testCase.position, got, testCase.want)
			}
		})
	}
}

func TestWavelengthPsychicPoints(t *testing.T) {
	testCases := []struct {
		name   string
		points []int
		want   int
	}{
		{name: "nobody answered", points: nil, want: 0},
		{name: "single guesser", points: []int{3}, want: 3},
		{name: "rounds half up", points: []int{4, 3}, want: 4},
		{name: "rounds down", points: []int{4, 0, 0}, want: 1},
		{name: "everyone missed", points: []int{0, 0}, want: 0},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := wavelengthPsychicPoints(testCase.points); got != testCase.want {
				t.Fatalf("wavelengthPsychicPoints(%v) = %d, want %d", testCase.points, got, testCase.want)
			}
		})
	}
}

func TestPickWavelengthPsychicSkipsPastPsychics(t *testing.T) {
	active := []string{"p1", "p2", "p3"}
	for attempt := 0; attempt < 20; attempt++ {
		psychic, ok := pickWavelengthPsychic(active, []string{"p1", "p3"})
		if !ok || psychic != "p2" {
			t.Fatalf("pickWavelengthPsychic() = (%q, %v), want (\"p2\", true)", psychic, ok)
		}
	}
}

func TestPickWavelengthPsychicEndsOnceEveryoneHadATurn(t *testing.T) {
	// A player who left after being the psychic must not keep the game going.
	if psychic, ok := pickWavelengthPsychic([]string{"p1", "p2"}, []string{"p1", "p2", "gone"}); ok {
		t.Fatalf("pickWavelengthPsychic() = (%q, true), want no psychic left", psychic)
	}
}

func TestMergeWavelengthCandidatesTopsUpWithUnplayedDefaults(t *testing.T) {
	defaults := defaultWavelengthSpectra()
	proposals := []wavelengthSpectrum{
		{Left: "Nul", Right: "Génial"},
		{Left: "nul", Right: "génial"},
	}
	played := []wavelengthSpectrum{defaults[0]}

	candidates := mergeWavelengthCandidates(proposals, played, 0)

	if len(candidates) != 3 {
		t.Fatalf("mergeWavelengthCandidates() returned %d candidates, want 3: %v", len(candidates), candidates)
	}
	if candidates[0] != proposals[0] {
		t.Fatalf("mergeWavelengthCandidates() first candidate = %v, want the deduplicated proposal", candidates[0])
	}
	for _, candidate := range candidates {
		if candidate == defaults[0] {
			t.Fatalf("mergeWavelengthCandidates() offered an already played spectrum: %v", candidates)
		}
	}
	if candidates[1] != defaults[1] || candidates[2] != defaults[2] {
		t.Fatalf("mergeWavelengthCandidates() defaults = %v, want the next ones from the offset", candidates[1:])
	}
}

func TestMergeWavelengthCandidatesReusesPlayedDefaultsWhenExhausted(t *testing.T) {
	candidates := mergeWavelengthCandidates(nil, defaultWavelengthSpectra(), 5)
	if len(candidates) != 3 {
		t.Fatalf("mergeWavelengthCandidates() returned %d candidates, want 3", len(candidates))
	}
}

func TestWavelengthDefaultsOffsetIsStablePerRound(t *testing.T) {
	first := wavelengthDefaultsOffset("game-a", 2)
	if again := wavelengthDefaultsOffset("game-a", 2); again != first {
		t.Fatalf("wavelengthDefaultsOffset() changed between calls: %d then %d", first, again)
	}
	if first < 0 || first >= len(defaultWavelengthSpectra()) {
		t.Fatalf("wavelengthDefaultsOffset() = %d, out of the defaults range", first)
	}
}

func TestWavelengthSpectrumLabel(t *testing.T) {
	if label := (wavelengthSpectrum{Left: "Froid", Right: "Chaud"}).label(); label != "Froid ↔ Chaud" {
		t.Fatalf("label() = %q, want %q", label, "Froid ↔ Chaud")
	}
}
