// Package main implements the tau CLI — a thin consumer of the Tau SDK.
//
// Three output modes are supported:
//   - interactive (default): full-screen TUI chat via bubbletea
//   - print (-p): single prompt → plain text output → exit
//   - json (--mode json): single prompt → JSONL events → exit
//
// Print and JSON modes have no TUI dependency and support stdin piping.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"runtime/debug"
	"strings"
)

// version is set by ldflags at build time (e.g., -ldflags "-X main.version=1.0.0").
var version = "dev"

// showVersion prints version and build information then exits.
func showVersion() {
	fmt.Printf("tau %s\n", version)
	if info, ok := debug.ReadBuildInfo(); ok {
		fmt.Printf("Go version: %s\n", info.GoVersion)
		for _, s := range info.Settings {
			switch s.Key {
			case "vcs.revision":
				fmt.Printf("Commit: %s\n", s.Value)
			case "vcs.time":
				fmt.Printf("Build time: %s\n", s.Value)
			}
		}
	}
	os.Exit(0)
}

// mode defines the CLI output mode.
type mode string

const (
	modeInteractive mode = "interactive"
	modePrint       mode = "print"
	modeJSON        mode = "json"
)

// cliConfig holds the parsed CLI flags.
type cliConfig struct {
	Prompt    string // text for print/json mode
	Output    mode   // output mode
	Model     string // model pattern override
	Continue  bool   // resume most recent session
	Ephemeral bool   // disable session persistence
	Resume    bool   // open session picker
	MockURL   string // mock provider URL for E2E testing
}

// parseFlags parses os.Args[1:] and returns the configuration.
// It exits the program on invalid input or help request.
func parseFlags() cliConfig {
	fs := flag.NewFlagSet("tau", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, `tau — AI assistant CLI

Usage: tau [flags]

Modes:
  Interactive (default): Full-screen TUI with chat history
  Print (-p):            Single prompt, plain text output
  JSON (--mode json):    Single prompt, JSONL event output

Flags:
  -p, --print TEXT   Print mode: send TEXT as prompt, output response, exit
  --mode MODE        Output mode: interactive (default), print, json
  --model PATTERN    Model pattern or ID (e.g., gpt-4o, ollama/llama3)
  -c, --continue     Resume most recent session
  -r                 Open session picker to resume a past session
  --no-session       Run in ephemeral mode (no session persistence)
  --mock URL         Use mock provider at URL (for E2E testing)
  --version          Show version information
  -h, --help         Show this help

Interactive Mode Commands:
  /quit, /exit    Exit the application
  /help           Show help message
  /name <name>    Rename the current session
  /session        Show session information
  /model          Change the active model
  /compact        Trigger context compaction
  /clear          Clear the viewport
  /skills         List available skills
  /skill:<name>   Load a skill's content

Keyboard Shortcuts:
  Enter         Send message
  Shift+Enter   New line
  Tab           Auto-complete commands and skill names
  Ctrl+D        Quit (when input is empty)
  Ctrl+C        Abort current response / Exit (double-tap when idle)
  Esc           Clear input

Environment Variables:
  TAU_HOME            Config directory (default: ~/.tau)
  TAU_DEFAULT_MODEL   Default model to use
  TAU_MOCK_URL        Mock provider URL for testing
  TAU_DEBUG=1         Enable debug logging
  OPENAI_API_KEY      OpenAI API key
  ANTHROPIC_API_KEY   Anthropic API key
  GOOGLE_API_KEY      Google API key

Examples:
  tau                               Start interactive chat
  tau -p "Explain quantum computing"  Single prompt
  echo "hello" | tau                Pipe stdin
  tau -c                            Resume last session
  tau --model gpt-4o                Use specific model

`)
	}

	var showVer bool
	fs.BoolVar(&showVer, "version", false, "show version")
	fs.BoolVar(&showVer, "v", false, "show version")

	var prompt string
	var outputMode string
	var model string
	var cont bool
	var ephemeral bool
	var resume bool
	var mockURL string

	fs.StringVar(&prompt, "p", "", "print mode prompt")
	fs.StringVar(&prompt, "print", "", "print mode prompt")
	fs.StringVar(&outputMode, "mode", "interactive", "output mode (interactive, json)")
	fs.StringVar(&model, "model", "", "model pattern or ID")
	fs.BoolVar(&cont, "c", false, "resume most recent session")
	fs.BoolVar(&cont, "continue", false, "resume most recent session")
	fs.BoolVar(&ephemeral, "no-session", false, "ephemeral mode")
	fs.BoolVar(&resume, "r", false, "open session picker to resume a past session")
	fs.StringVar(&mockURL, "mock", "", "mock provider URL for E2E testing")

	if err := fs.Parse(os.Args[1:]); err != nil {
		// flag.NewFlagSet with ContinueOnError returns ErrHelp on -h/--help
		// which we want to handle gracefully, and other errors which should exit.
		if err == flag.ErrHelp {
			os.Exit(0)
		}
		fmt.Fprintf(os.Stderr, "Error: %v\n\n", err)
		fs.Usage()
		os.Exit(2)
	}

	if showVer {
		showVersion()
	}

	// Validate output mode
	m := mode(strings.ToLower(outputMode))
	switch m {
	case modeInteractive, modePrint, modeJSON:
	default:
		fmt.Fprintf(os.Stderr, "Error: unknown mode %q (must be interactive or json)\n", outputMode)
		os.Exit(2)
	}

	// If --mode json is set without -p, treat as print mode with JSON output
	if m == modeJSON && prompt == "" {
		fmt.Fprintln(os.Stderr, "Error: --mode json requires -p prompt text or stdin input")
		os.Exit(2)
	}

	// If prompt is empty and stdin is not a terminal, read from stdin
	if prompt == "" {
		if info, err := os.Stdin.Stat(); err == nil && (info.Mode()&os.ModeCharDevice) == 0 {
			data, err := io.ReadAll(os.Stdin)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: reading stdin: %v\n", err)
				os.Exit(2)
			}
			prompt = strings.TrimSpace(string(data))
		}
	}

	// If we have prompt text (from flag or stdin), set mode to print
	if prompt != "" && m == modeInteractive {
		m = modePrint
	}

	// Validate mode combinations
	if m != modeInteractive && (cont || ephemeral || resume) {
		fmt.Fprintln(os.Stderr, "Error: session flags (-c/-r/--no-session) are only valid in interactive mode")
		os.Exit(2)
	}

	// If mock URL is set, propagate to environment for SDK consumption
	if mockURL != "" {
		os.Setenv("TAU_MOCK_URL", mockURL)
	}

	return cliConfig{
		Prompt:    prompt,
		Output:    m,
		Model:     model,
		Continue:  cont,
		Ephemeral: ephemeral,
		Resume:    resume,
		MockURL:   mockURL,
	}
}

func main() {
	cfg := parseFlags()

	switch cfg.Output {
	case modePrint:
		runPrint(cfg)
	case modeJSON:
		runJSON(cfg)
	case modeInteractive:
		runInteractive(cfg)
	}
}
