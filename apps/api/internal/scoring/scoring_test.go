package scoring

import "testing"

func TestFinalScore(t *testing.T) {
	tests := []struct {
		name string
		in   AnswerScore
		want int
	}{
		{"normal", AnswerScore{BaseScore: 7, MaximumScore: 10}, 7},
		{"lover exact", AnswerScore{BaseScore: 10, MaximumScore: 10, Exact: true, LoverSelected: true}, 20},
		{"lover close", AnswerScore{BaseScore: 9, MaximumScore: 10, LoverSelected: true}, -1},
		{"lover wrong", AnswerScore{BaseScore: 0, MaximumScore: 10, LoverSelected: true}, -10},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := FinalScore(tt.in); got != tt.want {
				t.Fatalf("FinalScore() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestIntegerRangeScore(t *testing.T) {
	tests := []struct {
		name      string
		official  int
		predicted int
		want      int
	}{
		{"exact", 7, 7, 10},
		{"close", 7, 6, 9},
		{"minimum boundary", 0, 10, 0},
		{"maximum boundary", 10, 0, 0},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := IntegerRangeScore(test.official, test.predicted, 0, 10, 10); got != test.want {
				t.Fatalf("IntegerRangeScore() = %d, want %d", got, test.want)
			}
		})
	}
}

func TestSingleChoiceScore(t *testing.T) {
	if got := SingleChoiceScore("quality-time", "quality-time", 10); got != 10 {
		t.Fatalf("SingleChoiceScore() exact = %d, want 10", got)
	}
	if got := SingleChoiceScore("quality-time", "gifts", 10); got != 0 {
		t.Fatalf("SingleChoiceScore() wrong = %d, want 0", got)
	}
}

func TestNumberScore(t *testing.T) {
	tests := []struct {
		name      string
		predicted float64
		want      int
	}{
		{"within ten percent", 108, 20},
		{"within twenty five percent", 120, 15},
		{"within fifty percent", 145, 10},
		{"within seventy five percent", 170, 5},
		{"outside tolerance", 180, 0},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := NumberScore(100, test.predicted, 1, 20); got != test.want {
				t.Fatalf("NumberScore() = %d, want %d", got, test.want)
			}
		})
	}
}

func TestMultiChoiceScore(t *testing.T) {
	if got := MultiChoiceScore([]string{"friends", "travel"}, []string{"friends", "travel"}, 10); got != 10 {
		t.Fatalf("MultiChoiceScore() exact = %d, want 10", got)
	}
	if got := MultiChoiceScore([]string{"friends", "travel"}, []string{"friends", "travel", "sport"}, 10); got != 7 {
		t.Fatalf("MultiChoiceScore() with extra option = %d, want 7", got)
	}
}

func TestRankedChoiceScore(t *testing.T) {
	if got := RankedChoiceScore([]string{"humor", "trust", "adventure"}, []string{"humor", "trust", "adventure"}); got != 15 {
		t.Fatalf("RankedChoiceScore() exact = %d, want 15", got)
	}
	if got := RankedChoiceScore([]string{"humor", "trust", "adventure"}, []string{"adventure", "trust", "humor"}); got != 8 {
		t.Fatalf("RankedChoiceScore() reversed = %d, want 8", got)
	}
}

func TestFinalScoreIsIdempotent(t *testing.T) {
	answerScore := AnswerScore{
		BaseScore:     10,
		MaximumScore:  10,
		Exact:         true,
		LoverSelected: true,
	}
	firstScore := FinalScore(answerScore)
	secondScore := FinalScore(answerScore)
	if firstScore != secondScore {
		t.Fatalf("FinalScore() changed between calls: %d then %d", firstScore, secondScore)
	}
}
