package types

import (
	"time"
)

// MessageRole defines the role of an AgentMessage sender.
type MessageRole string

const (
	// RoleUser represents a user-originated message.
	RoleUser MessageRole = "user"

	// RoleAssistant represents an assistant (LLM) response.
	RoleAssistant MessageRole = "assistant"

	// RoleToolResult represents a tool execution result.
	RoleToolResult MessageRole = "tool_result"
)

// AgentMessage represents a single message in the agent conversation transcript.
// It is persisted to session storage (JSONL) and used throughout the agent loop.
type AgentMessage struct {
	ID        string         `json:"id"`
	Role      MessageRole    `json:"role"`
	Content   []ContentBlock `json:"content"`
	Timestamp time.Time      `json:"timestamp"`
	// API identifies which provider API produced this assistant message.
	// E.g., "openai-responses", "anthropic-messages", "google-generative-ai".
	API string `json:"api,omitempty"`
	// Model is the model ID that generated this response.
	Model string `json:"model,omitempty"`
	// ToolCallID links a tool_result message to the original tool call.
	// Required by the OpenAI Chat Completions API for tool results.
	ToolCallID string `json:"tool_call_id,omitempty"`
}

// ContentBlockType defines the type of content within a message.
type ContentBlockType string

const (
	// BlockText represents plain text content.
	BlockText ContentBlockType = "text"

	// BlockThinking represents model reasoning/thinking output.
	BlockThinking ContentBlockType = "thinking"

	// BlockToolCall represents a tool call request from the model.
	BlockToolCall ContentBlockType = "tool_call"

	// BlockImage represents an image block (base64 encoded).
	BlockImage ContentBlockType = "image"
)

// ContentBlock is a single block of content within an AgentMessage.
// Only the field matching Type should be populated.
type ContentBlock struct {
	Type     ContentBlockType `json:"type"`
	Text     string           `json:"text,omitempty"`
	ToolCall *ToolCallBlock   `json:"tool_call,omitempty"`
	Image    *ImageBlock      `json:"image,omitempty"`
}

// ToolCallBlock represents a tool call request from the LLM.
type ToolCallBlock struct {
	ID        string         `json:"id"`
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

// ImageBlock represents an image within a message.
type ImageBlock struct {
	Data     string `json:"data"`
	MimeType string `json:"mime_type"`
}
