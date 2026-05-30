// Package sdk provides the high-level Tau Session API.
//
// Session composes all subsystems (agent, session persistence, provider,
// tools, skills) into a coherent programmatic interface. The CLI (cmd/tau)
// is a thin consumer of this SDK.
//
// Import rules: this package depends on all internal leaf packages and the
// agent/session packages. It is the top-level integration layer.
package sdk

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/adam/tau/internal/agent"
	"github.com/adam/tau/internal/config"
	"github.com/adam/tau/internal/provider"
	tausession "github.com/adam/tau/internal/session"
	"github.com/adam/tau/internal/skills"
	"github.com/adam/tau/internal/subagent"
	"github.com/adam/tau/internal/tools"
	"github.com/adam/tau/internal/types"
)

// defaultSystemPrompt is the base behavioral instructions given to the model.
// Skills and context files are appended to this prompt.
const defaultSystemPrompt = `You are Tau, a helpful AI assistant. You can execute tools like bash, file operations, and web search.

Rules:
- Be concise and direct
- Use tools when needed to accomplish tasks
- For web search, use the web_search tool
- Format responses in markdown when appropriate
- Respond in the same language as the user's input

Sub-agent usage:
- Use the subagent tool for complex, multi-step tasks that would consume significant context
- Use the subagent tool for research, code review, or self-contained implementation tasks
- When the user asks you to use a sub-agent, always use the subagent tool
- Do NOT use the subagent tool for simple tasks you can handle directly (reading one file, simple grep)
- Provide detailed, self-contained task descriptions — sub-agents have no prior conversation context`

// SessionOptions configures a new SDK session.
type SessionOptions struct {
	// Model pattern or exact ID (e.g., "gpt-4o", "anthropic/claude-sonnet-4-20250514").
	// If empty, uses the default model from config.
	Model string

	// WorkingDir is the cwd for tool execution and context file discovery.
	WorkingDir string

	// SessionPath is an explicit session file path for resuming.
	// If set with Continue=false, opens that specific session.
	SessionPath string

	// Continue resumes the most recent session for the working directory.
	Continue bool

	// Ephemeral disables session persistence.
	Ephemeral bool

	// ToolAllowlist restricts available tools to the named set.
	// Nil means all tools are available.
	ToolAllowlist []string

	// ReadOnly disables write, edit, and bash tools.
	ReadOnly bool
}

// Session is the high-level SDK interface. It composes all subsystems
// into a coherent API for programmatic use.
//
// Zero value is not usable — always create via CreateSession().
type Session struct {
	mu sync.Mutex

	// Subsystems
	ag        *agent.Agent
	sess      *tausession.Session // nil in ephemeral mode
	provReg   *provider.Registry
	prov      provider.Provider
	model     types.Model
	toolReg   *tools.Registry
	allSkills []*skills.Skill

	// State
	cwd       string
	usage     types.Usage
	ephemeral bool
	systemP   string // composed system prompt (skills progressive disclosure)
	closed    bool   // tracks whether Close/Delete has been called
	cfg       *config.Config
	cfgPath   string

	// Message tracking for persistence
	msgCount int // number of messages already persisted to session

	// Message queue for next-turn queuing (TUI-level enqueuing during streaming).
	// Uses separate mutex to avoid deadlock with s.mu during Prompt().
	queueMu       sync.Mutex
	messageQueue  []string
	overflowCount int
}

const maxMessageQueueSize = 10

// isProviderEnabled checks if a provider is enabled based on config.
// Returns true if provider is not in config (default enabled) or explicitly enabled.
// Returns false only if explicitly set to false in config.
func isProviderEnabled(cfg *config.Config, name string) bool {
	pc, exists := cfg.Providers[name]
	if !exists {
		return true
	}
	if pc.Enabled == nil {
		return true
	}
	return *pc.Enabled
}

// resolveModelResult holds the outcome of model resolution.
type resolveModelResult struct {
	model  types.Model
	prov   provider.Provider
	source string // "cli", "session", "config", "fallback", "none"
}

func canonicalModelRef(model types.Model) string {
	if model.Provider == "" || model.ID == "" {
		return ""
	}
	return model.Provider + "/" + model.ID
}

// resolveModel selects a model using deterministic priority:
// explicit pattern > config default > deterministic connected fallback.
// Only considers models whose providers are actually registered.
// When explicitCLI is true and the pattern fails, no fallback is attempted
// (the user explicitly requested this model, so silence is worse than failure).
func resolveModel(pattern string, cfgDefault string, provReg *provider.Registry, explicitCLI bool) resolveModelResult {
	// Step 1: Try the explicit/resumed pattern
	if pattern != "" {
		if model, prov, ok := resolveConnectedModel(pattern, provReg); ok {
			source := "cli"
			if !explicitCLI {
				source = "session"
			}
			return resolveModelResult{model: model, prov: prov, source: source}
		}

		model, err := provReg.ResolveModelWithFallback(pattern)
		if err == nil {
			slog.Warn("resolved model provider not registered or unavailable",
				"pattern", pattern, "provider", model.Provider)
		} else {
			slog.Warn("failed to resolve model pattern", "pattern", pattern, "error", err)
		}
		// Explicit CLI request failed — do not silently fall back
		if explicitCLI {
			slog.Info("explicit CLI model unavailable, no fallback — use /connect or /model")
			return resolveModelResult{}
		}
	}

	// Step 2: Try config default
	if cfgDefault != "" {
		if model, prov, ok := resolveConnectedModel(cfgDefault, provReg); ok {
			slog.Info("using config default model", "model", model.ID, "provider", model.Provider)
			return resolveModelResult{model: model, prov: prov, source: "config"}
		}

		model, err := provReg.ResolveModelWithFallback(cfgDefault)
		if err == nil {
			slog.Warn("config default model provider not registered",
				"default_model", cfgDefault, "provider", model.Provider)
		} else {
			slog.Warn("failed to resolve config default model", "default_model", cfgDefault, "error", err)
		}
	}

	// Step 3: Deterministic fallback to connected providers only
	connected := provReg.ListProviders()
	connectedSet := make(map[string]bool, len(connected))
	for _, p := range connected {
		connectedSet[p] = true
	}

	allModels := provReg.Models().ListAll()
	var candidates []types.Model
	for _, m := range allModels {
		if connectedSet[m.Provider] {
			candidates = append(candidates, m)
		}
	}

	if len(candidates) > 0 {
		// Sort deterministically: by provider, then by model ID
		sort.Slice(candidates, func(i, j int) bool {
			if candidates[i].Provider != candidates[j].Provider {
				return candidates[i].Provider < candidates[j].Provider
			}
			return candidates[i].ID < candidates[j].ID
		})

		// Prefer ollama (local, no auth) but deterministically
		for _, m := range candidates {
			if m.Provider == "ollama" {
				if prov, ok := provReg.Get(m.Provider); ok {
					slog.Info("auto-selected fallback model", "model", m.ID, "provider", m.Provider)
					return resolveModelResult{model: m, prov: prov, source: "fallback"}
				}
			}
		}

		// Use first connected model deterministically
		first := candidates[0]
		if prov, ok := provReg.Get(first.Provider); ok {
			slog.Info("auto-selected fallback model", "model", first.ID, "provider", first.Provider)
			return resolveModelResult{model: first, prov: prov, source: "fallback"}
		}
	}

	slog.Info("no model available, session created without model — use /connect to add a provider")
	return resolveModelResult{}
}

