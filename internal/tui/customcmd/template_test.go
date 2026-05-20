package customcmd

import "testing"

func TestProcessTemplate_ArgumentsPlaceholder(t *testing.T) {
	template := "Run tests with $ARGUMENTS and analyze"
	result := ProcessTemplate(template, "coverage -v")
	want := "Run tests with coverage -v and analyze"
	if result != want {
		t.Errorf("got %q, want %q", result, want)
	}
}

func TestProcessTemplate_PositionalArgs(t *testing.T) {
	template := "Run $1 on $2 with $3"
	result := ProcessTemplate(template, "tests coverage verbose")
	want := "Run tests on coverage with verbose"
	if result != want {
		t.Errorf("got %q, want %q", result, want)
	}
}

func TestProcessTemplate_MixedPlaceholders(t *testing.T) {
	template := "Command: $ARGUMENTS | First: $1 | Second: $2"
	result := ProcessTemplate(template, "arg1 arg2")
	want := "Command: arg1 arg2 | First: arg1 | Second: arg2"
	if result != want {
		t.Errorf("got %q, want %q", result, want)
	}
}

func TestProcessTemplate_MissingPositionalArgs(t *testing.T) {
	template := "$1 and $2 and $3"
	result := ProcessTemplate(template, "only")
	want := "only and  and "
	if result != want {
		t.Errorf("got %q, want %q", result, want)
	}
}

func TestProcessTemplate_NoPlaceholders(t *testing.T) {
	template := "Static template with no placeholders"
	result := ProcessTemplate(template, "some args")
	want := "Static template with no placeholders"
	if result != want {
		t.Errorf("got %q, want %q", result, want)
	}
}

func TestProcessTemplate_EmptyArgs(t *testing.T) {
	template := "Run $ARGUMENTS with $1"
	result := ProcessTemplate(template, "")
	want := "Run  with "
	if result != want {
		t.Errorf("got %q, want %q", result, want)
	}
}

func TestProcessTemplate_ArgumentsAppearsMultipleTimes(t *testing.T) {
	template := "$ARGUMENTS | $ARGUMENTS"
	result := ProcessTemplate(template, "test")
	want := "test | test"
	if result != want {
		t.Errorf("got %q, want %q", result, want)
	}
}

func TestProcessTemplate_PositionalOutOfOrder(t *testing.T) {
	template := "$3 then $1 then $2"
	result := ProcessTemplate(template, "first second third")
	want := "third then first then second"
	if result != want {
		t.Errorf("got %q, want %q", result, want)
	}
}

func TestProcessTemplate_NoArgs(t *testing.T) {
	template := "No args: $1 $2 $3 $ARGUMENTS"
	result := ProcessTemplate(template, "")
	want := "No args:    "
	if result != want {
		t.Errorf("got %q, want %q", result, want)
	}
}
