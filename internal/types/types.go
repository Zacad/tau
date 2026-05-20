// Package types provides the core data structures for the Tau agentic coding tool.
//
// This package has no internal dependencies and serves as the shared data layer
// for all other internal packages. It defines messages, events, tool results,
// session entries, provider types, and model metadata.
//
// External dependencies: github.com/invopop/jsonschema (for ToolDefinition schema generation).
package types

// ExecutionMode defines how a tool can be executed relative to other tools.
type ExecutionMode string

const (
	// ExecutionParallel allows concurrent execution with other parallel tools.
	ExecutionParallel ExecutionMode = "parallel"

	// ExecutionSequential requires one-at-a-time execution, serialized via per-file mutex.
	ExecutionSequential ExecutionMode = "sequential"

	// ExecutionExclusive runs alone — no other tool executes concurrently.
	ExecutionExclusive ExecutionMode = "exclusive_per_file"
)
