package skills

import (
	"strings"
	"testing"
)

func TestParseSkillMD(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		dirName string
		want    *Skill
		wantErr bool
	}{
		{
			name: "valid skill",
			input: `---
name: test-skill
description: A test skill for validation
---

# Test Skill

This is the skill content.
`,
			dirName: "test-skill",
			want: &Skill{
				Name:        "test-skill",
				Description: "A test skill for validation",
				Content:     "# Test Skill\n\nThis is the skill content.",
			},
			wantErr: false,
		},
		{
			name: "missing name",
			input: `---
description: No name here
---

Content
`,
			dirName: "some-skill",
			want:    nil,
			wantErr: true,
		},
		{
			name: "missing description",
			input: `---
name: test-skill
---

Content
`,
			dirName: "test-skill",
			want:    nil,
			wantErr: true,
		},
		{
			name: "invalid name chars",
			input: `---
name: Invalid_Name
description: Bad name
---

Content
`,
			dirName: "Invalid_Name",
			want:    nil,
			wantErr: true,
		},
		{
			name: "name mismatch with directory",
			input: `---
name: skill-a
description: A skill
---

Content
`,
			dirName: "skill-b",
			want:    nil,
			wantErr: true,
		},
		{
			name: "name too long",
			input: `---
name: ` + strings.Repeat("a", 65) + `
description: Long name
---

Content
`,
			dirName: strings.Repeat("a", 65),
			want:    nil,
			wantErr: true,
		},
		{
			name: "description too long",
			input: `---
name: test-skill
description: ` + strings.Repeat("x", 1025) + `
---

Content
`,
			dirName: "test-skill",
			want:    nil,
			wantErr: true,
		},
		{
			name:    "malformed frontmatter - no closing dashes",
			input:   `---` + "\nname: test\n\nContent",
			dirName: "test",
			want:    nil,
			wantErr: true,
		},
		{
			name:    "no frontmatter at all",
			input:   "# Just markdown\n\nNo frontmatter here.",
			dirName: "test",
			want:    nil,
			wantErr: true,
		},
		{
			name:    "empty file",
			input:   "",
			dirName: "test",
			want:    nil,
			wantErr: true,
		},
		{
			name: "with disable_model_invocation flag",
			input: `---
name: test-skill
description: Internal skill
disable_model_invocation: true
---

Internal content.
`,
			dirName: "test-skill",
			want: &Skill{
				Name:                   "test-skill",
				Description:            "Internal skill",
				DisableModelInvocation: true,
				Content:                "Internal content.",
			},
			wantErr: false,
		},
		{
			name: "with scripts references assets",
			input: `---
name: test-skill
description: Full skill
scripts:
  - scripts/setup.sh
references:
  - docs/api.md
assets:
  - images/logo.png
---

Full content.
`,
			dirName: "test-skill",
			want: &Skill{
				Name:        "test-skill",
				Description: "Full skill",
				Scripts:     []string{"scripts/setup.sh"},
				References:  []string{"docs/api.md"},
				Assets:      []string{"images/logo.png"},
				Content:     "Full content.",
			},
			wantErr: false,
		},
		{
			name: "multiline description",
			input: `---
name: test-skill
description: |
  This is a multiline
  description for testing
---

Content.
`,
			dirName: "test-skill",
			want: &Skill{
				Name:        "test-skill",
				Description: "This is a multiline\ndescription for testing\n",
				Content:     "Content.",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			skill, err := ParseSkillMD(strings.NewReader(tt.input), tt.dirName)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseSkillMD() error = %v, wantErr = %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}
			if skill == nil {
				t.Fatal("ParseSkillMD() returned nil skill without error")
			}
			if skill.Name != tt.want.Name {
				t.Errorf("Name = %q, want %q", skill.Name, tt.want.Name)
			}
			if skill.Description != tt.want.Description {
				t.Errorf("Description = %q, want %q", skill.Description, tt.want.Description)
			}
			if skill.Content != tt.want.Content {
				t.Errorf("Content = %q, want %q", skill.Content, tt.want.Content)
			}
			if skill.DisableModelInvocation != tt.want.DisableModelInvocation {
				t.Errorf("DisableModelInvocation = %v, want %v",
					skill.DisableModelInvocation, tt.want.DisableModelInvocation)
			}
			if len(skill.Scripts) != len(tt.want.Scripts) {
				t.Errorf("Scripts count = %d, want %d", len(skill.Scripts), len(tt.want.Scripts))
			}
			if len(skill.References) != len(tt.want.References) {
				t.Errorf("References count = %d, want %d",
					len(skill.References), len(tt.want.References))
			}
			if len(skill.Assets) != len(tt.want.Assets) {
				t.Errorf("Assets count = %d, want %d", len(skill.Assets), len(tt.want.Assets))
			}
		})
	}
}

func TestParseSkillMD_NameMismatch(t *testing.T) {
	input := `---
name: skill-a
description: A skill
---

Content
`
	_, err := ParseSkillMD(strings.NewReader(input), "skill-b")
	if err == nil {
		t.Fatal("expected error for name mismatch, got nil")
	}
}
