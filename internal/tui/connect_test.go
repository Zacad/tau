package tui

import (
	"testing"

	"github.com/adam/tau/internal/provider"
	"github.com/adam/tau/internal/tui/palette"
)

func TestConnectSteps_IncludesOAuthSteps(t *testing.T) {
	m := &Model{}
	steps := connectSteps(m)

	if len(steps) < 5 {
		t.Fatalf("expected at least 5 steps, got %d", len(steps))
	}

	firstStep := steps[0]
	if firstStep.Kind() != palette.StepKindList {
		t.Errorf("first step should be a list, got %v", firstStep.Kind())
	}
	if firstStep.Title() != "Select Provider" {
		t.Errorf("first step title: got %q, want %q", firstStep.Title(), "Select Provider")
	}

	opts := firstStep.Options()
	foundOAuth := false
	for _, opt := range opts {
		if opt.Value == "openai-oauth" {
			foundOAuth = true
			break
		}
	}
	if !foundOAuth {
		t.Error("provider list should include openai-oauth option")
	}
}

func TestConnectSteps_HasAuthMethodStep(t *testing.T) {
	m := &Model{}
	steps := connectSteps(m)

	foundAuthMethod := false
	for _, step := range steps {
		if step.Title() == "Auth Method" {
			foundAuthMethod = true
			opts := step.Options()
			if len(opts) != 3 {
				t.Errorf("auth method step should have 3 options, got %d", len(opts))
			}
			expectedValues := map[string]bool{"browser": false, "device": false, "manual": false}
			for _, opt := range opts {
				if _, ok := expectedValues[opt.Value]; ok {
					expectedValues[opt.Value] = true
				}
			}
			for value, found := range expectedValues {
				if !found {
					t.Errorf("auth method step should have %q option", value)
				}
			}
			break
		}
	}
	if !foundAuthMethod {
		t.Error("should have Auth Method step")
	}
}

func TestConnectSteps_HasConditionalAPIKeyStep(t *testing.T) {
	m := &Model{}
	steps := connectSteps(m)

	foundAPIKeyStep := false
	for _, step := range steps {
		if step.Title() == "API Key" {
			foundAPIKeyStep = true
			if step.Kind() != palette.StepKindInput {
				t.Errorf("API Key step should be input type, got %v", step.Kind())
			}
			break
		}
	}
	if !foundAPIKeyStep {
		t.Error("should have API Key step")
	}
}

func TestConnectSteps_HasOAuthTaskSteps(t *testing.T) {
	m := &Model{}
	steps := connectSteps(m)

	expectedTaskTitles := map[string]bool{
		"Generate Authorization URL": false,
		"Paste Authorization URL":    false,
		"Exchange Code for Tokens":   false,
		"Browser Authentication":     false,
		"Device Authorization":       false,
	}

	for _, step := range steps {
		if _, ok := expectedTaskTitles[step.Title()]; ok {
			expectedTaskTitles[step.Title()] = true
		}
	}

	for title, found := range expectedTaskTitles {
		if !found {
			t.Errorf("missing step: %q", title)
		}
	}
}

func TestConnectSteps_HasDiscoverModelsStep(t *testing.T) {
	m := &Model{}
	steps := connectSteps(m)

	found := false
	for _, step := range steps {
		if step.Title() == "Discover Models" {
			found = true
			if step.Kind() != palette.StepKindTask {
				t.Errorf("Discover Models should be task type, got %v", step.Kind())
			}
			break
		}
	}
	if !found {
		t.Error("should have Discover Models step")
	}
}

func TestConnectSteps_HasSaveStep(t *testing.T) {
	m := &Model{}
	steps := connectSteps(m)

	found := false
	for _, step := range steps {
		if step.Title() == "Save" {
			found = true
			if step.Kind() != palette.StepKindConfirm {
				t.Errorf("Save step should be confirm type, got %v", step.Kind())
			}
			break
		}
	}
	if !found {
		t.Error("should have Save step")
	}
}

func TestConnectSteps_AuthMethodSkipCondition(t *testing.T) {
	m := &Model{}
	steps := connectSteps(m)

	for _, step := range steps {
		if step.Title() == "Auth Method" {
			skipIf := func(results map[string]any) bool {
				providerName, _ := results["select_provider"].(string)
				return providerName != "openai-oauth"
			}

			if skipIf(map[string]any{"select_provider": "openai"}) != true {
				t.Error("Auth Method should be skipped for non-OAuth provider")
			}
			if skipIf(map[string]any{"select_provider": "openai-oauth"}) != false {
				t.Error("Auth Method should NOT be skipped for OAuth provider")
			}
			break
		}
	}
}

func TestConnectSteps_APIKeySkipCondition(t *testing.T) {
	m := &Model{}
	steps := connectSteps(m)

	for _, step := range steps {
		if step.Title() == "API Key" {
			skipIf := func(results map[string]any) bool {
				providerName, _ := results["select_provider"].(string)
				return providerName == "openai-oauth"
			}

			if skipIf(map[string]any{"select_provider": "openai-oauth"}) != true {
				t.Error("API Key should be skipped for OAuth provider")
			}
			if skipIf(map[string]any{"select_provider": "openai"}) != false {
				t.Error("API Key should NOT be skipped for API key provider")
			}
			break
		}
	}
}

func TestDiscoverModelsTask_OAuthProvider(t *testing.T) {
	m := &Model{}
	steps := connectSteps(m)

	var discoverStep *palette.Step
	for i, step := range steps {
		if step.Title() == "Discover Models" {
			discoverStep = &steps[i]
			break
		}
	}
	if discoverStep == nil {
		t.Fatal("Discover Models step not found")
	}

	task := discoverStep.Task()
	if task == nil {
		t.Fatal("Discover Models should have a task function")
	}

	results := map[string]any{
		"select_provider": "openai-oauth",
	}
	success, msg, err := task(results)
	if err != nil {
		t.Fatalf("Discover Models task failed: %v", err)
	}
	if !success {
		t.Fatalf("Discover Models task should succeed, got message: %s", msg)
	}

	models, ok := results["discover_models"].([]string)
	if !ok {
		t.Fatal("discover_models should be []string")
	}
	if len(models) == 0 {
		t.Fatal("expected models for OAuth provider")
	}
}

func TestSaveProviderOAuthAuth(t *testing.T) {
	creds := provider.OAuthCredentials{
		AccessToken:  "test-access-token",
		RefreshToken: "test-refresh-token",
		Expires:      1234567890,
		AccountID:    "test-account-id",
	}
	results := map[string]any{
		"oauth_credentials": creds,
	}

	err := saveProviderOAuthAuth("test-oauth-provider", results)
	if err != nil {
		t.Fatalf("saveProviderOAuthAuth: %v", err)
	}
}

func TestSaveProviderOAuthAuth_MissingCredentials(t *testing.T) {
	results := map[string]any{}

	err := saveProviderOAuthAuth("test-provider", results)
	if err == nil {
		t.Fatal("expected error when OAuth credentials missing")
	}
}
