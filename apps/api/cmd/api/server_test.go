package main

import (
	"errors"
	"regexp"
	"testing"
)

func TestOrderAndValidateQuestionsPreservesRequestedOrder(t *testing.T) {
	catalog := []question{
		{ID: "first", LoverEligible: true},
		{ID: "second", LoverEligible: true},
	}

	selected, err := orderAndValidateQuestions([]string{"second", "first"}, catalog)
	if err != nil {
		t.Fatalf("orderAndValidateQuestions() error = %v", err)
	}
	if len(selected) != 2 || selected[0].ID != "second" || selected[1].ID != "first" {
		t.Fatalf("orderAndValidateQuestions() = %#v", selected)
	}
}

func TestOrderAndValidateQuestionsRejectsUnknownAndIneligibleQuestions(t *testing.T) {
	tests := []struct {
		name      string
		ids       []string
		catalog   []question
		wantError string
	}{
		{"unknown", []string{"missing"}, []question{{ID: "known", LoverEligible: true}}, "unknown or inactive"},
		{"no lover", []string{"known"}, []question{{ID: "known"}}, "LOVER-eligible"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := orderAndValidateQuestions(test.ids, test.catalog)
			if err == nil || !regexp.MustCompile(test.wantError).MatchString(err.Error()) {
				t.Fatalf("error = %v, want match %q", err, test.wantError)
			}
		})
	}
}

func TestValidateLobbyConfigChange(t *testing.T) {
	ownerID := "owner"
	if err := validateLobbyConfigChange(&ownerID, ownerID, "ready_to_start"); err != nil {
		t.Fatalf("host before start should be allowed: %v", err)
	}
	if err := validateLobbyConfigChange(&ownerID, "guest", "ready_to_start"); !errors.Is(err, errHostRequired) {
		t.Fatalf("guest error = %v, want errHostRequired", err)
	}
	if err := validateLobbyConfigChange(&ownerID, ownerID, "in_game"); !errors.Is(err, errLobbyStarted) {
		t.Fatalf("started lobby error = %v, want errLobbyStarted", err)
	}
}

func TestSnapshotDoesNotTrackLaterConfigChanges(t *testing.T) {
	config := gameConfig{
		ID: "config", Name: "Original", Kind: "personal", Version: 2,
		Questions: []question{{ID: "romance", Label: "Original question"}},
	}
	snapshot := snapshotFromConfig(config)
	config.Name = "Changed"
	config.Questions[0].Label = "Changed question"

	if snapshot.Name != "Original" || snapshot.Questions[0].Label != "Original question" {
		t.Fatalf("snapshot changed with source config: %#v", snapshot)
	}
}

func TestSnapshotProfileFieldsFollowConfigKind(t *testing.T) {
	systemSnapshot := snapshotFromConfig(gameConfig{ID: "classic", Name: "Classique", Kind: "system"})
	if systemSnapshot.Kind != "system" || len(systemSnapshot.ProfileFields) != len(defaultProfileFields()) {
		t.Fatalf("system snapshot = %#v", systemSnapshot)
	}

	personalSnapshot := snapshotFromConfig(gameConfig{ID: "custom", Name: "Maison", Kind: "personal"})
	if personalSnapshot.Kind != "personal" || personalSnapshot.ProfileFields == nil || len(personalSnapshot.ProfileFields) != 0 {
		t.Fatalf("personal snapshot = %#v", personalSnapshot)
	}
}

func TestHydrateQuestionnaireSnapshotKeepsLegacyBehavior(t *testing.T) {
	personalSnapshot := questionnaireSnapshot{Kind: "personal"}
	hydrateQuestionnaireSnapshot(&personalSnapshot)
	if personalSnapshot.ProfileFields == nil || len(personalSnapshot.ProfileFields) != 0 {
		t.Fatalf("personal profile fields = %#v, want an empty list", personalSnapshot.ProfileFields)
	}

	legacySnapshot := questionnaireSnapshot{}
	hydrateQuestionnaireSnapshot(&legacySnapshot)
	if len(legacySnapshot.ProfileFields) != len(defaultProfileFields()) {
		t.Fatalf("legacy profile fields = %#v", legacySnapshot.ProfileFields)
	}

	systemSnapshot := questionnaireSnapshot{Kind: "system"}
	hydrateQuestionnaireSnapshot(&systemSnapshot)
	if len(systemSnapshot.ProfileFields) != len(defaultProfileFields()) {
		t.Fatalf("system profile fields = %#v", systemSnapshot.ProfileFields)
	}
}

func TestRandomLobbyCodeUsesUnambiguousCharacters(t *testing.T) {
	pattern := regexp.MustCompile(`^[ABCDEFGHJKLMNPQRSTUVWXYZ23456789]{6}$`)
	for range 100 {
		code, err := randomLobbyCode()
		if err != nil {
			t.Fatalf("randomLobbyCode() error = %v", err)
		}
		if !pattern.MatchString(code) {
			t.Fatalf("randomLobbyCode() = %q", code)
		}
	}
}

func TestNormalizeCustomQuestion(t *testing.T) {
	minimum := -5
	maximum := 15
	tests := []struct {
		name      string
		input     configQuestionInput
		wantType  string
		wantCount int
	}{
		{
			"number range",
			configQuestionInput{Label: "Niveau de patience", Type: "integer_range", Minimum: &minimum, Maximum: &maximum},
			"integer_range",
			0,
		},
		{
			"text list",
			configQuestionInput{Label: "Activité préférée", Type: "single_choice", Options: []string{"Sport", "Cinéma", "Cuisine"}},
			"single_choice",
			3,
		},
		{
			"yes no",
			configQuestionInput{Label: "Aime voyager ?", Type: "binary_choice"},
			"binary_choice",
			2,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := normalizeCustomQuestion(test.input)
			if err != nil {
				t.Fatalf("normalizeCustomQuestion() error = %v", err)
			}
			if result.Type != test.wantType || len(result.Options) != test.wantCount {
				t.Fatalf("normalizeCustomQuestion() = %#v", result)
			}
			if result.MaximumScore != 10 || !result.LoverEligible || result.Kind != "personal" {
				t.Fatalf("custom defaults = %#v", result)
			}
		})
	}
}

func TestNormalizeCustomQuestionRejectsInvalidSettings(t *testing.T) {
	minimum := 10
	maximum := 5
	tests := []configQuestionInput{
		{Label: "", Type: "binary_choice"},
		{Label: "Range", Type: "integer_range", Minimum: &minimum, Maximum: &maximum},
		{Label: "List", Type: "single_choice", Options: []string{"Only one"}},
		{Label: "Unknown", Type: "short_text"},
	}
	for _, input := range tests {
		if _, err := normalizeCustomQuestion(input); err == nil {
			t.Fatalf("normalizeCustomQuestion(%#v) expected error", input)
		}
	}
}

func TestValidateConfigEnvelopeAcceptsMixedForm(t *testing.T) {
	input := gameConfigInput{
		Name: "Soirée",
		Questions: []configQuestionInput{
			{QuestionID: "romance"},
			{Label: "Aime danser ?", Type: "binary_choice"},
		},
	}
	if err := validateConfigEnvelope(input); err != nil {
		t.Fatalf("validateConfigEnvelope() error = %v", err)
	}
}
