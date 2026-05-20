package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	tea "charm.land/bubbletea/v2"

	"github.com/adam/tau/internal/sdk"
	"github.com/adam/tau/internal/tui"
)

// runInteractive starts the full-screen TUI chat mode using bubbletea.
func runInteractive(cfg cliConfig) {
	// Suppress all non-error logs in TUI mode to prevent terminal corruption.
	// Set TAU_DEBUG=1 to enable debug logging (goes to stderr).
	logLevel := slog.LevelError
	if os.Getenv("TAU_DEBUG") == "1" {
		logLevel = slog.LevelDebug
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: logLevel,
	})))
	// Resolve working directory
	cwd, err := os.Getwd()
	if err != nil {
		exitError(fmt.Errorf("cannot determine working directory: %w", err), exitRuntime)
	}

	// Handle session picker mode
	var sessionPath string
	if cfg.Resume {
		sessionPath, err = runSessionPicker(cwd)
		if err != nil {
			exitError(err, exitRuntime)
		}
		if sessionPath == "" {
			// User cancelled
			os.Exit(0)
		}
	}

	// Create context with signal handling for graceful shutdown
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// Create SDK session
	session, err := sdk.CreateSession(ctx, sdk.SessionOptions{
		Model:       cfg.Model,
		WorkingDir:  cwd,
		SessionPath: sessionPath,
		Continue:    cfg.Continue,
		Ephemeral:   cfg.Ephemeral,
	})
	if err != nil {
		exitError(err, exitRuntime)
	}

	// Ensure session is closed on exit
	defer func() {
		if err := session.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to close session: %v\n", err)
		}
	}()

	// Create the TUI model
	model := tui.NewModel(session)

	// Run the bubbletea program
	p := tea.NewProgram(model)
	model.SetProgram(p)

	// Handle signal-based shutdown (SIGTERM, or SIGINT if not caught by TUI)
	go func() {
		<-ctx.Done()
		p.Send(tea.QuitMsg{})
	}()

	if _, err := p.Run(); err != nil {
		exitError(err, exitRuntime)
	}
}
