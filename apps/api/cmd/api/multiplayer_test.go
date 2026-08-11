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
			primaryPhotoQuestionID: json.RawMessage(`"photo-1"`),
			"choice":               json.RawMessage(`"yes"`),
			"range":                json.RawMessage(`7`),
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

func TestPersonalQuestionnaireRoundHasNoAutomaticBioAnswers(t *testing.T) {
	snapshot := questionnaireSnapshot{
		Kind:          "personal",
		ProfileFields: []profileField{},
		Questions: []question{{
			ID: "custom", Type: "binary_choice", MaximumScore: 10, LoverEligible: true,
			Options: []questionOption{{ID: "yes"}, {ID: "no"}},
		}},
	}
	officialInput := roundSubmissionInput{
		BioAnswers: map[string]string{},
		QuestionAnswers: map[string]json.RawMessage{
			primaryPhotoQuestionID: json.RawMessage(`"photo-1"`),
			"custom":               json.RawMessage(`"yes"`),
		},
	}
	if err := validateRoundSubmission(snapshot, officialInput, "official"); err != nil {
		t.Fatalf("personal official submission rejected: %v", err)
	}

	predictionInput := officialInput
	predictionInput.Tagline = "Partant pour rire"
	predictionInput.LoverQuestionID = "custom"
	if err := validateRoundSubmission(snapshot, predictionInput, "prediction"); err != nil {
		t.Fatalf("personal prediction rejected: %v", err)
	}

	official := storedSubmission{
		BioAnswers: officialInput.BioAnswers, QuestionAnswers: officialInput.QuestionAnswers,
	}
	prediction := storedSubmission{
		BioAnswers: predictionInput.BioAnswers, QuestionAnswers: predictionInput.QuestionAnswers,
		LoverQuestionID: predictionInput.LoverQuestionID,
	}
	_, _, _, _, _, lines, err := scorePrediction(snapshot, official, prediction, true)
	if err != nil {
		t.Fatalf("personal score rejected: %v", err)
	}
	if len(lines) != 2 {
		t.Fatalf("personal score lines = %#v, want photo plus one custom question", lines)
	}
}

func TestLegacySubmissionGetsPrimaryPhotoFallback(t *testing.T) {
	snapshot := multiplayerTestSnapshot()
	answers := map[string]json.RawMessage{
		"choice": json.RawMessage(`"yes"`),
		"range":  json.RawMessage(`7`),
	}
	if !needsLegacyPrimaryPhotoFallback(snapshot, answers) {
		t.Fatal("complete legacy answers should receive a primary photo fallback")
	}

	addLegacyPrimaryPhotoFallback(answers, "photo-1")
	input := roundSubmissionInput{
		BioAnswers:      map[string]string{"quality": "funny"},
		QuestionAnswers: answers,
	}
	if err := validateRoundSubmission(snapshot, input, "official"); err != nil {
		t.Fatalf("legacy submission rejected after fallback: %v", err)
	}

	addLegacyPrimaryPhotoFallback(answers, "photo-2")
	primaryPhotoID, err := primaryPhotoAnswer(answers)
	if err != nil || primaryPhotoID != "photo-1" {
		t.Fatalf("fallback overwrote the primary photo: %q, %v", primaryPhotoID, err)
	}
}

func TestPrimaryPhotoFallbackRejectsIncompleteOrCurrentSubmission(t *testing.T) {
	snapshot := multiplayerTestSnapshot()
	if needsLegacyPrimaryPhotoFallback(snapshot, map[string]json.RawMessage{
		"choice": json.RawMessage(`"yes"`),
	}) {
		t.Fatal("incomplete answers should not receive a fallback")
	}
	if needsLegacyPrimaryPhotoFallback(snapshot, map[string]json.RawMessage{
		primaryPhotoQuestionID: json.RawMessage(`"photo-1"`),
		"choice":               json.RawMessage(`"yes"`),
		"range":                json.RawMessage(`7`),
	}) {
		t.Fatal("current submissions should not receive a fallback")
	}
}

func TestScorePredictionAppliesLoverAndTaglineBonusDeterministically(t *testing.T) {
	snapshot := multiplayerTestSnapshot()
	official := storedSubmission{
		BioAnswers: map[string]string{"quality": "funny"},
		QuestionAnswers: map[string]json.RawMessage{
			primaryPhotoQuestionID: json.RawMessage(`"photo-1"`),
			"choice":               json.RawMessage(`"yes"`),
			"range":                json.RawMessage(`10`),
		},
	}
	prediction := storedSubmission{
		BioAnswers: map[string]string{"quality": "funny"},
		QuestionAnswers: map[string]json.RawMessage{
			primaryPhotoQuestionID: json.RawMessage(`"photo-1"`),
			"choice":               json.RawMessage(`"yes"`),
			"range":                json.RawMessage(`8`),
		},
		LoverQuestionID: "choice",
	}

	base, lover, tagline, total, exact, lines, err := scorePrediction(snapshot, official, prediction, true)
	if err != nil {
		t.Fatalf("scorePrediction() error = %v", err)
	}
	if base != 38 || lover != 10 || tagline != 10 || total != 58 || exact != 3 {
		t.Fatalf("score breakdown = (%d, %d, %d, %d, %d), want (38, 10, 10, 58, 3)", base, lover, tagline, total, exact)
	}
	if len(lines) != 4 || !lines[2].IsLoverApplied || lines[2].FinalScore != 20 {
		t.Fatalf("unexpected score lines: %#v", lines)
	}

	prediction.QuestionAnswers["choice"] = json.RawMessage(`"no"`)
	base, lover, tagline, total, _, _, err = scorePrediction(snapshot, official, prediction, false)
	if err != nil {
		t.Fatalf("scorePrediction() error = %v", err)
	}
	if base != 28 || lover != -10 || tagline != 0 || total != 18 {
		t.Fatalf("failed LOVER breakdown = (%d, %d, %d, %d), want (28, -10, 0, 18)", base, lover, tagline, total)
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
			primaryPhotoQuestionID: json.RawMessage(`"photo-1"`),
			"choice":               json.RawMessage(`"yes"`),
			"range":                json.RawMessage(`7`),
		},
		LoverQuestionID: "choice",
	}
	if err := validateRoundSubmission(multiplayerTestSnapshot(), input, "prediction"); err == nil {
		t.Fatal("a 101-character tagline should be rejected")
	}
}
