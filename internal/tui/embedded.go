package tui

import "github.com/adam/tau/internal/tui/customcmd"

// EmbeddedCommands returns custom commands that ship with tau by default.
// These are overridden by project-level and global custom commands with the same name.
func EmbeddedCommands() []customcmd.CustomCommand {
	return []customcmd.CustomCommand{
		{
			Name:        "test-command",
			Description: "Verify embedded command loading",
			Template:    "This is an embedded test command. Arguments: $ARGUMENTS. Positional: $1 $2 $3.",
		},
	}
}
