package main

import (
	"fmt"
	"os"
	"strings"
)

const (
	exitSuccess = 0
	exitRuntime = 1
	exitUsage   = 2
)

// exitError formats and prints an error then exits with the given code.
func exitError(err error, code int) {
	msg := friendlyError(err)
	fmt.Fprintf(os.Stderr, "Error: %s\n", msg)
	os.Exit(code)
}

// friendlyError converts internal errors to user-friendly messages.
func friendlyError(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()

	switch {
	case strings.Contains(msg, "no model specified, no default configured"):
		return "No model configured. Set a default model in config or run with --model."
	case strings.Contains(msg, "resolve model"):
		return "No model available. Set a model with --model or configure a default."
	case strings.Contains(msg, "no provider registered"):
		return "No provider available. Set an API key or run Ollama locally."
	case strings.Contains(msg, "API key is empty"):
		return "API key not configured. Set the appropriate *_API_KEY environment variable."
	case strings.Contains(msg, "no sessions found"):
		return "No previous sessions found. Start a new session without -c or -r flags."
	case strings.Contains(msg, "open session"):
		return "Failed to open session file. It may be corrupted or from a different version."
	case strings.Contains(msg, "resume session"):
		return "Failed to resume session. Use -c to continue the most recent session."
	case strings.Contains(msg, "create session"):
		return "Failed to create session. Check permissions and disk space."
	case strings.Contains(msg, "agent prompt"):
		return "Request failed. Check your connection and try again."
	case strings.Contains(msg, "agent continue"):
		return "Continue failed. Check your connection and try again."
	case strings.Contains(msg, "compaction summarization"):
		return "Context compaction failed. The session may be too large to compact."
	case strings.Contains(msg, "persist model change"):
		return "Failed to persist model change to session file."
	default:
		return msg
	}
}
