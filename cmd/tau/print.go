package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"

	"github.com/adam/tau/internal/sdk"
	"github.com/adam/tau/internal/types"
)

// runPrint executes a single prompt and prints the plain text response.
// It supports stdin piping (handled in parseFlags).
func runPrint(cfg cliConfig) {
	logLevel := slog.LevelError
	if os.Getenv("TAU_DEBUG") == "1" {
		logLevel = slog.LevelDebug
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: logLevel,
	})))

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

	// Collect assistant text output
	var mu sync.Mutex
	var response strings.Builder

	unsub := session.Subscribe(func(event types.AgentEvent) {
		if event.Type == types.AgentEventTextDelta {
			if text, ok := event.Data.(string); ok {
				mu.Lock()
				response.WriteString(text)
				mu.Unlock()
			}
		}
	})
	defer unsub()

	if err := session.Prompt(ctx, cfg.Prompt); err != nil {
		exitError(err, exitRuntime)
	}

	mu.Lock()
	text := response.String()
	mu.Unlock()

	fmt.Println(text)
}
