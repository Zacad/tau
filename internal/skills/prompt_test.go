package skills

import (
	"testing"
)

func TestFormatForPrompt(t *testing.T) {
	t.Run("empty list", func(t *testing.T) {
		got := FormatForPrompt(nil)
		want := "<skills>\n</skills>"
		if got != want {
			t.Errorf("FormatForPrompt(nil) = %q, want %q", got, want)
		}
	})

	t.Run("single skill", func(t *testing.T) {
		skills := []*Skill{
			{Name: "test-skill", Description: "A test skill"},
		}
		got := FormatForPrompt(skills)
		want := `<skills>
<skill name="test-skill" description="A test skill"/>
</skills>`
		if got != want {
			t.Errorf("FormatForPrompt() = %q, want %q", got, want)
		}
	})

	t.Run("multiple skills", func(t *testing.T) {
		skills := []*Skill{
			{Name: "skill-a", Description: "First skill"},
			{Name: "skill-b", Description: "Second skill"},
		}
		got := FormatForPrompt(skills)
		want := `<skills>
<skill name="skill-a" description="First skill"/>
<skill name="skill-b" description="Second skill"/>
</skills>`
		if got != want {
			t.Errorf("FormatForPrompt() = %q, want %q", got, want)
		}
	})

	t.Run("excludes disabled skills", func(t *testing.T) {
		skills := []*Skill{
			{Name: "visible-skill", Description: "Visible"},
			{Name: "hidden-skill", Description: "Hidden", DisableModelInvocation: true},
			{Name: "also-visible", Description: "Also visible"},
		}
		got := FormatForPrompt(skills)
		if containsStr(got, "hidden-skill") {
			t.Errorf("FormatForPrompt() should exclude disabled skill, got: %s", got)
		}
		if !containsStr(got, "visible-skill") {
			t.Errorf("FormatForPrompt() should include visible-skill, got: %s", got)
		}
		if !containsStr(got, "also-visible") {
			t.Errorf("FormatForPrompt() should include also-visible, got: %s", got)
		}
	})

	t.Run("all disabled", func(t *testing.T) {
		skills := []*Skill{
			{Name: "hidden", Description: "Hidden", DisableModelInvocation: true},
		}
		got := FormatForPrompt(skills)
		want := "<skills>\n</skills>"
		if got != want {
			t.Errorf("FormatForPrompt() = %q, want %q", got, want)
		}
	})

	t.Run("escapes XML special chars in name", func(t *testing.T) {
		// While skill names should only contain [a-z0-9-], we still
		// escape defensively.
		skills := []*Skill{
			{Name: "safe-skill", Description: "Has <special> & \"chars\""},
		}
		got := FormatForPrompt(skills)
		if containsStr(got, "<special>") || containsStr(got, "& \"chars\"") {
			t.Errorf("FormatForPrompt() did not escape XML chars: %s", got)
		}
		if !containsStr(got, "&lt;special&gt;") || !containsStr(got, "&quot;chars&quot;") {
			t.Errorf("FormatForPrompt() escaping wrong: %s", got)
		}
	})

	t.Run("escapes XML in description", func(t *testing.T) {
		skills := []*Skill{
			{Name: "test", Description: "A & B < C > D"},
		}
		got := FormatForPrompt(skills)
		want := `<skills>
<skill name="test" description="A &amp; B &lt; C &gt; D"/>
</skills>`
		if got != want {
			t.Errorf("FormatForPrompt() = %q, want %q", got, want)
		}
	})
}

func containsStr(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 || findSubstring(s, sub))
}

func findSubstring(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