func resolveConnectedModel(pattern string, provReg *provider.Registry) (types.Model, provider.Provider, bool) {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		return types.Model{}, nil, false
	}

	connected := provReg.ListProviders()
	connectedSet := make(map[string]bool, len(connected))
	for _, name := range connected {
		connectedSet[name] = true
	}

	var models []types.Model
	for _, m := range provReg.Models().ListAll() {
		if connectedSet[m.Provider] {
			models = append(models, m)
		}
	}
	if len(models) == 0 {
		return types.Model{}, nil, false
	}

	if m, ok := findExactConnectedProviderModel(pattern, models); ok {
		prov, _ := provReg.Get(m.Provider)
		return m, prov, true
	}
	if m, ok := findExactConnectedBareID(pattern, models); ok {
		prov, _ := provReg.Get(m.Provider)
		return m, prov, true
	}
	if m, ok := findBestConnectedPartial(pattern, models); ok {
		prov, _ := provReg.Get(m.Provider)
		return m, prov, true
	}

	return types.Model{}, nil, false
}

func findExactConnectedProviderModel(ref string, models []types.Model) (types.Model, bool) {
	slashIdx := strings.Index(ref, "/")
	if slashIdx == -1 {
		return types.Model{}, false
	}
	providerName := strings.ToLower(strings.TrimSpace(ref[:slashIdx]))
	modelID := strings.TrimSpace(ref[slashIdx+1:])
	for _, m := range models {
		if strings.ToLower(m.Provider) == providerName && strings.EqualFold(m.ID, modelID) {
			return m, true
		}
	}
	return types.Model{}, false
}

func findExactConnectedBareID(id string, models []types.Model) (types.Model, bool) {
	var matches []types.Model
	for _, m := range models {
		if strings.EqualFold(m.ID, id) {
			matches = append(matches, m)
		}
	}
	if len(matches) != 1 {
		return types.Model{}, false
	}
	return matches[0], true
}

func findBestConnectedPartial(pattern string, models []types.Model) (types.Model, bool) {
	lower := strings.ToLower(pattern)
	var matches []types.Model
	for _, m := range models {
		if strings.Contains(strings.ToLower(m.ID), lower) || strings.Contains(strings.ToLower(m.Name), lower) {
			matches = append(matches, m)
		}
	}
	if len(matches) == 0 {
		return types.Model{}, false
	}
	if len(matches) == 1 {
		return matches[0], true
	}

	var aliases []types.Model
	var dated []types.Model
	for _, m := range matches {
		if connectedModelIDIsAlias(m.ID) {
			aliases = append(aliases, m)
		} else {
			dated = append(dated, m)
		}
	}
	if len(aliases) > 0 {
		sort.Slice(aliases, func(i, j int) bool {
			if aliases[i].Provider != aliases[j].Provider {
				return aliases[i].Provider < aliases[j].Provider
			}
			return aliases[i].ID > aliases[j].ID
		})
		return aliases[0], true
	}
	sort.Slice(dated, func(i, j int) bool {
		if dated[i].Provider != dated[j].Provider {
			return dated[i].Provider < dated[j].Provider
		}
		return dated[i].ID > dated[j].ID
	})
	return dated[0], true
}

