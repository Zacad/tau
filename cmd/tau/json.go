package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/adam/tau/internal/sdk"
	"github.com/adam/tau/internal/types"
)

// jsonEvent is a single line of JSONL output.
type jsonEvent struct {
	Type      string       `json:"type"`
	Data      string       `json:"data,omitempty"`
	SessionID string       `json:"session_id"`
	Timestamp string       `json:"timestamp"`
	Usage     *types.Usage `json:"usage,omitempty"`
}

func newJSONEvent(event types.AgentEvent, sessionID string) jsonEvent {
	je := jsonEvent{
		Type:      string(event.Type),
		SessionID: sessionID,
	}
	if event.Data != nil {
		if b, err := json.Marshal(event.Data); err == nil {
			je.Data = string(b)
		}
	}
	return je
}

// runJSON executes a single prompt and outputs JSONL events to stdout.
// One JSON object per line, parseable by downstream tools.
func runJSON(cfg cliConfig) {
	if cfg.Prompt == "" {
		fmt.Fprintln(os.Stderr, "Error: no prompt provided (use -p or pipe stdin)")
		os.Exit(exitUsage)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cwd, err := os.Getwd()
	if err != nil {
		cwd = "."
	}

	session, err := sdk.CreateSession(ctx, sdk.SessionOptions{
		Model:      cfg.Model,
		WorkingDir: cwd,
	})
	if err != nil {
		exitError(err, exitRuntime)
	}
	defer session.Close()

	sessionID := session.ID()
	enc := json.NewEncoder(os.Stdout)

	unsub := session.Subscribe(func(event types.AgentEvent) {
		je := newJSONEvent(event, sessionID)

		// Write JSONL line
		if err := enc.Encode(je); err != nil {
			fmt.Fprintf(os.Stderr, "Error writing JSON: %v\n", err)
		}
	})
	defer unsub()

	if err := session.Prompt(ctx, cfg.Prompt); err != nil {
		exitError(err, exitRuntime)
	}

	// Emit final agent_end with usage after Prompt returns (avoids deadlock
	// with the session mutex held during the agent loop's agent_end event).
	usage := session.Usage()
	if usage.TotalTokens > 0 {
		je := jsonEvent{
			Type:      string(types.AgentEventAgentEnd),
			SessionID: sessionID,
			Usage:     &usage,
		}
		_ = enc.Encode(je)
	}
}
