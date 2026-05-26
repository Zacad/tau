package provider

import (
	"encoding/json"
	"testing"

	"github.com/adam/tau/internal/types"
	"github.com/invopop/jsonschema"
)

func TestSanitizeToolSchema_NilSchema(t *testing.T) {
	result := sanitizeToolSchema(nil)
	if result != nil {
		t.Fatalf("expected nil, got %v", result)
	}
}

func TestSanitizeToolSchema_RemovesMetaFields(t *testing.T) {
	schema := &jsonschema.Schema{
		Version: "https://json-schema.org/draft/2020-12/schema",
		ID:      "test-schema",
		Type:    "object",
	}

	result := sanitizeToolSchema(schema)

	if _, ok := result["$schema"]; ok {
		t.Error("expected $schema to be removed")
	}
	if _, ok := result["$id"]; ok {
		t.Error("expected $id to be removed")
	}
	if result["type"] != "object" {
		t.Errorf("expected type=object, got %v", result["type"])
	}
}

func TestSanitizeToolSchema_InlinesRefs(t *testing.T) {
	// Use reflector to create a schema with nested types (generates $refs)
	type Config struct {
		Name string `json:"name"`
	}
	type Params struct {
		Query  string `json:"query"`
		Config Config `json:"config"`
	}

	reflector := jsonschema.Reflector{}
	schema := reflector.Reflect(Params{})

	result := sanitizeToolSchema(schema)

	// $defs should be removed
	if _, ok := result["$defs"]; ok {
		t.Error("expected $defs to be removed")
	}

	// Check that properties exist and config is inlined
	props, ok := result["properties"].(map[string]any)
	if !ok {
		t.Fatalf("expected properties to be map, got %T", result["properties"])
	}

	configSchema, ok := props["config"].(map[string]any)
	if !ok {
		t.Fatalf("expected config to be map, got %T", props["config"])
	}

	// The inlined schema should not have $ref
	if _, hasRef := configSchema["$ref"]; hasRef {
		t.Error("expected $ref to be inlined in config property")
	}

	// The inlined schema should have type: object
	if configSchema["type"] != "object" {
		t.Errorf("expected config type=object after inlining, got %v", configSchema["type"])
	}
}

func TestSanitizeToolSchema_AddsObjectType(t *testing.T) {
	// Schema with properties but no explicit type should get type: "object"
	type Params struct {
		Query string `json:"query"`
	}

	reflector := jsonschema.Reflector{}
	schema := reflector.Reflect(Params{})

	result := sanitizeToolSchema(schema)

	if result["type"] != "object" {
		t.Errorf("expected type=object, got %v", result["type"])
	}
}

func TestSanitizeToolSchema_FiltersRequired(t *testing.T) {
	type Params struct {
		Query       string `json:"query" jsonschema:"required"`
		NonRequired string `json:"non_required"`
	}

	reflector := jsonschema.Reflector{}
	schema := reflector.Reflect(Params{})

	result := sanitizeToolSchema(schema)

	req, ok := result["required"].([]any)
	if !ok {
		t.Fatalf("expected required to be array, got %T", result["required"])
	}

	// Should only contain fields that exist in properties
	props := result["properties"].(map[string]any)
	for _, r := range req {
		field := r.(string)
		if _, exists := props[field]; !exists {
			t.Errorf("required field %q does not exist in properties", field)
		}
	}
}

func TestToolDefToSchema(t *testing.T) {
	// Create a ToolDefinition with a real jsonschema.Schema
	type TestParams struct {
		Query string `json:"query" jsonschema_description:"The search query"`
	}

	reflector := jsonschema.Reflector{}
	paramsSchema := reflector.Reflect(TestParams{})

	toolDef := types.ToolDefinition{
		Name:        "test_tool",
		Description: "A test tool",
		Parameters:  paramsSchema,
	}

	result := toolDefToSchema(toolDef)

	// Should not have $schema or $id
	if _, ok := result["$schema"]; ok {
		t.Error("expected $schema to be removed")
	}
	if _, ok := result["$id"]; ok {
		t.Error("expected $id to be removed")
	}

	// Should have type: object
	if result["type"] != "object" {
		t.Errorf("expected type=object, got %v", result["type"])
	}

	// Should have properties
	props, ok := result["properties"].(map[string]any)
	if !ok {
		t.Fatalf("expected properties to be map, got %T", result["properties"])
	}

	if _, ok := props["query"]; !ok {
		t.Error("expected query property")
	}
}

