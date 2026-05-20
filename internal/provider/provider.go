// Package provider implements the LLM provider abstraction layer.
//
// It defines the Provider interface, model registry, authentication resolution,
// and concrete implementations for OpenAI, Anthropic, Google, and OpenAI-compatible APIs.
//
// Import rules: this package depends only on internal/types and the Go standard library.
package provider

import (
	"context"
	"fmt"

	"github.com/adam/tau/internal/types"
)

// Provider is the interface all LLM providers must implement.
// Each Provider corresponds to one API type (e.g., openai-responses,
// anthropic-messages), not necessarily one vendor.
type Provider interface {
	// Name returns the provider identifier (e.g., "openai", "anthropic").
	Name() string

	// Stream sends messages to the LLM and returns a channel of streaming events.
	// The channel is closed when the response is complete or an error occurs.
	// The caller must consume the channel to completion to avoid goroutine leaks.
	Stream(ctx context.Context, model types.Model, messages []types.AgentMessage, tools []types.ToolDefinition, opts types.StreamOptions) <-chan types.StreamEvent

	// Complete sends messages to the LLM and returns the full response.
	// Internally this may use streaming or a single request depending on the provider.
	Complete(ctx context.Context, model types.Model, messages []types.AgentMessage, tools []types.ToolDefinition, opts types.StreamOptions) (*types.AgentMessage, error)
}

// baseProvider holds fields shared across all provider implementations.
type baseProvider struct {
	name                 string
	httpClient           HTTPClient
	apiKey               string
	skipAPIKeyValidation bool
}

// HTTPClient is the interface for HTTP requests. Using an interface enables
// mocking in tests without needing httptest in every test file.
type HTTPClient interface {
	Do(req *Request) (*Response, error)
}

// Request wraps an outbound HTTP request.
type Request struct {
	Method  string
	URL     string
	Headers map[string]string
	Body    []byte
}

// Response wraps an inbound HTTP response.
type Response struct {
	StatusCode int
	Header     map[string][]string
	Body       []byte
}

// Name returns the provider name.
func (b *baseProvider) Name() string {
	return b.name
}

// apiKeyOrErr returns an error if the API key is empty.
// Some providers (e.g. Ollama) don't require an API key and can skip validation.
func (b *baseProvider) apiKeyOrErr() error {
	if b.skipAPIKeyValidation {
		return nil
	}
	if b.apiKey == "" {
		return fmt.Errorf("API key is empty for provider %s", b.name)
	}
	return nil
}
