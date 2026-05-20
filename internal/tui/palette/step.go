package palette

import (
	tea "charm.land/bubbletea/v2"
)

// ListOption is a selectable option in a ListStep.
type ListOption struct {
	Title       string
	Description string
	Value       string
}

// StepKind identifies the type of a step.
type StepKind int

const (
	StepKindList StepKind = iota
	StepKindInput
	StepKindConfirm
	StepKindTask
	StepKindMessage
)

// Step is a single step in a multi-step flow.
type Step struct {
	kind StepKind

	// List step fields
	title   string
	prompt  string
	options []ListOption

	// Input step fields
	placeholder string
	skipIf      func(results map[string]any) bool
	skipValue   string

	// Task step field
	task StepTaskFunc

	// Message step field
	message string
}

// StepTaskFunc is a function that performs async work for a TaskStep.
// It receives prior step results.
type StepTaskFunc func(results map[string]any) (success bool, message string, err error)

// ListStep creates a step that shows a selectable list.
func ListStep(title, prompt string, options []ListOption) Step {
	return Step{kind: StepKindList, title: title, prompt: prompt, options: options}
}

// InputStep creates a step that prompts for text input.
func InputStep(title, prompt, placeholder string) Step {
	return Step{kind: StepKindInput, title: title, prompt: prompt, placeholder: placeholder}
}

// ConditionalInputStep creates an input step that can be skipped based on prior results.
func ConditionalInputStep(title, prompt, placeholder string, skipIf func(map[string]any) bool, skipValue string) Step {
	return Step{kind: StepKindInput, title: title, prompt: prompt, placeholder: placeholder, skipIf: skipIf, skipValue: skipValue}
}

// ConfirmStep creates a yes/no confirmation step.
func ConfirmStep(title, prompt string) Step {
	return Step{kind: StepKindConfirm, title: title, prompt: prompt}
}

// TaskStep creates a step that runs an async function with a spinner.
func TaskStep(title string, task StepTaskFunc) Step {
	return Step{kind: StepKindTask, title: title, task: task}
}

// MessageStep creates a step that displays a static message.
func MessageStep(title, message string) Step {
	return Step{kind: StepKindMessage, title: title, message: message}
}

// Kind returns the step type.
func (s *Step) Kind() StepKind { return s.kind }

// Title returns the step title.
func (s *Step) Title() string { return s.title }

// Prompt returns the step prompt (for list and input steps).
func (s *Step) Prompt() string { return s.prompt }

// Options returns the list options (for list steps).
func (s *Step) Options() []ListOption { return s.options }

// Placeholder returns the input placeholder (for input steps).
func (s *Step) Placeholder() string { return s.placeholder }

// Task returns the task function (for task steps).
func (s *Step) Task() StepTaskFunc { return s.task }

// Message returns the message text (for message steps).
func (s *Step) Message() string { return s.message }

// ResultKey returns the key used to store this step's result.
func (s *Step) ResultKey() string {
	if s.title != "" {
		return normalizeKey(s.title)
	}
	return "value"
}

func normalizeKey(s string) string {
	var out []byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		} else if c == ' ' {
			c = '_'
		}
		out = append(out, c)
	}
	return string(out)
}

// StepRunner sequences through Steps, routing to palette components.
type StepRunner struct {
	steps    []Step
	current  int
	results  map[string]any
	cancelled bool
	finished bool
	title    string
}

// NewRunner creates a StepRunner with the given steps.
func NewRunner(steps []Step, title string) *StepRunner {
	return &StepRunner{
		steps:   steps,
		results: make(map[string]any),
		title:   title,
	}
}

// Init initializes the first step.
func (r *StepRunner) Init() tea.Cmd {
	if len(r.steps) == 0 {
		r.finished = true
		return nil
	}
	r.current = 0
	r.skipIfNeeded()
	return nil
}

// CurrentStep returns the active step, or nil if done/out of bounds.
func (r *StepRunner) CurrentStep() *Step {
	if r.current < 0 || r.current >= len(r.steps) {
		return nil
	}
	return &r.steps[r.current]
}

// IsFinished reports whether all steps completed.
func (r *StepRunner) IsFinished() bool { return r.finished }

// IsCancelled reports whether the runner was cancelled.
func (r *StepRunner) IsCancelled() bool { return r.cancelled }

// IsActive reports whether the runner is still running.
func (r *StepRunner) IsActive() bool { return !r.cancelled && !r.finished }

// Results returns collected results from completed steps.
func (r *StepRunner) Results() map[string]any { return r.results }

// Title returns the runner's title.
func (r *StepRunner) Title() string { return r.title }

// Cancel marks the runner as cancelled.
func (r *StepRunner) Cancel() { r.cancelled = true }

// RecordResult stores a result from the current step and advances to the next.
func (r *StepRunner) RecordResult(key string, value any) {
	r.results[key] = value
	r.advance()
}

// RecordResults stores multiple results and advances.
func (r *StepRunner) RecordResults(m map[string]any) {
	for k, v := range m {
		r.results[k] = v
	}
	r.advance()
}

// SkipIfNeeded checks if the current step should be auto-skipped.
func (r *StepRunner) SkipIfNeeded() {
	r.skipIfNeeded()
}

func (r *StepRunner) advance() {
	r.current++
	if r.current >= len(r.steps) {
		r.finished = true
		return
	}
	r.skipIfNeeded()
}

func (r *StepRunner) skipIfNeeded() {
	step := r.CurrentStep()
	if step == nil {
		return
	}
	if step.kind == StepKindInput && step.skipIf != nil && step.skipIf(r.results) {
		if step.skipValue != "" {
			r.results[step.ResultKey()] = step.skipValue
		}
		r.advance()
	}
}