func connectedModelIDIsAlias(id string) bool {
	if strings.HasSuffix(id, "-latest") || len(id) < 9 {
		return true
	}
	suffix := id[len(id)-9:]
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

// CreateSession creates a new SDK session, initializing all subsystems.
//
// Lifecycle:
//   - If opts.SessionPath is set, resumes that session file.
//   - If opts.Continue is true, resumes the most recent session for opts.WorkingDir.
//   - Otherwise, creates a new session (or ephemeral if opts.Ephemeral).
//
// The session file is closed when Close() is called.
func CreateSession(ctx context.Context, opts SessionOptions) (*Session, error) {
	if opts.WorkingDir == "" {
		opts.WorkingDir = "."
	}

	// 1. Load config
	cfg, err := config.LoadConfig("")
	if err != nil {
		slog.Warn("config load failed, using defaults", "error", err)
		defaultCfg := config.DefaultConfig()
		cfg = &defaultCfg
	}

	// 2. Create provider registry and register providers
	provReg := provider.NewRegistry()

	// Load full model catalog from models.dev (falls back to built-in minimal models)
	provider.LoadFromCatalog(ctx, provReg.Models(), provider.CachePath())

	// Check for mock provider URL (for E2E testing)
	var mockURL string
	for _, envKey := range []string{"TAU_MOCK_URL", "PRAXIS_MOCK_URL"} {
		if v := os.Getenv(envKey); v != "" {
			mockURL = v
			break
		}
	}
	if mockURL != "" {
		slog.Info("using mock provider", "url", mockURL)
		mockProv := provider.NewOpenAICompatProvider("", provider.OpenAICompatConfig{
			BaseURL:      mockURL,
			ProviderName: "mock",
		})
		provReg.Register(mockProv)
		provReg.Models().Register(types.Model{
			ID:       "mock-model",
			Name:     "mock-model",
			Provider: "mock",
			API:      "openai-completions",
			BaseURL:  mockURL,
		})
		provReg.SetDefaultModel("mock-model")
	} else {
		// Try to register each provider (skip if no auth available or disabled in config)
		registerOpenAI(provReg, cfg)
		registerOpenAIOAuth(provReg, cfg)
		registerAnthropic(provReg, cfg)
		registerGoogle(provReg, cfg)
		registerOllama(provReg, cfg)
		registerOpenCodeZen(provReg, cfg)
		registerOpenCodeGo(provReg, cfg)
		registerOpenRouter(provReg, cfg)
	}

	// Set default model from config
	if cfg.DefaultModel != "" && mockURL == "" {
		provReg.SetDefaultModel(cfg.DefaultModel)
	}

	// 3. Create or resume session (needed before model resolution for resume priority)
	var sess *tausession.Session
	var msgCount int
	// recentSessionModel holds the model from the most recent session file
	// (captured before creating a new session, used as fallback)
	var recentSessionModel, recentSessionProvider string

	if opts.Ephemeral {
		slog.Info("session created in ephemeral mode")
	} else if opts.SessionPath != "" {
		// Resume specific session
		sess, err = tausession.OpenSession(opts.SessionPath)
		if err != nil {
			return nil, fmt.Errorf("open session %q: %w", opts.SessionPath, err)
		}
		msgCount = len(sess.Messages())
	} else if opts.Continue {
		// Resume most recent session
		sess, msgCount, err = resumeMostRecent(opts.WorkingDir)
		if err != nil {
			return nil, fmt.Errorf("resume session: %w", err)
		}
	} else {
		// Create new session — but first check the most recent session file for its model
		sessDir, err := config.SessionsDir(opts.WorkingDir)
		if err != nil {
			return nil, fmt.Errorf("get sessions dir: %w", err)
		}

		// Check most recent existing session file before creating new one
		if latest, _ := config.LatestSessionFile(sessDir); latest != "" {
			if prevSess, err := tausession.OpenSession(latest); err == nil {
				recentSessionModel = prevSess.CurrentModel()
				recentSessionProvider = prevSess.CurrentProvider()
				prevSess.Close()
				if recentSessionModel != "" {
					slog.Info("found model from most recent session", "model", recentSessionModel, "provider", recentSessionProvider)
				}
			}
		}

		sess, err = tausession.CreateSession(sessDir, opts.WorkingDir, "", "")
		if err != nil {
			return nil, fmt.Errorf("create session: %w", err)
		}
		msgCount = 0
	}

	// 4. Resolve model
	// Priority: explicit CLI model > resumed session model > most recent session model > config default > auto fallback
	var model types.Model
	var prov provider.Provider
	var selectedResult resolveModelResult
	var explicitModel bool

	if mockURL != "" {
		// Use mock model directly — skip normal resolution
		model = types.Model{
			ID:       "mock-model",
			Name:     "mock-model",
			Provider: "mock",
			API:      "openai-completions",
			BaseURL:  mockURL,
		}
		prov, _ = provReg.Get("mock")
	} else {
		modelPattern := opts.Model
		explicitModel = modelPattern != ""

		// If no explicit model, try resumed session model
		if !explicitModel && sess != nil {
			sessionModel := sess.CurrentModel()
			sessionProvider := sess.CurrentProvider()
			if sessionModel != "" {
				if sessionProvider != "" {
					modelPattern = sessionProvider + "/" + sessionModel
				} else {
					modelPattern = sessionModel
				}
				slog.Info("using model from resumed session", "model", sessionModel, "provider", sessionProvider)
			}
		}

		// If still no model and we created a new session, try the most recent session file's model
		if !explicitModel && modelPattern == "" && opts.Continue == false && opts.SessionPath == "" && recentSessionModel != "" {
			if recentSessionProvider != "" {
				modelPattern = recentSessionProvider + "/" + recentSessionModel
			} else {
				modelPattern = recentSessionModel
			}
			slog.Info("using model from most recent session file", "model", recentSessionModel, "provider", recentSessionProvider)
		}

		selectedResult = resolveModel(modelPattern, cfg.DefaultModel, provReg, explicitModel)
		model = selectedResult.model
		prov = selectedResult.prov
	}

	// 5. Discover skills and compose system prompt
	discovered := skills.DiscoverSkills(opts.WorkingDir)
	skillsXML := skills.FormatForPrompt(discovered)
	systemPrompt := buildSystemPromptWithSkills(defaultSystemPrompt, skillsXML, len(discovered) > 0)

	// 5. Create tool registry with options
	var toolReg *tools.Registry
	toolOpts := []tools.RegistryOption{}
	if len(opts.ToolAllowlist) > 0 {
		toolOpts = append(toolOpts, tools.WithAllowlist(opts.ToolAllowlist))
	}
	if opts.ReadOnly {
		toolOpts = append(toolOpts, tools.WithReadOnly(true))
	}
	toolReg = tools.NewRegistry(toolOpts...)
	registerBuiltinTools(toolReg, opts.WorkingDir, cfg, prov, model, provReg)

	// 6. Create agent (only if provider is available)
	var ag *agent.Agent
	if prov != nil {
		ag = newAgent(systemPrompt, opts.WorkingDir, prov, model, toolReg)
	}

	s := &Session{
		ag:        ag,
		sess:      sess,
		provReg:   provReg,
		prov:      prov,
		model:     model,
		toolReg:   toolReg,
		allSkills: discovered,
		cwd:       opts.WorkingDir,
		ephemeral: opts.Ephemeral,
		systemP:   systemPrompt,
		msgCount:  msgCount,
		cfg:       cfg,
		cfgPath:   config.ConfigPath(""),
	}

	if model.ID != "" && !explicitModel {
		s.persistResolvedModel(selectedResult)
	}

	// If resuming, restore usage, thinking level, and messages from session
	if sess != nil {
		s.usage = sess.Usage()

		// Restore thinking level for the current model
		if ag != nil && model.ID != "" {
			level := sess.GetThinkingLevelForModel(model.ID)
			ag.SetThinkingLevel(level)
			slog.Debug("sdk: restored thinking level on session resume", "model", model.ID, "level", level)
		}

		// Load session messages into agent so they're available for context
		if ag != nil {
			sessionMsgs := sess.Messages()
			if len(sessionMsgs) > 0 {
				ag.SetMessages(sessionMsgs)
				slog.Debug("sdk: loaded session messages into agent", "count", len(sessionMsgs))
			}
		}
	}

	return s, nil
}

// Prompt sends a user message and runs the agent loop until completion.
// Blocks until the agent reaches DONE or context is cancelled.
//
// The message and all assistant responses are persisted to the session file
// (unless ephemeral mode is active).
func (s *Session) Prompt(ctx context.Context, message string) error {
	slog.Debug("sdk: Prompt entering", "message_len", len(message))
	s.mu.Lock()
	slog.Debug("sdk: Prompt lock acquired")
	defer s.mu.Unlock()

	if s.ag == nil || s.model.ID == "" {
		return fmt.Errorf("no model selected — use /connect to add a provider, then /model to choose a model")
	}

	slog.Debug("sdk: Prompt calling ag.Prompt", "model", s.model.ID)
	if err := s.ag.Prompt(ctx, message); err != nil {
		slog.Debug("sdk: Prompt ag.Prompt returned error", "error", err)
		return fmt.Errorf("agent prompt: %w", err)
	}
	slog.Debug("sdk: Prompt ag.Prompt returned success, calling persistNewMessages")

	return s.persistNewMessages()
}

// Continue runs the agent loop without adding a new user message.
// Used for follow-up tasks after the agent has reached DONE.
func (s *Session) Continue(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.ag == nil || s.model.ID == "" {
		return fmt.Errorf("no model selected — use /connect to add a provider, then /model to choose a model")
	}

	if err := s.ag.Continue(ctx); err != nil {
		return fmt.Errorf("agent continue: %w", err)
	}

	return s.persistNewMessages()
}

// Steer delivers a message to the running agent via the steering queue.
// The message will be processed after the current tool call batch completes.
// Non-blocking — safe to call from any goroutine.
func (s *Session) Steer(message string) error {
	if s.ag == nil {
		return fmt.Errorf("no agent available")
	}
	return s.ag.Steer(message)
}

// Subscribe registers a listener function that will be called for every
// agent event. Returns an unsubscribe function.
//
// Listeners receive canonical typed tool payloads. Use AgentEvent.LegacyData()
// inside the listener if you need the pre-064 map[string]any tool payload shape.
//
// Listeners are called synchronously on the emitting goroutine.
// Long-running listeners will block the agent loop.
func (s *Session) Subscribe(listener func(types.AgentEvent)) func() {
	if s.ag == nil {
		return func() {}
	}
	return s.ag.Subscribe(listener)
}

// Compact triggers context compaction: detects overflow, calls the provider
// for LLM summarization, and writes a compaction entry to the session.
//
// Returns nil if compaction was not needed (context within limits).
// Returns an error if compaction was needed but failed.
func (s *Session) Compact(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.ephemeral || s.sess == nil || s.ag == nil {
		return nil // nothing to compact
	}

	messages := s.ag.Messages()
	model := s.model

	plan := tausession.PlanCompaction(messages, model.ContextWindow)
	if !plan.ShouldCompact {
		return nil
	}

	// Build the messages to summarize (up to cut point)
	toSummarize := messages[:plan.CutIndex]
	if len(toSummarize) == 0 {
		return nil
	}

	// Call provider for summarization
	summary, err := s.summarizeForCompaction(ctx, toSummarize)
	if err != nil {
		return fmt.Errorf("compaction summarization: %w", err)
	}

	// Write compaction entry to session
	compactionData := tausession.CompactionData{
		FirstKeptEntryID: plan.FirstKeptID,
		TokensBefore:     plan.TokensTotal,
		Summary:          summary,
	}
	if err := s.sess.Append(types.EntryCompaction, compactionData); err != nil {
		return fmt.Errorf("write compaction entry: %w", err)
	}

	slog.Info("session compacted",
		"tokens_before", plan.TokensTotal,
		"cut_index", plan.CutIndex,
	)

	return nil
}

// Usage returns the cumulative token usage and cost for this session.
func (s *Session) Usage() types.Usage {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.refreshUsage()
	return s.usage
}

// Model returns the currently active model.
func (s *Session) Model() types.Model {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.model
}

// SetModel changes the active model. The pattern follows the same resolution
// rules as CreateSession: exact ID → provider/ID → smart disambiguation.
func (s *Session) SetModel(pattern string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	model, prov, ok := resolveConnectedModel(pattern, s.provReg)
	if !ok {
		resolved, err := s.provReg.ResolveModelWithFallback(pattern)
		if err != nil {
			return fmt.Errorf("resolve model: %w", err)
		}
		return fmt.Errorf("provider %q is not connected; use /connect or choose another model", resolved.Provider)
	}

	slog.Debug("sdk: SetModel", "pattern", pattern, "resolved_model", model.ID, "provider", model.Provider, "reasoning", model.Reasoning)

	s.model = model
	s.prov = prov

	// Persist model change to session (provider + model ID)
	if !s.ephemeral && s.sess != nil {
		if err := s.sess.SetModel(model.ID, model.Provider); err != nil {
			return fmt.Errorf("persist model change: %w", err)
		}
	}

	// Reload config to avoid clobbering /connect or /disconnect changes made after SDK startup
	s.persistDefaultModel(model)

	// Switch provider and model on the existing agent (preserves messages).
	// This matches PI's architecture where model is a mutable state property,
	// not an agent recreation — conversation history is fully preserved.
	{
		if s.ag == nil {
			// Agent was nil (e.g., resumed without provider) — create it now.
			ag := newAgent(s.systemP, s.cwd, prov, model, s.toolReg)
			s.ag = ag

			// Load session messages into the new agent
			if s.sess != nil {
				sessionMsgs := s.sess.Messages()
				if len(sessionMsgs) > 0 {
					ag.SetMessages(sessionMsgs)
					slog.Debug("sdk: loaded session messages into recovered agent", "count", len(sessionMsgs))
				}
			}

			// Set thinking level for the new model
			if s.sess != nil {
				level := s.sess.GetThinkingLevelForModel(model.ID)
				ag.SetThinkingLevel(level)
				slog.Debug("sdk: set thinking level on agent recovery", "model", model.ID, "level", level)
			}

			slog.Info("sdk: created agent during SetModel recovery", "model", model.ID, "provider", model.Provider)
		} else {
			s.ag.SetModel(prov, model)

			// Set thinking level for the new model
			if s.sess != nil {
				level := s.sess.GetThinkingLevelForModel(model.ID)
				s.ag.SetThinkingLevel(level)
				slog.Debug("sdk: set thinking level for new model", "model", model.ID, "level", level)
			}
		}

		// Update subagent tool so future subagent spawns inherit the new parent model.
		if sat, ok := s.toolReg.Get("subagent").(*tools.SubAgentTool); ok {
			sat.UpdateParentModel(prov, model)
			slog.Debug("sdk: updated subagent tool parent model", "model", model.ID, "provider", model.Provider)
		}
	}

	return nil
}

func (s *Session) persistResolvedModel(result resolveModelResult) {
	if result.model.ID == "" {
		return
	}

	if s.sess != nil {
		currentProvider := s.sess.CurrentProvider()
		currentModel := s.sess.CurrentModel()
		if currentProvider != result.model.Provider || currentModel != result.model.ID {
			if err := s.sess.SetModel(result.model.ID, result.model.Provider); err != nil {
				slog.Warn("failed to persist resolved model to session", "error", err)
			}
		}
	}

	canonical := canonicalModelRef(result.model)
	if canonical == "" || s.cfg == nil || s.cfg.DefaultModel == canonical {
		return
	}

	// Repair stale or non-canonical defaults after automatic resolution. Explicit
	// CLI model selection remains non-persistent; /model uses SetModel instead.
	if result.source == "session" || result.source == "config" || result.source == "fallback" {
		s.persistDefaultModel(result.model)
	}
}

func (s *Session) persistDefaultModel(model types.Model) {
	canonical := canonicalModelRef(model)
	if canonical == "" || s.cfgPath == "" {
		return
	}

	latestCfg, err := config.LoadConfig(s.cfgPath)
	if err != nil {
		slog.Warn("failed to reload config before saving default model, using cached", "error", err)
		latestCfg = s.cfg
	}
	if latestCfg == nil {
		latestCfg = &config.Config{}
	}
	latestCfg.DefaultModel = canonical
	if err := config.SaveConfig(latestCfg, s.cfgPath); err != nil {
		slog.Warn("failed to save default model to config", "error", err)
		return
	}
	s.cfg = latestCfg
}

// Rename updates the session display name.
func (s *Session) Rename(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.ephemeral || s.sess == nil {
		return nil
	}

	return s.sess.SetName(name)
}

// NewSession creates a fresh session while reusing existing infrastructure.
// Closes the current session file (not deleted), creates a new one, and
// clears the agent message history. In ephemeral mode, only clears history
// and resets counters. Returns the new session ID (empty in ephemeral mode).
func (s *Session) NewSession() (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Clear message queue
	s.queueMu.Lock()
	s.messageQueue = nil
	s.overflowCount = 0
	s.queueMu.Unlock()

	if s.ephemeral || s.sess == nil {
		if s.ag != nil {
			s.ag.ClearMessages()
		}
		s.msgCount = 0
		s.usage = types.Usage{}
		return "", nil
	}

	// Close old session file (best-effort, don't delete)
	_ = s.sess.Close()

	// Create new session file
	sessDir, err := config.SessionsDir(s.cwd)
	if err != nil {
		return "", fmt.Errorf("get sessions dir: %w", err)
	}

	newSess, err := tausession.CreateSession(sessDir, s.cwd, "", "")
	if err != nil {
		return "", fmt.Errorf("create session: %w", err)
	}

	s.sess = newSess
	if s.ag != nil {
		s.ag.ClearMessages()
	}
	s.msgCount = 0
	s.usage = types.Usage{}

	return newSess.ID(), nil
}

// ResumeSession swaps the current session file with an existing session file
// at the given path, loading its messages and state into the agent. Reuses
// all existing infrastructure (provider registry, model, tools) — no provider
// re-registration or HTTP requests are made.
func (s *Session) ResumeSession(filePath string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Clear message queue
	s.queueMu.Lock()
	s.messageQueue = nil
	s.overflowCount = 0
	s.queueMu.Unlock()

	// Close current session file (best-effort)
	if s.sess != nil {
		_ = s.sess.Close()
	}

	// Open the resumed session file
	resumedSess, err := tausession.OpenSession(filePath)
	if err != nil {
		return fmt.Errorf("open session %q: %w", filePath, err)
	}

	s.sess = resumedSess
	s.ephemeral = false

	// Restore usage
	s.usage = resumedSess.Usage()

	// Restore model/provider from resumed session, falling back to config default
	sessionModel := resumedSess.CurrentModel()
	sessionProvider := resumedSess.CurrentProvider()
	var modelPattern string
	if sessionModel != "" {
		if sessionProvider != "" {
			modelPattern = sessionProvider + "/" + sessionModel
		} else {
			modelPattern = sessionModel
		}
	}

	// Use shared resolution: resumed session model > config default > fallback
	result := resolveModel(modelPattern, s.cfg.DefaultModel, s.provReg, false)
	if result.model.ID != "" {
		s.model = result.model
		s.prov = result.prov
		s.persistResolvedModel(result)
		if s.ag != nil {
			s.ag.SetModel(result.prov, result.model)
			level := resumedSess.GetThinkingLevelForModel(result.model.ID)
			s.ag.SetThinkingLevel(level)
		}

		// Update subagent tool with new parent model
		if sat, ok := s.toolReg.Get("subagent").(*tools.SubAgentTool); ok {
			sat.UpdateParentModel(result.prov, result.model)
		}

		slog.Info("sdk: restored model on resume",
			"model", result.model.ID, "provider", result.model.Provider, "source", result.source)
	} else if s.ag != nil && s.model.ID != "" {
		// No model available — restore thinking level for current model
		level := resumedSess.GetThinkingLevelForModel(s.model.ID)
		s.ag.SetThinkingLevel(level)
		slog.Debug("sdk: restored thinking level on resume", "model", s.model.ID, "level", level)
	}

	// Load session messages into agent
	sessionMsgs := resumedSess.Messages()
	s.msgCount = len(sessionMsgs)
	if s.ag != nil && len(sessionMsgs) > 0 {
		s.ag.SetMessages(sessionMsgs)
		slog.Debug("sdk: loaded session messages on resume", "count", len(sessionMsgs))
	}

	return nil
}

// ID returns the session ID.
func (s *Session) ID() string {
	if s.sess == nil {
		return ""
	}
	return s.sess.ID()
}

// Name returns the session name.
func (s *Session) Name() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.sess == nil {
		return ""
	}
	return s.sess.Name()
}

