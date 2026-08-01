package main

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
)

func multiplayerTestSnapshot() questionnaireSnapshot {
	minimum := 0
	maximum := 10
	return questionnaireSnapshot{
		ProfileFields: []profileField{{
			ID: "quality", Label: "Qualité",
			Options: []questionOption{{ID: "funny"}, {ID: "kind"}},
		}},
		Questions: []question{
			{
				ID: "choice", Type: "single_choice", MaximumScore: 10, LoverEligible: true,
				Options: []questionOption{{ID: "yes"}, {ID: "no"}},
			},
			{
				ID: "range", Type: "integer_range", MaximumScore: 10, LoverEligible: true,
				Minimum: &minimum, Maximum: &maximum,
			},
		},
	}
}

func TestValidateRoundSubmissionSeparatesSubjectAndCupid(t *testing.T) {
	snapshot := multiplayerTestSnapshot()
	valid := roundSubmissionInput{
		BioAnswers: map[string]string{"quality": "funny"},
		QuestionAnswers: map[string]json.RawMessage{
			"choice": json.RawMessage(`"yes"`),
			"range":  json.RawMessage(`7`),
		},
	}
	if err := validateRoundSubmission(snapshot, valid, "official"); err != nil {
		t.Fatalf("valid official submission rejected: %v", err)
	}

	prediction := valid
	prediction.Tagline = "Toujours partant pour rire"
	prediction.LoverQuestionID = "choice"
	if err := validateRoundSubmission(snapshot, prediction, "prediction"); err != nil {
		t.Fatalf("valid prediction rejected: %v", err)
	}

	officialWithHiddenFields := prediction
	if err := validateRoundSubmission(snapshot, officialWithHiddenFields, "official"); err == nil {
		t.Fatal("subject submission with a tagline and LOVER should be rejected")
	}

	invalid := prediction
	invalid.QuestionAnswers = map[string]json.RawMessage{
		"choice": json.RawMessage(`"unknown"`),
		"range":  json.RawMessage(`11`),
	}
	if err := validateRoundSubmission(snapshot, invalid, "prediction"); err == nil {
		t.Fatal("out-of-catalog answers should be rejected")
	}
}

func TestScorePredictionAppliesLoverAndTaglineBonusDeterministically(t *testing.T) {
	snapshot := multiplayerTestSnapshot()
	official := storedSubmission{
		BioAnswers: map[string]string{"quality": "funny"},
		QuestionAnswers: map[string]json.RawMessage{
			"choice": json.RawMessage(`"yes"`),
			"range":  json.RawMessage(`10`),
		},
	}
	prediction := storedSubmission{
		BioAnswers: map[string]string{"quality": "funny"},
		QuestionAnswers: map[string]json.RawMessage{
			"choice": json.RawMessage(`"yes"`),
			"range":  json.RawMessage(`8`),
		},
		LoverQuestionID: "choice",
	}

	base, lover, tagline, total, exact, lines, err := scorePrediction(snapshot, official, prediction, true)
	if err != nil {
		t.Fatalf("scorePrediction() error = %v", err)
	}
	if base != 28 || lover != 10 || tagline != 10 || total != 48 || exact != 2 {
		t.Fatalf("score breakdown = (%d, %d, %d, %d, %d), want (28, 10, 10, 48, 2)", base, lover, tagline, total, exact)
	}
	if len(lines) != 3 || !lines[1].IsLoverApplied || lines[1].FinalScore != 20 {
		t.Fatalf("unexpected score lines: %#v", lines)
	}

	prediction.QuestionAnswers["choice"] = json.RawMessage(`"no"`)
	base, lover, tagline, total, _, _, err = scorePrediction(snapshot, official, prediction, false)
	if err != nil {
		t.Fatalf("scorePrediction() error = %v", err)
	}
	if base != 18 || lover != -10 || tagline != 0 || total != 8 {
		t.Fatalf("failed LOVER breakdown = (%d, %d, %d, %d), want (18, -10, 0, 8)", base, lover, tagline, total)
	}
}

func TestDecodeImageDimensionsValidatesSupportedHeaders(t *testing.T) {
	pngData, err := base64.StdEncoding.DecodeString(onePixelPNG)
	if err != nil {
		t.Fatal(err)
	}
	width, height, err := decodeImageDimensions(pngData, "image/png")
	if err != nil || width != 1 || height != 1 {
		t.Fatalf("PNG dimensions = %dx%d, %v", width, height, err)
	}

	if _, _, err := decodeImageDimensions([]byte("not an image"), "image/png"); err == nil {
		t.Fatal("invalid PNG header should be rejected")
	}
}

func TestRawAnswerMapsEqualIgnoresWhitespaceButNotValues(t *testing.T) {
	first := map[string]json.RawMessage{"answer": json.RawMessage(` 7 `)}
	second := map[string]json.RawMessage{"answer": json.RawMessage(`7`)}
	if !rawAnswerMapsEqual(first, second) {
		t.Fatal("equivalent raw answers should compare equal")
	}
	second["answer"] = json.RawMessage(`8`)
	if rawAnswerMapsEqual(first, second) {
		t.Fatal("different raw answers should not compare equal")
	}
}

func TestPredictionTaglineLimitUsesRunes(t *testing.T) {
	input := roundSubmissionInput{
		Tagline:    strings.Repeat("é", 101),
		BioAnswers: map[string]string{"quality": "funny"},
		QuestionAnswers: map[string]json.RawMessage{
			"choice": json.RawMessage(`"yes"`),
			"range":  json.RawMessage(`7`),
		},
		LoverQuestionID: "choice",
	}
	if err := validateRoundSubmission(multiplayerTestSnapshot(), input, "prediction"); err == nil {
		t.Fatal("a 101-character tagline should be rejected")
	}
}
