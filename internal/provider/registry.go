package provider

import (
	"fmt"
	"sort"
	"strings"

	"github.com/adam/tau/internal/types"
)

// Registry manages provider instances and model resolution.
type Registry struct {
	providers    map[string]Provider
	models       *ModelRegistry
	defaultModel string
}

// NewRegistry creates a new provider registry with a model registry.
func NewRegistry() *Registry {
	return &Registry{
		providers: make(map[string]Provider),
		models:    NewModelRegistry(),
	}
}

// Register adds a provider to the registry.
func (r *Registry) Register(p Provider) {
	r.providers[p.Name()] = p
}

// Unregister removes a provider from the registry by name.
func (r *Registry) Unregister(name string) {
	delete(r.providers, name)
}

// Get returns a provider by name.
func (r *Registry) Get(name string) (Provider, bool) {
	p, ok := r.providers[name]
	return p, ok
}

// Models returns the model registry for direct access.
func (r *Registry) Models() *ModelRegistry {
	return r.models
}

// SetDefaultModel sets the default model pattern used when no model is specified.
func (r *Registry) SetDefaultModel(pattern string) {
	r.defaultModel = pattern
}

// ResolveModel finds a model by pattern. If pattern is empty, uses the default.
func (r *Registry) ResolveModel(pattern string) (types.Model, error) {
	if pattern == "" {
		pattern = r.defaultModel
	}
	if pattern == "" {
		return types.Model{}, fmt.Errorf("no model specified and no default configured")
	}
	return r.models.Find(pattern)
}

// ListProviders returns all registered provider names.
func (r *Registry) ListProviders() []string {
	names := make([]string, 0, len(r.providers))
	for name := range r.providers {
		names = append(names, name)
	}
	return names
}

// ResolveModelWithFallback resolves a model pattern to a single Model using
// PI-style smart disambiguation. Unlike ResolveModel (which errors on multiple
// substring matches), this always picks the best candidate deterministically:
//
//  1. If pattern matches "provider/modelId" exactly → return that model
//  2. If pattern matches a bare model ID exactly → return it (only if unique
//     across all providers; otherwise falls through to step 3)
//  3. Partial match on ID or Name → separate aliases from dated versions:
//     - Aliases (no date suffix like -20250514) preferred, highest sort wins
//     - Dated versions → pick the latest (highest sort)
//
// If pattern is empty, uses the default model. If no match found at all,
// returns an error listing available models.
func (r *Registry) ResolveModelWithFallback(pattern string) (types.Model, error) {
	if pattern == "" {
		pattern = r.defaultModel
	}
	if pattern == "" {
		return types.Model{}, fmt.Errorf("no model specified and no default configured")
	}

	allModels := r.models.ListAll()

	// Step 1: Try exact "provider/modelId" match
	if m, ok := r.findExactProviderModel(pattern, allModels); ok {
		return m, nil
	}

	// Step 2: Try exact bare model ID match (only if unique)
	if m, ok := r.findExactBareID(pattern, allModels); ok {
		return m, nil
	}

	// Step 3: Partial match with PI-style disambiguation
	return r.findBestPartialMatch(pattern, allModels)
}

// findExactProviderModel looks for "provider/modelId" format.
func (r *Registry) findExactProviderModel(ref string, models []types.Model) (types.Model, bool) {
	slashIdx := strings.Index(ref, "/")
	if slashIdx == -1 {
		return types.Model{}, false
	}

	prov := strings.ToLower(strings.TrimSpace(ref[:slashIdx]))
	modelID := strings.TrimSpace(ref[slashIdx+1:])

	for _, m := range models {
		if strings.ToLower(m.Provider) == prov && strings.EqualFold(m.ID, modelID) {
			return m, true
		}
	}
	return types.Model{}, false
}

// findExactBareID looks for an exact model ID match that is unique across providers.
func (r *Registry) findExactBareID(id string, models []types.Model) (types.Model, bool) {
	var matches []types.Model
	for _, m := range models {
		if strings.EqualFold(m.ID, id) {
			matches = append(matches, m)
		}
	}
	if len(matches) == 1 {
		return matches[0], true
	}
	// Multiple providers share this ID — don't guess, fall through to partial
	return types.Model{}, false
}

// isAlias checks if a model ID looks like an alias (no date suffix like -20250514).
func isAlias(id string) bool {
	if strings.HasSuffix(id, "-latest") {
		return true
	}
	// Check if ID ends with a date pattern (-YYYYMMDD)
	if len(id) < 9 {
		return true // too short to have a date suffix
	}
	suffix := id[len(id)-9:] // last 9 chars: -YYYYMMDD
	if suffix[0] != '-' {
		return true
	}
	for i := 1; i < 9; i++ {
		if suffix[i] < '0' || suffix[i] > '9' {
			return true
		}
	}
	return false
}

// findBestPartialMatch does substring matching on ID and Name, then picks the
// best candidate using PI's disambiguation logic.
func (r *Registry) findBestPartialMatch(pattern string, models []types.Model) (types.Model, error) {
	lower := strings.ToLower(pattern)
	var matches []types.Model
	for _, m := range models {
		if strings.Contains(strings.ToLower(m.ID), lower) ||
			strings.Contains(strings.ToLower(m.Name), lower) {
			matches = append(matches, m)
		}
	}

	if len(matches) == 0 {
		// Build a helpful error message
		var ids []string
		for _, m := range models {
			ids = append(ids, fmt.Sprintf("%s/%s", m.Provider, m.ID))
		}
		sort.Strings(ids)
		return types.Model{}, fmt.Errorf("no model matching %q; available: %s", pattern, strings.Join(ids, ", "))
	}

	if len(matches) == 1 {
		return matches[0], nil
	}

	// Separate aliases from dated versions
	var aliases []types.Model
	var dated []types.Model
	for _, m := range matches {
		if isAlias(m.ID) {
			aliases = append(aliases, m)
		} else {
			dated = append(dated, m)
		}
	}

	if len(aliases) > 0 {
		// Prefer alias — pick the one that sorts highest alphabetically
		sort.Slice(aliases, func(i, j int) bool {
			return aliases[i].ID > aliases[j].ID
		})
		return aliases[0], nil
	}

	// No alias — pick latest dated version (highest sort = latest date)
	sort.Slice(dated, func(i, j int) bool {
		return dated[i].ID > dated[j].ID
	})
	return dated[0], nil
}