// Cwd returns the session working directory.
func (s *Session) Cwd() string {
	return s.cwd
}

// Ephemeral returns true if the session is not persisted.
func (s *Session) Ephemeral() bool {
	return s.ephemeral
}

// Close flushes and closes the session file.
// After Close, the session cannot be used for further operations.
// Safe to call multiple times (idempotent).
func (s *Session) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil
	}
	s.closed = true

	if s.sess == nil {
		return nil
	}

	s.refreshUsage()
	if s.usage.TotalTokens > 0 {
		_ = s.sess.SaveUsage() // best-effort
	}

	return s.sess.Close()
}

// Delete removes the session file from disk.
func (s *Session) Delete() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil
	}
	s.closed = true

	if s.sess == nil {
		return nil
	}

	return s.sess.Delete()
}

// Messages returns a copy of the current conversation transcript.
func (s *Session) Messages() []types.AgentMessage {
	if s.ag != nil {
		return s.ag.Messages()
	}
	if s.sess != nil {
		return s.sess.Messages()
	}
	return nil
}

// AgentState returns the current agent loop state.
func (s *Session) AgentState() agent.AgentState {
	if s.ag == nil {
		return ""
	}
	return s.ag.State()
}

// Skills returns the list of discovered skills for this session.
func (s *Session) Skills() []*skills.Skill {
	return s.allSkills
}