func TestOpenAIProvider_IncludeField_WithZenBaseURL(t *testing.T) {
	provider := NewOpenAIProviderWithClient("sk-test", &testHTTPClient{})

	model := types.Model{
		ID:       "gpt-5.5",
		Name:     "GPT 5.5",
		Provider: "opencode-zen",
		API:      "openai-responses",
		BaseURL:  "https://opencode.ai/zen/v1",
	}

	body, err := provider.buildStreamRequest(model, nil, nil, types.StreamOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var req map[string]any
	if err := json.Unmarshal(body, &req); err != nil {
		t.Fatalf("failed to unmarshal body: %v", err)
	}

	// Zen should NOT have session_usage in include
	if include, ok := req["include"].([]any); ok {
		for _, item := range include {
			if item == "session_usage" {
				t.Error("expected session_usage to be excluded for Zen")
			}
		}
	}
}

func TestOpenAIProvider_IncludeField_WithOpenAIBaseURL(t *testing.T) {
	provider := NewOpenAIProviderWithClient("sk-test", &testHTTPClient{})

	model := types.Model{
		ID:       "gpt-5.5",
		Name:     "GPT 5.5",
		Provider: "openai",
		API:      "openai-responses",
		BaseURL:  "https://api.openai.com/v1",
	}

	body, err := provider.buildStreamRequest(model, nil, nil, types.StreamOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var req map[string]any
	if err := json.Unmarshal(body, &req); err != nil {
		t.Fatalf("failed to unmarshal body: %v", err)
	}

	// session_usage is not a valid value for OpenAI Responses API include field
	if include, ok := req["include"].([]any); ok {
		for _, item := range include {
			if item == "session_usage" {
				t.Error("session_usage should never be included — not supported by OpenAI Responses API")
			}
		}
	}
}

func TestOpenAIProvider_IncludeField_WithEmptyBaseURL(t *testing.T) {
	provider := NewOpenAIProviderWithClient("sk-test", &testHTTPClient{})

	model := types.Model{
		ID:       "gpt-5.5",
		Name:     "GPT 5.5",
		Provider: "openai",
		API:      "openai-responses",
	}

	body, err := provider.buildStreamRequest(model, nil, nil, types.StreamOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var req map[string]any
	if err := json.Unmarshal(body, &req); err != nil {
		t.Fatalf("failed to unmarshal body: %v", err)
	}

	// session_usage is not a valid value for OpenAI Responses API include field
	if include, ok := req["include"].([]any); ok {
		for _, item := range include {
			if item == "session_usage" {
				t.Error("session_usage should never be included — not supported by OpenAI Responses API")
			}
		}
	}
}

func TestOpenAIProvider_ToolSchemaSanitization(t *testing.T) {
	provider := NewOpenAIProviderWithClient("sk-test", &testHTTPClient{})

	type TestParams struct {
		Query string `json:"query"`
	}
	reflector := jsonschema.Reflector{}
	paramsSchema := reflector.Reflect(TestParams{})

	tools := []types.ToolDefinition{
		{
			Name:        "test_tool",
			Description: "A test tool",
			Parameters:  paramsSchema,
		},
	}

	model := types.Model{
		ID:       "gpt-5.5",
		Name:     "GPT 5.5",
		Provider: "opencode-zen",
		API:      "openai-responses",
		BaseURL:  "https://opencode.ai/zen/v1",
	}

	body, err := provider.buildStreamRequest(model, nil, tools, types.StreamOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var req map[string]any
	if err := json.Unmarshal(body, &req); err != nil {
		t.Fatalf("failed to unmarshal body: %v", err)
	}

	toolsArr, ok := req["tools"].([]any)
	if !ok || len(toolsArr) == 0 {
		t.Fatal("expected tools in request")
	}

	tool := toolsArr[0].(map[string]any)
	if tool["type"] != "function" {
		t.Errorf("expected tool type=function, got %v", tool["type"])
	}

	params := tool["parameters"].(map[string]any)
	// Should not have $schema or $id
	if _, ok := params["$schema"]; ok {
		t.Error("expected $schema to be removed from tool parameters")
	}
	if _, ok := params["$id"]; ok {
		t.Error("expected $id to be removed from tool parameters")
	}
	// Should have type: object
	if params["type"] != "object" {
		t.Errorf("expected parameters type=object, got %v", params["type"])
	}
}

func TestAnthropicProvider_ToolSchemaSanitization(t *testing.T) {
	provider := NewAnthropicProviderWithClient("sk-test", &testHTTPClient{})

	type TestParams struct {
		Query string `json:"query"`
	}
	reflector := jsonschema.Reflector{}
	paramsSchema := reflector.Reflect(TestParams{})

	tools := []types.ToolDefinition{
		{
			Name:        "test_tool",
			Description: "A test tool",
			Parameters:  paramsSchema,
		},
	}

	model := types.Model{
		ID:       "claude-sonnet-4-6",
		Name:     "Claude Sonnet 4.6",
		Provider: "opencode-zen",
		API:      "anthropic-messages",
		BaseURL:  "https://opencode.ai/zen/v1",
	}

	body, err := provider.buildRequest(model, nil, tools, types.StreamOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var req map[string]any
	if err := json.Unmarshal(body, &req); err != nil {
		t.Fatalf("failed to unmarshal body: %v", err)
	}

	toolsArr, ok := req["tools"].([]any)
	if !ok || len(toolsArr) == 0 {
		t.Fatal("expected tools in request")
	}

	tool := toolsArr[0].(map[string]any)
	inputSchema := tool["input_schema"].(map[string]any)

	// Should not have $schema or $id
	if _, ok := inputSchema["$schema"]; ok {
		t.Error("expected $schema to be removed from input_schema")
	}
	if _, ok := inputSchema["$id"]; ok {
		t.Error("expected $id to be removed from input_schema")
	}
	// Should have type: object
	if inputSchema["type"] != "object" {
		t.Errorf("expected input_schema type=object, got %v", inputSchema["type"])
	}
}

func TestInlineSchemaRefs_NestedRefs(t *testing.T) {
	// Test with a real reflector-generated schema that has nested types
	type Inner struct {
		Value string `json:"value"`
	}
	type Outer struct {
		Inner Inner `json:"inner"`
	}
	type Params struct {
		Outer Outer `json:"outer"`
	}

	reflector := jsonschema.Reflector{}
	schema := reflector.Reflect(Params{})

	result := sanitizeToolSchema(schema)

	// $defs should be removed
	if _, ok := result["$defs"]; ok {
		t.Error("expected $defs to be removed")
	}

	// Check nested structure is inlined
	props := result["properties"].(map[string]any)
	outer := props["outer"].(map[string]any)

	// Outer should have type: object
	if outer["type"] != "object" {
		t.Errorf("expected outer type=object, got %v", outer["type"])
	}

	// Inner should also be inlined
	outerProps := outer["properties"].(map[string]any)
	inner := outerProps["inner"].(map[string]any)

	if inner["type"] != "object" {
		t.Errorf("expected inner type=object, got %v", inner["type"])
	}

	// Innermost field should exist
	innerProps := inner["properties"].(map[string]any)
	if _, ok := innerProps["value"]; !ok {
		t.Error("expected value property in inner")
	}
}

func TestInlineSchemaRefs_PreservesValidFields(t *testing.T) {
	type Params struct {
		Name  string `json:"name" jsonschema:"minLength=1,maxLength=100" jsonschema_description:"The name"`
		Count int    `json:"count" jsonschema:"minimum=0,maximum=1000" jsonschema_description:"The count"`
	}

	reflector := jsonschema.Reflector{}
	schema := reflector.Reflect(Params{})

	result := sanitizeToolSchema(schema)

	props := result["properties"].(map[string]any)
	name := props["name"].(map[string]any)

	if name["type"] != "string" {
		t.Errorf("expected name type=string, got %v", name["type"])
	}
	if name["description"] != "The name" {
		t.Errorf("expected name description, got %v", name["description"])
	}

	count := props["count"].(map[string]any)
	if count["type"] != "integer" {
		t.Errorf("expected count type=integer, got %v", count["type"])
	}
}
