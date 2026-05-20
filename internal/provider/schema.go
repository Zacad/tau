package provider

import (
	"encoding/json"
	"strings"

	"github.com/adam/tau/internal/types"
	"github.com/invopop/jsonschema"
)

// schemaMetaFields are JSON Schema meta fields that most APIs reject.
var schemaMetaFields = map[string]bool{
	"$schema": true,
	"$id":     true,
	"$ref":    true,
	"$defs":   true,
}

// sanitizeToolSchema converts a *jsonschema.Schema to a map[string]any suitable
// for sending to LLM APIs. It marshals the schema to JSON, unmarshals as a map,
// strips meta fields ($schema, $id), and inlines $ref definitions from $defs.
// This is needed because jsonschema generates $ref/$defs format, but most LLM
// APIs (Anthropic, Google, OpenAI Responses) require fully inlined schemas.
func sanitizeToolSchema(schema *jsonschema.Schema) map[string]any {
	if schema == nil {
		return nil
	}

	raw, err := json.Marshal(schema)
	if err != nil {
		return nil
	}

	var schemaMap map[string]any
	if err := json.Unmarshal(raw, &schemaMap); err != nil {
		return nil
	}

	// Extract $defs for passing to recursive inlining
	var defs map[string]any
	if d, ok := schemaMap["$defs"].(map[string]any); ok {
		defs = d
	}

	return inlineSchemaRefs(schemaMap, defs)
}

// inlineSchemaRefs removes $ref/$defs meta fields and recursively inlines
// all $ref references using the provided defs map.
func inlineSchemaRefs(schema map[string]any, defs map[string]any) map[string]any {
	if schema == nil {
		return nil
	}

	result := make(map[string]any)
	for key, value := range schema {
		if schemaMetaFields[key] {
			continue
		}

		if m, ok := value.(map[string]any); ok {
			result[key] = inlineSchemaRefs(m, defs)
		} else if arr, ok := value.([]any); ok {
			sanitized := make([]any, len(arr))
			for i, item := range arr {
				if m, ok := item.(map[string]any); ok {
					sanitized[i] = inlineSchemaRefs(m, defs)
				} else {
					sanitized[i] = item
				}
			}
			result[key] = sanitized
		} else {
			result[key] = value
		}
	}

	// Inline $ref from defs
	if ref, ok := schema["$ref"].(string); ok {
		refName := strings.TrimPrefix(ref, "#/$defs/")
		if def, ok := defs[refName]; ok {
			if defMap, ok := def.(map[string]any); ok {
				// Merge definition fields into result (definition takes precedence for missing fields)
				for k, v := range defMap {
					if _, exists := result[k]; !exists {
						if m, ok := v.(map[string]any); ok {
							result[k] = inlineSchemaRefs(m, defs)
						} else if arr, ok := v.([]any); ok {
							sanitized := make([]any, len(arr))
							for i, item := range arr {
								if m, ok := item.(map[string]any); ok {
									sanitized[i] = inlineSchemaRefs(m, defs)
								} else {
									sanitized[i] = item
								}
							}
							result[k] = sanitized
						} else {
							result[k] = v
						}
					}
				}
			}
		}
	}

	// Ensure type: "object" at top level if properties exist
	if result["type"] == nil && result["properties"] != nil {
		result["type"] = "object"
	}

	// Filter required array to only include fields that exist in properties
	if result["type"] == "object" {
		if props, ok := result["properties"].(map[string]any); ok {
			if req, ok := result["required"].([]any); ok {
				filtered := make([]any, 0, len(req))
				for _, r := range req {
					if s, ok := r.(string); ok {
						if _, exists := props[s]; exists {
							filtered = append(filtered, s)
						}
					}
				}
				if len(filtered) > 0 {
					result["required"] = filtered
				} else {
					delete(result, "required")
				}
			}
		}
	} else if result["type"] != nil && result["type"] != "object" {
		delete(result, "properties")
		delete(result, "required")
	}

	return result
}

// toolDefToSchema converts a ToolDefinition's Parameters to a sanitized
// map[string]any suitable for API requests.
func toolDefToSchema(t types.ToolDefinition) map[string]any {
	return sanitizeToolSchema(t.Parameters)
}