// Provider returns the current provider instance.
func (s *Session) Provider() provider.Provider {
	return s.prov
}

// SetThinkingLevel sets the thinking level for the current model.
func (s *Session) SetThinkingLevel(level types.ThinkingLevel) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.ag == nil {
		return fmt.Errorf("no agent available")
	}

	s.ag.SetThinkingLevel(level)
	slog.Debug("sdk: SetThinkingLevel", "model", s.model.ID, "level", level)

	if !s.ephemeral && s.sess != nil {
		if err := s.sess.SetThinkingLevel(s.model.ID, level); err != nil {
			return fmt.Errorf("persist thinking level: %w", err)
		}
	}

	return nil
}

// ThinkingLevel returns the current thinking level.
func (s *Session) ThinkingLevel() types.ThinkingLevel {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.ag == nil {
		return types.ThinkingOff
	}
	return s.ag.ThinkingLevel()
}

// ListModels returns models from connected providers only, sorted by provider then ID.
func (s *Session) ListModels() []types.Model {
	connected := s.provReg.ListProviders()
	connectedSet := make(map[string]bool, len(connected))
	for _, p := range connected {
		connectedSet[p] = true
	}
	slog.Debug("ListModels: connected providers", "providers", connected)

	allModels := s.provReg.Models().ListAll()
	slog.Debug("ListModels: all models in registry", "count", len(allModels))
	var models []types.Model
	for _, m := range allModels {
		if connectedSet[m.Provider] {
			models = append(models, m)
		} else {
			slog.Debug("ListModels: filtered out model (provider not connected)", "model", m.ID, "provider", m.Provider)
		}
	}
	sort.Slice(models, func(i, j int) bool {
		if models[i].Provider != models[j].Provider {
			return models[i].Provider < models[j].Provider
		}
		return models[i].ID < models[j].ID
	})
	return models
}

// ListProviders returns all registered provider names.
func (s *Session) ListProviders() []string {
	return s.provReg.ListProviders()
}

// RegisterProvider registers a new provider and its models into the session at runtime.
// This is used by the /connect command to add providers without restarting.
func (s *Session) RegisterProvider(prov provider.Provider, providerName string, baseURL string, modelIDs []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Register provider
	s.provReg.Register(prov)

	// For OAuth OpenAI provider, use pre-configured Codex models
	if providerName == "openai-oauth" {
		codexModels := provider.CodexModels()
		for i := range codexModels {
			codexModels[i].Provider = providerName
			codexModels[i].BaseURL = baseURL
			s.provReg.Models().Register(codexModels[i])
			slog.Debug("model registered at runtime", "model", codexModels[i].ID, "provider", providerName)
		}

		slog.Info("provider registered at runtime",
			"provider", providerName,
			"models", len(codexModels),
		)
		return nil
	}

	// Register models
	for _, modelID := range modelIDs {
		api := "openai-completions"
		switch providerName {
		case "anthropic":
			api = "anthropic-messages"
		case "google":
			api = "google-generative-ai"
		case "ollama":
			api = "ollama-chat"
		case "openrouter":
			api = "openai-completions"
		}

		m := types.Model{
			ID:       modelID,
			Name:     modelID,
			Provider: providerName,
			API:      api,
			BaseURL:  baseURL,
		}

		// Add thinking support for known reasoning models
		if isReasoningModel(modelID, providerName) {
			m.Reasoning = true
			m.ThinkingLevelMap = defaultThinkingLevelMap(providerName, modelID)
		}

		s.provReg.Models().Register(m)
		slog.Debug("model registered at runtime", "model", modelID, "provider", providerName)
	}

	slog.Info("provider registered at runtime",
		"provider", providerName,
		"models", len(modelIDs),
	)

	return nil
}

// isReasoningModel checks if a model ID is known to support reasoning.
func isReasoningModel(modelID, providerName string) bool {
	id := strings.ToLower(modelID)
	switch providerName {
	case "openai":
		return strings.HasPrefix(id, "o1") || strings.HasPrefix(id, "o3") || strings.HasPrefix(id, "gpt-5")
	case "anthropic":
		return strings.Contains(id, "claude") && (strings.Contains(id, "sonnet-4") || strings.Contains(id, "3-7-sonnet") || strings.Contains(id, "opus-4"))
	case "google":
		return strings.Contains(id, "gemini-2.5") || strings.Contains(id, "gemini-3") || strings.Contains(id, "gemma")
	case "openrouter":
		return strings.Contains(id, "r1") || strings.Contains(id, "o1") || strings.Contains(id, "o3") || strings.Contains(id, "claude-sonnet-4") || strings.Contains(id, "claude-3-7")
	case "ollama":
		return strings.Contains(id, "gemma") || strings.Contains(id, "qwq") || strings.Contains(id, "deepseek-r1")
	}
	return false
}

// defaultThinkingLevelMap returns a default thinking level map for a provider.
func defaultThinkingLevelMap(providerName, modelID string) map[string]string {
	switch providerName {
	case "openai":
		return map[string]string{
			"off": "none", "minimal": "minimal", "low": "low",
			"medium": "medium", "high": "high", "xhigh": "xhigh",
		}
	case "anthropic":
		return map[string]string{
			"low": "low", "medium": "medium", "high": "high", "xhigh": "xhigh",
		}
	case "google":
		id := strings.ToLower(modelID)
		if strings.Contains(id, "gemini-3") || strings.Contains(id, "gemma") {
			return map[string]string{
				"minimal": "MINIMAL", "low": "LOW", "medium": "MEDIUM", "high": "HIGH",
			}
		}
		// Gemini 2.x uses token budgets
		return map[string]string{
			"minimal": "128", "low": "2048", "medium": "8192", "high": "32768",
		}
	case "openrouter":
		return map[string]string{
			"off": "none", "low": "low", "medium": "medium", "high": "high",
		}
	case "ollama":
		return map[string]string{
			"low": "low", "medium": "medium", "high": "high",
		}
	default:
		return map[string]string{
			"low": "low", "medium": "medium", "high": "high",
		}
	}
}

