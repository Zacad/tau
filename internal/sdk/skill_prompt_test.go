package sdk

import (
	"strings"
	"testing"
)

func TestBuildSystemPromptWithSkills_IncludesUsageInstructions(t *testing.T) {
	base := "You are Tau."
	skillsXML := `<skills>
<skill name="web-deep-research" description="Perform web research"/>
</skills>`

	result := buildSystemPromptWithSkills(base, skillsXML, true)

	if !strings.Contains(result, "Skills provide specialized instructions") {
		t.Error("expected skill usage instructions in system prompt")
	}
	if !strings.Contains(result, "When a user request matches a skill's description") {
		t.Error("expected instruction to match skill descriptions")
	}
	if !strings.Contains(result, skillsXML) {
		t.Error("expected skills XML in system prompt")
	}
	if !strings.Contains(result, "reference them to determine the appropriate workflow") {
		t.Error("expected instruction to reference skills for workflow")
	}
}

func TestBuildSystemPromptWithSkills_NoSkills(t *testing.T) {
	base := "You are Tau."
	result := buildSystemPromptWithSkills(base, "", false)

	if result != base {
		t.Errorf("expected base prompt unchanged when no skills, got: %q", result)
	}
}

func TestBuildSystemPromptWithSkills_EmptyXML(t *testing.T) {
	base := "You are Tau."
	result := buildSystemPromptWithSkills(base, "", true)

	if !strings.Contains(result, "Skills provide specialized instructions") {
		t.Error("expected skill usage instructions even with empty XML")
	}
}

func TestBuildSystemPromptWithSkills_FullIntegration(t *testing.T) {
	result := buildSystemPromptWithSkills(defaultSystemPrompt, `<skills>
<skill name="web-deep-research" description="Perform comprehensive web research"/>
<skill name="skill-builder" description="Create or update SKILL.md files"/>
</skills>`, true)

	if !strings.Contains(result, "You are Tau") {
		t.Error("expected base prompt content")
	}
	if !strings.Contains(result, "web-deep-research") {
		t.Error("expected web-deep-research skill in prompt")
	}
	if !strings.Contains(result, "skill-builder") {
		t.Error("expected skill-builder skill in prompt")
	}
	if !strings.Contains(result, "Skills provide specialized instructions") {
		t.Error("expected skill usage instructions")
	}

	t.Logf("System prompt length: %d characters", len(result))
	t.Logf("System prompt contains skills section: %v", strings.Contains(result, "<skills>"))
	t.Logf("System prompt contains usage instructions: %v", strings.Contains(result, "When a user request matches"))
}
