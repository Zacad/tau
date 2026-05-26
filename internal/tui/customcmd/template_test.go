package customcmd

import "testing"

func TestProcessTemplate_ArgumentsPlaceholder(t *testing.T) {
	template := "Run tests with $ARGUMENTS and analyze"
	result := ProcessTemplate(template, "coverage -v", nil)
	want := "Run tests with coverage -v and analyze"
	if result != want {
		t.Errorf("got %q, want %q", result, want)
	}
}

func TestProcessTemplate_PositionalArgs(t *testing.T) {
	template := "Run $1 on $2 with $3"
	result := ProcessTemplate(template, "tests coverage verbose", nil)
	want := "Run tests on coverage with verbose"
	if result != want {
		t.Errorf("got %q, want %q", result, want)
	}
}

func TestProcessTemplate_MixedPlaceholders(t *testing.T) {
	template := "Command: $ARGUMENTS | First: $1 | Second: $2"
	result := ProcessTemplate(template, "arg1 arg2", nil)
	want := "Command: arg1 arg2 | First: arg1 | Second: arg2"
	if result != want {
		t.Errorf("got %q, want %q", result, want)
	}
}

func TestProcessTemplate_MissingPositionalArgs(t *testing.T) {
	template := "$1 and $2 and $3"
	result := ProcessTemplate(template, "only", nil)
	want := "only and  and "
	if result != want {
		t.Errorf("got %q, want %q", result, want)
	}
}

func TestProcessTemplate_NoPlaceholders(t *testing.T) {
	template := "Static template with no placeholders"
	result := ProcessTemplate(template, "some args", nil)
	want := "Static template with no placeholders"
	if result != want {
		t.Errorf("got %q, want %q", result, want)
	}
}

func TestProcessTemplate_EmptyArgs(t *testing.T) {
	template := "Run $ARGUMENTS with $1"
	result := ProcessTemplate(template, "", nil)
	want := "Run  with "
	if result != want {
		t.Errorf("got %q, want %q", result, want)
	}
}

func TestProcessTemplate_ArgumentsAppearsMultipleTimes(t *testing.T) {
	template := "$ARGUMENTS | $ARGUMENTS"
	result := ProcessTemplate(template, "test", nil)
	want := "test | test"
	if result != want {
		t.Errorf("got %q, want %q", result, want)
	}
}

func TestProcessTemplate_PositionalOutOfOrder(t *testing.T) {
	template := "$3 then $1 then $2"
	result := ProcessTemplate(template, "first second third", nil)
	want := "third then first then second"
	if result != want {
		t.Errorf("got %q, want %q", result, want)
	}
}

func TestProcessTemplate_NoArgs(t *testing.T) {
	template := "No args: $1 $2 $3 $ARGUMENTS"
	result := ProcessTemplate(template, "", nil)
	want := "No args:    "
	if result != want {
		t.Errorf("got %q, want %q", result, want)
	}
}

func TestProcessTemplate_SkillResolved(t *testing.T) {
	template := "Use $SKILL:my-skill for this task"
	resolver := func(name string) string {
		if name == "my-skill" {
			return "This is my skill content"
		}
		return ""
	}
	result := ProcessTemplate(template, "", resolver)
	want := "Use This is my skill content for this task"
	if result != want {
		t.Errorf("got %q, want %q", result, want)
	}
}

func TestProcessTemplate_SkillNotFound(t *testing.T) {
	template := "Use $SKILL:missing-skill for this task"
	resolver := func(name string) string { return "" }
	result := ProcessTemplate(template, "", resolver)
	want := "Use [Skill missing-skill not found] for this task"
	if result != want {
		t.Errorf("got %q, want %q", result, want)
	}
}

func TestProcessTemplate_SkillNilResolver(t *testing.T) {
	template := "Use $SKILL:my-skill for this task"
	result := ProcessTemplate(template, "", nil)
	want := "Use $SKILL:my-skill for this task"
	if result != want {
		t.Errorf("got %q, want %q", result, want)
	}
}

func TestProcessTemplate_SkillWithArgs(t *testing.T) {
	template := "$SKILL:research\n\nTopic: $ARGUMENTS"
	resolver := func(name string) string {
		if name == "research" {
			return "# Research Skill\nFollow the workflow..."
		}
		return ""
	}
	result := ProcessTemplate(template, "Go generics", resolver)
	want := "# Research Skill\nFollow the workflow...\n\nTopic: Go generics"
	if result != want {
		t.Errorf("got %q, want %q", result, want)
	}
}

func TestProcessTemplate_MultipleSkills(t *testing.T) {
	template := "$SKILL:a $SKILL:b"
	resolver := func(name string) string {
		switch name {
		case "a":
			return "skill-a"
		case "b":
			return "skill-b"
		}
		return ""
	}
	result := ProcessTemplate(template, "", resolver)
	want := "skill-a skill-b"
	if result != want {
		t.Errorf("got %q, want %q", result, want)
	}
}

func TestProcessTemplate_SkillAtEndOfLine(t *testing.T) {
	template := "$SKILL:web-deep-research\nResearch topic: $ARGUMENTS"
	resolver := func(name string) string {
		if name == "web-deep-research" {
			return "[Skill: web-deep-research]\n# Workflow\nSteps..."
		}
		return ""
	}
	result := ProcessTemplate(template, "test topic", resolver)
	want := "[Skill: web-deep-research]\n# Workflow\nSteps...\nResearch topic: test topic"
	if result != want {
		t.Errorf("got %q, want %q", result, want)
	}
}

func TestProcessTemplate_SkillNameFollowedBySpace(t *testing.T) {
	template := "$SKILL:my-skill more text"
	resolver := func(name string) string {
		if name == "my-skill" {
			return "content"
		}
		return ""
	}
	result := ProcessTemplate(template, "", resolver)
	want := "content more text"
	if result != want {
		t.Errorf("got %q, want %q", result, want)
	}
}