// DisableProvider removes a provider from the registry and hides its models.
// Credentials are preserved in auth.json. If the current active model belongs
// to the disabled provider, the session provider/model are not changed — the
// user must switch models explicitly.
func (s *Session) DisableProvider(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check provider exists
	if _, ok := s.provReg.Get(name); !ok {
		return fmt.Errorf("provider %q not registered", name)
	}

	// Remove provider from registry
	s.provReg.Unregister(name)

	// Remove provider's models from model registry
	s.provReg.Models().RemoveByProvider(name)

	// Update config.json to mark provider as disabled
	cfg, err := config.LoadConfig("")
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	if cfg.Providers == nil {
		cfg.Providers = make(map[string]config.ProviderConfig)
	}

	pc := cfg.Providers[name]
	enabled := false
	pc.Enabled = &enabled
	cfg.Providers[name] = pc

	if err := config.SaveConfig(cfg, ""); err != nil {
		return fmt.Errorf("save config: %w", err)
	}

	slog.Info("provider disabled at runtime",
		"provider", name,
	)

	return nil
}

// EnqueueMessage adds a message to the next-turn queue.
// Non-blocking — safe to call during streaming.
// Returns true if queued, false if queue was full (oldest dropped).
func (s *Session) EnqueueMessage(text string) bool {
	s.queueMu.Lock()
	defer s.queueMu.Unlock()

	if len(s.messageQueue) >= maxMessageQueueSize {
		s.messageQueue = s.messageQueue[1:]
		s.overflowCount++
		slog.Warn("message queue overflow, dropped oldest message")
	}
	s.messageQueue = append(s.messageQueue, text)
	return s.overflowCount == 0 || len(s.messageQueue) <= maxMessageQueueSize
}

// DequeueMessage removes and returns the next queued message.
// Returns empty string if queue is empty.
func (s *Session) DequeueMessage() string {
	s.queueMu.Lock()
	defer s.queueMu.Unlock()

	if len(s.messageQueue) == 0 {
		return ""
	}
	msg := s.messageQueue[0]
	s.messageQueue = s.messageQueue[1:]
	return msg
}

// PendingCount returns the number of queued messages.
func (s *Session) PendingCount() int {
	s.queueMu.Lock()
	defer s.queueMu.Unlock()
	return len(s.messageQueue)
}

// OverflowCount returns the number of messages dropped due to queue overflow.
func (s *Session) OverflowCount() int {
	s.queueMu.Lock()
	defer s.queueMu.Unlock()
	return s.overflowCount
}

// ResetOverflow clears the overflow counter (after warning has been shown).
func (s *Session) ResetOverflow() {
	s.queueMu.Lock()
	defer s.queueMu.Unlock()
	s.overflowCount = 0
}

// --- Internal helpers ---

// buildSystemPromptWithSkills composes the system prompt with explicit skill usage instructions.
// This ensures models (especially GPT variants) know to actively use available skills.
func buildSystemPromptWithSkills(basePrompt, skillsXML string, hasSkills bool) string {
	if !hasSkills {
		return basePrompt
	}
	return basePrompt + "\n\n" + skillsXML + "\n\n" + `Skills provide specialized instructions and workflows for specific tasks.
When a user request matches a skill's description, use that skill to guide your approach.
Skills are listed above with their name and description — reference them to determine the appropriate workflow.`
}

// newAgent creates a configured agent instance.
func newAgent(systemPrompt, cwd string, prov provider.Provider, model types.Model, toolReg *tools.Registry) *agent.Agent {
	return agent.New(agent.Options{
		SystemPrompt: systemPrompt,
		WorkingDir:   cwd,
		Provider:     prov,
		Model:        model,
		ToolRegistry: toolReg,
	})
}

// registerOpenAI tries to resolve auth and register the OpenAI provider.
func registerOpenAI(reg *provider.Registry, cfg *config.Config) {
	if !isProviderEnabled(cfg, "openai") {
		slog.Debug("skipping openai provider (disabled in config)")
		reg.Models().RemoveByProvider("openai")
		return
	}
	key, err := provider.ResolveKey("openai", "")
	if err != nil {
		slog.Debug("skipping openai provider (no auth)")
		return
	}
	reg.Register(provider.NewOpenAIProvider(key))
}

// registerOpenAIOAuth tries to load stored OAuth credentials and register the
// OpenAI OAuth (ChatGPT Plus/Pro subscription) provider on startup.
func registerOpenAIOAuth(reg *provider.Registry, cfg *config.Config) {
	if !isProviderEnabled(cfg, "openai-oauth") {
		slog.Debug("skipping openai-oauth provider (disabled in config)")
		return
	}

	authPath := config.AuthPath("")
	store, err := config.LoadAuth(authPath)
	if err != nil {
		slog.Debug("skipping openai-oauth provider (cannot load auth)", "error", err)
		return
	}

	authVal, exists := store["openai-oauth"]
	if !exists || !authVal.IsOAuth() {
		slog.Debug("skipping openai-oauth provider (no OAuth credentials stored)")
		return
	}

	creds := provider.OAuthCredentials{
		AccessToken:  authVal.Access,
		RefreshToken: authVal.Refresh,
		Expires:      authVal.Expires,
		AccountID:    authVal.AccountID,
	}

	persist := func(creds provider.OAuthCredentials) error {
		store, err := config.LoadAuth(authPath)
		if err != nil {
			return fmt.Errorf("load auth for persistence: %w", err)
		}
		store["openai-oauth"] = config.AuthValue{
			Type:      "oauth",
			Access:    creds.AccessToken,
			Refresh:   creds.RefreshToken,
			Expires:   creds.Expires,
			AccountID: creds.AccountID,
		}
		return config.SaveAuth(store, authPath)
	}

	prov := provider.NewOpenAIOAuthProviderWithPersist(creds, persist)
	reg.Register(prov)

	codexModels := provider.CodexModels()
	for i := range codexModels {
		codexModels[i].Provider = "openai-oauth"
		reg.Models().Register(codexModels[i])
	}

	slog.Info("openai-oauth provider registered from stored credentials",
		"models", len(codexModels),
		"account_id", authVal.AccountID,
	)
}

// registerAnthropic tries to resolve auth and register the Anthropic provider.
func registerAnthropic(reg *provider.Registry, cfg *config.Config) {
	if !isProviderEnabled(cfg, "anthropic") {
		slog.Debug("skipping anthropic provider (disabled in config)")
		reg.Models().RemoveByProvider("anthropic")
		return
	}
	key, err := provider.ResolveKey("anthropic", "")
	if err != nil {
		slog.Debug("skipping anthropic provider (no auth)")
		return
	}
	reg.Register(provider.NewAnthropicProvider(key))
}

// registerGoogle tries to resolve auth and register the Google provider.
func registerGoogle(reg *provider.Registry, cfg *config.Config) {
	if !isProviderEnabled(cfg, "google") {
		slog.Debug("skipping google provider (disabled in config)")
		reg.Models().RemoveByProvider("google")
		return
	}
	key, err := provider.ResolveKey("google", "")
	if err != nil {
		slog.Debug("skipping google provider (no auth)")
		return
	}
	reg.Register(provider.NewGoogleProvider(key))
}

// ollamaTagsResponse is the JSON response from Ollama's /api/tags endpoint.
type ollamaTagsResponse struct {
	Models []ollamaModelEntry `json:"models"`
}

type ollamaModelEntry struct {
	Name string `json:"name"`
}

