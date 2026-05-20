package skills

import (
	"errors"
	"testing"
)

func TestValidateSkillName(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr error
	}{
		// Valid names
		{"simple", "skill-builder", nil},
		{"single char", "a", nil},
		{"with digits", "skill-123", nil},
		{"starts with digit", "1tool", nil},
		{"all lowercase", "myawesomecapability", nil},

		// Invalid names
		{"empty", "", errEmptyName},
		{"uppercase", "Skill-Builder", errInvalidNameChars},
		{"all uppercase", "SKILL", errInvalidNameChars},
		{"special chars", "skill_builder", errInvalidNameChars},
		{"spaces", "skill builder", errInvalidNameChars},
		{"dots", "skill.builder", errInvalidNameChars},
		{"starts with hyphen", "-skill", errInvalidNameChars},
		{"just hyphens", "---", errInvalidNameChars},
		{"too long", "a-very-long-skill-name-that-exceeds-the-sixty-four-character-limit-for-validation", errNameTooLong},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateSkillName(tt.input)
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("ValidateSkillName(%q) = %v, want %v", tt.input, err, tt.wantErr)
			}
		})
	}
}

func TestValidateDescription(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr error
	}{
		{"valid", "A simple skill", nil},
		{"empty", "", errEmptyDescription},
		{"too long", string(make([]byte, 1025)), errDescriptionTooLong},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateDescription(tt.input)
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("ValidateDescription(%q) error = %v, want %v", tt.input, err, tt.wantErr)
			}
		})
	}
}

func TestSkillValidate(t *testing.T) {
	tests := []struct {
		name    string
		skill   Skill
		wantErr bool
	}{
		{
			name: "valid skill",
			skill: Skill{
				Name:        "test-skill",
				Description: "A test skill",
			},
			wantErr: false,
		},
		{
			name: "invalid name",
			skill: Skill{
				Name:        "Invalid_Name",
				Description: "A test skill",
			},
			wantErr: true,
		},
		{
			name: "missing description",
			skill: Skill{
				Name:        "test-skill",
				Description: "",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.skill.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Skill.Validate() error = %v, wantErr = %v", err, tt.wantErr)
			}
		})
	}
}

func TestSkillSource(t *testing.T) {
	tests := []struct {
		source string
		want   bool
	}{
		{SourceBuiltin, true},
		{SourceGlobal, true},
		{SourceProject, true},
		{"invalid", false},
	}

	for _, tt := range tests {
		t.Run(tt.source, func(t *testing.T) {
			s := Skill{Source: tt.source}
			if got := s.IsValidSource(); got != tt.want {
				t.Errorf("Skill(%q).IsValidSource() = %v, want %v", tt.source, got, tt.want)
			}
		})
	}
}