// registerOllama tries to connect to a local Ollama instance, discovers
// available models via /api/tags, and registers each using the native
// Ollama provider (/api/chat) which properly separates thinking from response.
func registerOllama(reg *provider.Registry, cfg *config.Config) {
	if !isProviderEnabled(cfg, "ollama") {
		slog.Debug("skipping ollama provider (disabled in config)")
		reg.Models().RemoveByProvider("ollama")
		return
	}
	const ollamaBase = "http://localhost:11434"

	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(ollamaBase + "/api/tags")
	if err != nil {
		slog.Debug("ollama not available (local dev only)", "error", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		slog.Debug("ollama /api/tags returned non-200", "status", resp.StatusCode)
		return
	}

	var tags ollamaTagsResponse
	if err := json.NewDecoder(resp.Body).Decode(&tags); err != nil {
		slog.Debug("failed to decode ollama /api/tags response", "error", err)
		return
	}

	if len(tags.Models) == 0 {
		slog.Debug("ollama running but no models pulled")
		return
	}

	// Register a native Ollama provider (/api/chat) — this properly separates
	// thinking from response content for models like gemma4:26b.
	ollamaProv := provider.NewOllamaProvider(ollamaBase)
	reg.Register(ollamaProv)

	// Register each discovered model
	for _, entry := range tags.Models {
		modelName := entry.Name
		m := types.Model{
			ID:       modelName,
			Name:     modelName,
			Provider: "ollama",
			API:      "ollama-chat",
			BaseURL:  ollamaBase,
		}
		// Ollama models with known reasoning capability
		if strings.Contains(strings.ToLower(modelName), "gemma") ||
			strings.Contains(strings.ToLower(modelName), "qwq") ||
			strings.Contains(strings.ToLower(modelName), "deepseek-r1") ||
			strings.Contains(strings.ToLower(modelName), "deepseek-r1-distill") {
			m.Reasoning = true
			m.ThinkingLevelMap = map[string]string{
				"low":    "low",
				"medium": "medium",
				"high":   "high",
			}
		}
		reg.Models().Register(m)
	}

	slog.Info("ollama auto-discovered models",
		"count", len(tags.Models),
	)
}

// openAIModelsResponse is the JSON response from OpenAI-compatible /v1/models endpoint.
type openAIModelsResponse struct {
	Data []openAIModelEntry `json:"data"`
}

type openAIModelEntry struct {
	ID            string `json:"id"`
	Object        string `json:"object"`
	Created       int64  `json:"created"`
	OwnedBy       string `json:"owned_by"`
	ContextLength *int   `json:"context_length,omitempty"`
	MaxTokens     *int   `json:"max_tokens,omitempty"`
}

// discoverOpenAICompatModels fetches models from an OpenAI-compatible /v1/models
// endpoint and registers them into the model registry. The apiKey may be empty
// for providers that don't require authentication for model listing.
func discoverOpenAICompatModels(baseURL, apiKey, providerName string, reg *provider.Registry) int {
	modelsURL := strings.TrimRight(baseURL, "/") + "/models"

	req, err := http.NewRequest(http.MethodGet, modelsURL, nil)
	if err != nil {
		slog.Debug("failed to create model discovery request", "provider", providerName, "error", err)
		return 0
	}

	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		slog.Debug("model discovery request failed", "provider", providerName, "url", modelsURL, "error", err)
		return 0
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		slog.Debug("model discovery returned non-200", "provider", providerName, "status", resp.StatusCode)
		return 0
	}

	var modelsResp openAIModelsResponse
	if err := json.NewDecoder(resp.Body).Decode(&modelsResp); err != nil {
		slog.Debug("failed to decode model discovery response", "provider", providerName, "error", err)
		return 0
	}

	count := 0
	for _, entry := range modelsResp.Data {
		if entry.ID == "" {
			continue
		}

		model := types.Model{
			ID:       entry.ID,
			Name:     entry.ID,
			Provider: providerName,
			API:      "openai-completions",
			BaseURL:  baseURL,
		}

		if entry.ContextLength != nil {
			model.ContextWindow = *entry.ContextLength
		}
		if entry.MaxTokens != nil {
			model.MaxTokens = *entry.MaxTokens
		}

		reg.Models().Register(model)
		count++
	}

	if count > 0 {
		slog.Info("discovered models via OpenAI-compatible API",
			"provider", providerName,
			"count", count,
		)
	}

	return count
}

// registerOpenCodeZen tries to resolve auth and register the OpenCode Zen provider.
func registerOpenCodeZen(reg *provider.Registry, cfg *config.Config) {
	if !isProviderEnabled(cfg, "opencode-zen") {
		slog.Debug("skipping opencode-zen provider (disabled in config)")
		reg.Models().RemoveByProvider("opencode-zen")
		return
	}
	key, err := provider.ResolveKey("opencode-zen", "")
	if err != nil {
		slog.Info("skipping opencode-zen provider (no auth)", "error", err)
		return
	}

	const zenBaseURL = "https://opencode.ai/zen/v1"

	zenProv := provider.NewZenProvider(key)
	reg.Register(zenProv)

	count := provider.DiscoverZenModels(zenBaseURL, key, reg)
	slog.Info("discovered models via OpenCode Zen provider=opencode-zen", "count", count)
}

// registerOpenCodeGo tries to resolve auth and register the OpenCode Go provider.
func registerOpenCodeGo(reg *provider.Registry, cfg *config.Config) {
	if !isProviderEnabled(cfg, "opencode-go") {
		slog.Debug("skipping opencode-go provider (disabled in config)")
		reg.Models().RemoveByProvider("opencode-go")
		return
	}
	key, err := provider.ResolveKey("opencode-go", "")
	if err != nil {
		slog.Debug("skipping opencode-go provider (no auth)")
		return
	}

	const goBaseURL = "https://opencode.ai/zen/go/v1"

	goProv := provider.NewOpenAICompatProvider(key, provider.OpenAICompatConfig{
		BaseURL:      goBaseURL,
		ProviderName: "opencode-go",
	})
	reg.Register(goProv)

	discoverOpenAICompatModels(goBaseURL, key, "opencode-go", reg)
}

// registerOpenRouter tries to resolve auth and register the OpenRouter provider.
func registerOpenRouter(reg *provider.Registry, cfg *config.Config) {
	if !isProviderEnabled(cfg, "openrouter") {
		slog.Debug("skipping openrouter provider (disabled in config)")
		reg.Models().RemoveByProvider("openrouter")
		return
	}
	key, err := provider.ResolveKey("openrouter", "")
	if err != nil {
		slog.Debug("skipping openrouter provider (no auth)")
		return
	}

	openRouterProv := provider.NewOpenRouterProvider(key)
	reg.Register(openRouterProv)

	// Discover top 30 popular models from OpenRouter API
	models, err := provider.DiscoverOpenRouterModels(key)
	if err != nil {
		slog.Debug("openrouter model discovery failed, using curated fallback", "error", err)
		provider.RegisterOpenRouterModels(reg.Models())
	} else if len(models) > 0 {
		baseURL := "https://openrouter.ai/api/v1"
		for _, id := range models {
			reg.Models().Register(types.Model{
				ID:       id,
				Name:     id,
				Provider: "openrouter",
				API:      "openai-completions",
				BaseURL:  baseURL,
			})
		}
		slog.Info("registered OpenRouter models from API", "count", len(models))
	} else {
		provider.RegisterOpenRouterModels(reg.Models())
	}

	// Register user-defined models from config
	if pc, exists := cfg.Providers["openrouter"]; exists && len(pc.Models) > 0 {
		provider.RegisterOpenRouterModelsFromConfig(reg.Models(), pc.Models)
		slog.Info("registered user-defined OpenRouter models", "count", len(pc.Models))
	}

	slog.Info("registered OpenRouter provider")
}

// persistNewMessages persists any messages that haven't been saved yet.
// Must be called with s.mu held.
func (s *Session) persistNewMessages() error {
	if s.ephemeral || s.sess == nil {
		return nil
	}

	allMessages := s.ag.Messages()
	if s.msgCount > len(allMessages) {
		s.msgCount = len(allMessages)
	}
	newMessages := allMessages[s.msgCount:]

	for _, msg := range newMessages {
		if err := s.sess.Append(types.EntryMessage, tausession.MessageData{Message: msg}); err != nil {
			return fmt.Errorf("persist message: %w", err)
		}
	}
	s.msgCount = len(allMessages)

	// Update and persist usage
	s.refreshUsage()
	if s.usage.TotalTokens > 0 {
		if err := s.sess.SaveUsage(); err != nil {
			slog.Warn("failed to save usage", "error", err)
		}
	}

	return nil
}

// refreshUsage accumulates any untracked usage from the agent's last turn.
// Must be called with s.mu held.
func (s *Session) refreshUsage() {
	if s.ag == nil {
		return
	}
	lastUsage := s.ag.LastUsage()
	if lastUsage.TotalTokens > 0 {
		s.usage = addUsage(s.usage, lastUsage)
	}
}

// summarizeForCompaction calls the provider to generate a structured summary
// of the given messages for compaction.
func (s *Session) summarizeForCompaction(ctx context.Context, messages []types.AgentMessage) (string, error) {
	var sb strings.Builder
	sb.WriteString("Summarize the following conversation history for context compaction.\n")
	sb.WriteString("Focus on: goals, constraints, key decisions, files read/modified, and current state.\n")
	sb.WriteString("Format as a structured summary.\n\n")

	for _, msg := range messages {
		switch msg.Role {
		case types.RoleUser:
			for _, block := range msg.Content {
				if block.Type == types.BlockText {
					sb.WriteString("User: ")
					sb.WriteString(block.Text)
					sb.WriteString("\n\n")
				}
			}
		case types.RoleAssistant:
			for _, block := range msg.Content {
				if block.Type == types.BlockText {
					sb.WriteString("Assistant: ")
					sb.WriteString(block.Text)
					sb.WriteString("\n\n")
				}
			}
		case types.RoleToolResult:
			sb.WriteString("[Tool result]\n\n")
		}
	}

	summaryMsg := []types.AgentMessage{
		{
			ID:   "summary-user",
			Role: types.RoleUser,
			Content: []types.ContentBlock{
				{Type: types.BlockText, Text: sb.String()},
			},
		},
	}

	opts := types.StreamOptions{
		SystemPrompt: "You are a context compaction assistant. Summarize conversation history concisely.",
	}

	result, err := s.prov.Complete(ctx, s.model, summaryMsg, nil, opts)
	if err != nil {
		return "", err
	}

	var summaryText string
	for _, block := range result.Content {
		if block.Type == types.BlockText {
			summaryText += block.Text
		}
	}

	return summaryText, nil
}

// addUsage accumulates two Usage structs together.
func addUsage(a, b types.Usage) types.Usage {
	return types.Usage{
		Input:       a.Input + b.Input,
		Output:      a.Output + b.Output,
		CacheRead:   a.CacheRead + b.CacheRead,
		CacheWrite:  a.CacheWrite + b.CacheWrite,
		TotalTokens: a.TotalTokens + b.TotalTokens,
		Cost: types.CostDollars{
			Input:      a.Cost.Input + b.Cost.Input,
			Output:     a.Cost.Output + b.Cost.Output,
			CacheRead:  a.Cost.CacheRead + b.Cost.CacheRead,
			CacheWrite: a.Cost.CacheWrite + b.Cost.CacheWrite,
			Total:      a.Cost.Total + b.Cost.Total,
		},
	}
}

// registerBuiltinTools registers all built-in tools into the registry.
func registerBuiltinTools(reg *tools.Registry, cwd string, cfg *config.Config, prov provider.Provider, model types.Model, provReg *provider.Registry) {
	const defaultMaxChars = 50000
	reg.Register(tools.NewReadTool(cwd, defaultMaxChars))
	reg.Register(tools.NewWriteTool(cwd, defaultMaxChars))
	reg.Register(tools.NewEditTool(cwd, defaultMaxChars))
	reg.Register(tools.NewBashTool(cwd, defaultMaxChars, false))
	reg.Register(tools.NewGrepTool(cwd, defaultMaxChars))
	reg.Register(tools.NewFindTool(cwd, defaultMaxChars))
	reg.Register(tools.NewLsTool(cwd, defaultMaxChars))

	registerSearchTools(reg, cfg)

	if prov != nil && model.ID != "" {
		// Discover user-defined agents
		agents := subagent.AllAgents(cwd)
		slog.Debug("discovered agents", "count", len(agents), "cwd", cwd)

		reg.Register(tools.NewSubAgentTool(prov, model, provReg, reg, reg.Names(), agents, cfg.SubagentTimeout))
	}
}

// registerSearchTools registers websearch and webfetch tools if backends are available.
func registerSearchTools(reg *tools.Registry, cfg *config.Config) {
	var backends []tools.SearchBackend

	searxngURL := cfg.Search.SearXNGURL
	if searxngURL == "" {
		searxngURL = "http://localhost:8964"
	}
	searxng := tools.NewSearXNGBackend(searxngURL, 10*time.Second)
	if searxng.Available() {
		slog.Info("websearch: SearXNG reachable", "url", searxngURL)
		backends = append(backends, searxng)
	} else {
		slog.Info("websearch: SearXNG not reachable", "url", searxngURL)
	}

	tavilyKey, tavilyErr := provider.ResolveKey("tavily", "")
	if tavilyErr == nil && tavilyKey != "" {
		slog.Info("websearch: Tavily API key found")
		backends = append(backends, tools.NewTavilyBackend(tavilyKey, 10*time.Second))
	} else {
		slog.Info("websearch: Tavily API key not configured")
	}

	braveKey, braveErr := provider.ResolveKey("brave", "")
	if braveErr == nil && braveKey != "" {
		slog.Info("websearch: Brave API key found")
		backends = append(backends, tools.NewBraveBackend(braveKey, 10*time.Second))
	} else {
		slog.Info("websearch: Brave API key not configured")
	}

	if cfg.Search.Backend != "" && cfg.Search.Backend != "auto" {
		backends = reorderBackends(backends, cfg.Search.Backend)
	}

	searchTool := tools.NewWebSearchTool(backends, time.Now())
	if searchTool != nil {
		names := make([]string, len(backends))
		for i, b := range backends {
			names[i] = b.Name()
		}
		slog.Info("websearch: available backends", "backends", names)
		reg.Register(searchTool)
	} else {
		slog.Warn("websearch: NO backends available — tool disabled. Configure an API key or start SearXNG.")
	}

	slog.Info("webfetch: always available (no API key required)")
	reg.Register(tools.NewWebFetchTool())
}

// reorderBackends moves the preferred backend to the front of the list.
func reorderBackends(backends []tools.SearchBackend, preferred string) []tools.SearchBackend {
	for i, b := range backends {
		if b.Name() == preferred {
			if i == 0 {
				return backends
			}
			result := make([]tools.SearchBackend, len(backends))
			result[0] = b
			copy(result[1:], backends[:i])
			copy(result[1+i:], backends[i+1:])
			return result
		}
	}
	return backends
}

// resumeMostRecent finds and opens the most recent session for the given cwd.
func resumeMostRecent(cwd string) (*tausession.Session, int, error) {
	sessionsDir, err := config.SessionsDir(cwd)
	if err != nil {
		return nil, 0, fmt.Errorf("get sessions dir: %w", err)
	}

	latest, err := config.LatestSessionFile(sessionsDir)
	if err != nil {
		return nil, 0, fmt.Errorf("find latest session: %w", err)
	}

	if latest == "" {
		return nil, 0, fmt.Errorf("no sessions found for %s", cwd)
	}

	sess, err := tausession.OpenSession(latest)
	if err != nil {
		return nil, 0, fmt.Errorf("open session %q: %w", latest, err)
	}

	return sess, len(sess.Messages()), nil
}
