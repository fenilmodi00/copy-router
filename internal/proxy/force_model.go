package proxy

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"workweave/router/internal/observability"
	"workweave/router/internal/providers"
	"workweave/router/internal/router"
	"workweave/router/internal/router/catalog"
	"workweave/router/internal/router/sessionpin"
	"workweave/router/internal/translate"

	"github.com/google/uuid"
)

// ForceModelHeader pins the session to a specific model, mirroring the
// /force-model chat command. Needed for headless clients (eval harness, CI
// smoke runs): Claude Code eats "/force-model …" as a client-side slash
// command before it reaches the router. The header rides on every request,
// so the pin is (re)written and served on the same turn. Values that name no
// catalog model fail the request; so do excluded ones.
const ForceModelHeader = "x-weave-force-model"

// ErrForcedModelUnknown is returned when a caller forces a model name that
// resolves to no catalog entry. Failing is the point: routing on regardless
// serves a model the caller never asked for while looking like the force took.
var ErrForcedModelUnknown = errors.New("forced model is not a known model")

// ForcedModelUnknownError carries the unresolvable value so the dispatch
// classifier can quote it back.
type ForcedModelUnknownError struct {
	Model string
}

// Error implements error.
func (e *ForcedModelUnknownError) Error() string {
	return fmt.Sprintf("%q is not a known model", e.Model)
}

// Unwrap ties the typed error to ErrForcedModelUnknown for errors.Is.
func (e *ForcedModelUnknownError) Unwrap() error { return ErrForcedModelUnknown }

var forceModelAliases = map[string]string{
	// Anthropic-family aliases retarget onto the aiand-only catalog.
	"anthropic":     "z-ai/glm-5.2",
	"claude":        "z-ai/glm-5.2",
	"opus":          "z-ai/glm-5.2",
	"claude-opus":   "z-ai/glm-5.2",
	"opus-5":        "z-ai/glm-5.2",
	"opus-5.0":      "z-ai/glm-5.2",
	"opus5":         "z-ai/glm-5.2",
	"claude-5":      "z-ai/glm-5.2",
	"opus-4-8":      "moonshotai/kimi-k3",
	"opus-4.8":      "moonshotai/kimi-k3",
	"claude-4-8":    "moonshotai/kimi-k3",
	"claude-4.8":    "moonshotai/kimi-k3",
	"fable":         "moonshotai/kimi-k3",
	"fable-5":       "moonshotai/kimi-k3",
	"fable5":        "moonshotai/kimi-k3",
	"claude-fable":  "moonshotai/kimi-k3",
	"sonnet":        "moonshotai/kimi-k2.7",
	"claude-sonnet": "moonshotai/kimi-k2.7",
	"sonnet-5":      "moonshotai/kimi-k2.7",
	"sonnet-4-6":    "deepseek/deepseek-v4-pro-0813",
	"sonnet-4.6":    "deepseek/deepseek-v4-pro-0813",
	"haiku":         "deepseek/deepseek-v4-flash",
	"claude-haiku":  "deepseek/deepseek-v4-flash",
	"haiku-4-5":     "deepseek/deepseek-v4-flash",
	"haiku-4.5":     "deepseek/deepseek-v4-flash",
	// GPT / OpenAI family → gpt-oss on aiand.
	"gpt":           "openai/gpt-oss-120b",
	"openai":        "openai/gpt-oss-120b",
	"gpt-5.6":       "openai/gpt-oss-120b",
	"gpt-5-6":       "openai/gpt-oss-120b",
	"sol":           "openai/gpt-oss-120b",
	"gpt-5-6-sol":   "openai/gpt-oss-120b",
	"terra":         "openai/gpt-oss-120b",
	"gpt-5-6-terra": "openai/gpt-oss-120b",
	"luna":          "openai/gpt-oss-120b",
	"gpt-5-6-luna":  "openai/gpt-oss-120b",
	"gpt-5-5":       "openai/gpt-oss-120b",
	"gpt-5-5-pro":   "openai/gpt-oss-120b",
	"gpt-5-5-mini":  "openai/gpt-oss-120b",
	"gpt-5-5-nano":  "openai/gpt-oss-120b",
	"gpt-5-4":       "openai/gpt-oss-120b",
	"gpt-5-4-pro":   "openai/gpt-oss-120b",
	"gpt-5-4-mini":  "openai/gpt-oss-120b",
	"gpt-5-4-nano":  "openai/gpt-oss-120b",
	// Grok / xAI → kimi-k2.7.
	"grok":     "moonshotai/kimi-k2.7",
	"grok-4.5": "moonshotai/kimi-k2.7",
	"grok4.5":  "moonshotai/kimi-k2.7",
	"xai":      "moonshotai/kimi-k2.7",
	"grok-4.6": "moonshotai/kimi-k2.7",
	"grok4.6":  "moonshotai/kimi-k2.7",
	"grok-max": "moonshotai/kimi-k2.7",
	// Google / Gemini → gemma.
	"google":                "google/gemma-4-31b-it",
	"gemini":                "google/gemma-4-31b-it",
	"gemini-pro":            "google/gemma-4-31b-it",
	"gemini-flash":          "google/gemma-4-31b-it",
	"gemini-3-6-flash":      "google/gemma-4-31b-it",
	"gemini-3-5-flash-lite": "google/gemma-4-31b-it",
	"gemini-3-7-flash":      "google/gemma-4-31b-it",
	"deepseek":              "deepseek/deepseek-v4-flash",
	"deepseek-pro":          "deepseek/deepseek-v4-pro-0813",
	"deepseek-flash":        "deepseek/deepseek-v4-flash",
	"qwen":                  "qwen/qwen3.6-27b",
	"qwen-coder":            "qwen/qwen3.6-27b",
	"qwen3.7-plus":          "qwen/qwen3.6-27b",
	"qwen-max":              "qwen/qwen3.6-27b",
	"qwen3.8-max":           "qwen/qwen3.6-27b",
	"qwen3.8":               "qwen/qwen3.6-27b",
	"qwen/qwen-3.8-max":     "qwen/qwen3.6-27b",
	"qwen-3.8-max":          "qwen/qwen3.6-27b",
	"qwen-3.8":              "qwen/qwen3.6-27b",
	// Generic kimi alias stays on 2.7; k3 needs an explicit pin.
	"kimi":         "moonshotai/kimi-k2.7",
	"kimi-k3":      "moonshotai/kimi-k3",
	"kimi-k2.7":    "moonshotai/kimi-k2.7",
	"kimi-k2.6":    "moonshotai/kimi-k2.7",
	"glm":          "z-ai/glm-5.2",
	"zai":          "z-ai/glm-5.2",
	"z-ai":         "z-ai/glm-5.2",
	"glm-5.2":      "z-ai/glm-5.2",
	"glm-5.1":      "z-ai/glm-5.2",
	"glm-5":        "z-ai/glm-5.2",
	"minimax":      "deepseek/deepseek-v4-flash",
	"minimax-m3":   "deepseek/deepseek-v4-flash",
	"minimax-m2.7": "deepseek/deepseek-v4-flash",
	"mistral":      "deepseek/deepseek-v4-flash",
	"aiand":        "deepseek/deepseek-v4-flash",
}

// resolveForceModel is the legacy two-return surface. New pin-and-effort
// callers use resolveForceModelWithEffort.
func resolveForceModel(model string) (canonicalID, provider string, known bool) {
	canon, prov, kn, _ := resolveForceModelWithEffort(model)
	return canon, prov, kn
}

// resolveForceModelWithEffort is like resolveForceModel but also strips a
// `:level` suffix. `known` is true only for catalog matches; known=false +
// effort!="" lets callers surface "model not found" without losing the effort.
//
// Matching is exact: no prefix, substring, or nearest-match fallback.
// Approximate matching silently served the wrong model; an unrecognized
// name must fail loudly instead.
func resolveForceModelWithEffort(model string) (canonicalID, provider string, known bool, effort string) {
	effortLevel, stripped := stripEffortSuffix(model)
	model = stripped
	model = strings.ToLower(strings.TrimSpace(model))
	effort = effortLevel
	// Prefer an exact catalog / upstream hit on the full string before the
	// openai/ native-prefix strip — aiand catalog IDs like openai/gpt-oss-120b
	// must not be misread as OpenAI-native names.
	if m, ok := catalog.ByIDOrUpstream(model); ok && len(m.Providers) > 0 {
		return m.ID, m.Providers[0].Provider, true, effort
	}
	unknownID := model
	requiredProvider := ""
	if nativeID, ok := strings.CutPrefix(model, "openai/"); ok {
		model = nativeID
		unknownID = nativeID
		requiredProvider = providers.ProviderOpenAI
	}
	if alias, ok := forceModelAliases[model]; ok {
		model = alias
		// Aliases retarget onto the aiand catalog; the openai/ native-prefix
		// constraint only applies to unresolved OpenAI-native names.
		requiredProvider = ""
		unknownID = model
	} else if canonical, ok := bareCatalogNames[model]; ok {
		model = canonical
	}
	// Accept catalog IDs and provider UpstreamIDs (what /v1/router/models lists
	// for ai& deploys). Upstream wire names stay on the binding; we only map
	// them back to the catalog row for pinning/dispatch.
	if m, ok := catalog.ByIDOrUpstream(model); ok && len(m.Providers) > 0 && (requiredProvider == "" || m.Providers[0].Provider == requiredProvider) {
		return m.ID, m.Providers[0].Provider, true, effort
	}
	if requiredProvider != "" {
		return unknownID, requiredProvider, false, effort
	}
	switch {
	case strings.HasPrefix(model, "claude-"):
		return model, providers.ProviderAnthropic, false, effort
	case strings.HasPrefix(model, "gpt-"),
		model == "o1", model == "o3", model == "o1-pro", model == "o3-pro",
		strings.HasPrefix(model, "o1-"), strings.HasPrefix(model, "o3-"), strings.HasPrefix(model, "o4-"):
		return model, providers.ProviderOpenAI, false, effort
	case strings.HasPrefix(model, "gemini-"):
		return model, providers.ProviderGoogle, false, effort
	case strings.Contains(model, "/"):
		return model, providers.ProviderOpenRouter, false, effort
	default:
		return model, providers.ProviderAnthropic, false, effort
	}
}

// bareCatalogNames maps a slash-form model's bare tail to its canonical
// ID ("qwen3-coder" -> "qwen/qwen3-coder") for vendor-prefix-optional lookup.
// Tails that are ambiguous, match a full catalog ID, or duplicate an alias
// are excluded; TestBareCatalogNames_Unambiguous asserts the invariant.
var bareCatalogNames = func() map[string]string {
	owners := make(map[string][]string)
	for _, m := range catalog.Models {
		if _, tail, ok := strings.Cut(m.ID, "/"); ok && len(m.Providers) > 0 {
			owners[tail] = append(owners[tail], m.ID)
		}
	}
	out := make(map[string]string, len(owners))
	for tail, ids := range owners {
		if len(ids) > 1 {
			continue
		}
		if _, isFullID := catalog.ByID(tail); isFullID {
			continue
		}
		if _, aliased := forceModelAliases[tail]; aliased {
			continue
		}
		out[tail] = ids[0]
	}
	return out
}()

// stripEffortSuffix splits a `:level` suffix off model, canonicalizes it via
// CanonicalizeEffort, and returns ("", model) when no recognized suffix found.
func stripEffortSuffix(model string) (effort string, modelOut string) {
	const sep = ":"
	idx := strings.LastIndex(model, sep)
	if idx < 0 || idx == len(model)-1 {
		return "", model
	}
	tail := strings.TrimSpace(model[idx+1:])
	if !looksLikeEffortAlias(tail) {
		return "", model
	}
	return translate.CanonicalizeEffort(tail), model[:idx]
}

// looksLikeEffortAlias guards against future catalog IDs that contain `:`,
// ensuring the colon is only treated as a suffix separator for known levels.
func looksLikeEffortAlias(tail string) bool {
	switch strings.ToLower(strings.TrimSpace(tail)) {
	case "fast", "low", "medium", "med", "high", "max", "xhigh",
		"ultra", "minimal", "min":
		return true
	default:
		return false
	}
}

// setForceModelPin upserts an immutable user-forced session pin. It preserves
// the prior pin's LastServedModel so the next turn can detect a mid-session
// model switch and strip stale Anthropic thinking-block signatures. No-op if
// the pin store is unconfigured or installationID is nil.
func (s *Service) setForceModelPin(
	ctx context.Context,
	sessionKey [sessionpin.SessionKeyLen]byte,
	role string,
	installationID uuid.UUID,
	canonicalModel, provider string,
) error {
	if s.pinStore == nil || installationID == uuid.Nil {
		return nil
	}
	log := observability.FromContext(ctx)
	var lastServedModel string
	existing, found, err := s.pinStore.Get(ctx, sessionKey, role)
	if err != nil {
		log.Error("force-model: prior pin lookup failed", "err", err)
	} else if found {
		lastServedModel = existing.LastServedModel
	}
	forced := sessionpin.Pin{
		SessionKey:      sessionKey,
		Role:            role,
		InstallationID:  installationID,
		Provider:        provider,
		Model:           canonicalModel,
		Reason:          translate.ReasonUserForceModel,
		TurnCount:       1,
		PinnedUntil:     pinNeverExpires,
		LastServedModel: lastServedModel,
	}
	// context.Background(): ctx may already be canceled here (response written,
	// client disconnected); a canceled ctx would leave the prior pin stuck.
	return s.pinStore.Upsert(context.Background(), forced)
}

// applyForceModelHeader honors the x-weave-force-model request header,
// writing the same session pin the /force-model command writes. It's
// (re)written on every request carrying the header. A model that names no
// catalog entry, or one the exclusion policy forbids, fails the request —
// silently routing elsewhere would serve a model the caller never asked for.
//
// A `:level` suffix is stashed on context as router.Overrides.ForceEffort
// so pin + effort land in one header.
func (s *Service) applyForceModelHeader(
	ctx context.Context,
	r *http.Request,
	env *translate.RequestEnvelope,
	installationID uuid.UUID,
	sessionKey [sessionpin.SessionKeyLen]byte,
) (string, error) {
	raw := strings.TrimSpace(r.Header.Get(ForceModelHeader))
	if raw == "" {
		return "", nil
	}
	log := observability.FromContext(ctx)
	canonicalModel, provider, known, effortLevel := resolveForceModelWithEffort(raw)
	if effortLevel != "" {
		// Merge with any existing knobs so ForceEffort doesn't drop Alpha/QualityBias.
		merged := router.Overrides{ForceEffort: effortLevel}
		if existing := router.RoutingKnobsFromContext(r.Context()); existing != nil {
			merged.Alpha = existing.Alpha
			merged.QualityBias = existing.QualityBias
			merged.SpeedWeight = existing.SpeedWeight
			merged.OutputCostRatio = existing.OutputCostRatio
			merged.ExpectedOutputTokens = existing.ExpectedOutputTokens
			merged.PerModelVerbosity = existing.PerModelVerbosity
		}
		// Mutate *r so the caller's downstream routingKnobsForRequest
		// (which reads ctx from r.Context()) discovers the knob.
		*r = *r.WithContext(router.WithRoutingKnobs(r.Context(), &merged))
	}
	if !known {
		log.Warn("x-weave-force-model: rejected unrecognized model",
			"input_model", raw,
			"session_key_hex", fmt.Sprintf("%x", sessionKey),
		)
		return "", &ForcedModelUnknownError{Model: raw}
	}
	binding, reason := s.forcedModelBinding(ctx, canonicalModel, provider)
	if reason != "" {
		log.Warn("x-weave-force-model: rejected excluded model",
			"input_model", raw,
			"canonical_model", canonicalModel,
			"provider", provider,
			"reason", reason,
		)
		return "", &ForcedModelExcludedError{Model: canonicalModel, Reason: reason}
	}
	provider = binding
	if s.pinStore == nil {
		return canonicalModel, nil
	}
	role := roleForTier(catalog.TierFor(env.Model()))
	if err := s.setForceModelPin(ctx, sessionKey, role, installationID, canonicalModel, provider); err != nil {
		log.Error("x-weave-force-model: pin store upsert failed", "err", err)
		return canonicalModel, nil
	}
	log.Info("x-weave-force-model applied",
		"input_model", raw,
		"canonical_model", canonicalModel,
		"provider", provider,
		"effort", effortLevel,
		"session_key_hex", fmt.Sprintf("%x", sessionKey),
		"role", role,
	)
	return canonicalModel, nil
}

// handleForceModelCommand processes a /force-model or /unforce-model directive:
// writes (or expires) the session pin and returns a synthetic acknowledgment
// response without dispatching upstream. inputTokens should be the request's
// RoutingFeatures.Tokens so the token counter reflects actual turn input, not
// just the synthetic response text.
func (s *Service) handleForceModelCommand(
	ctx context.Context,
	w http.ResponseWriter,
	env *translate.RequestEnvelope,
	cmd translate.ForceModelResult,
	installationID uuid.UUID,
	sessionKey [sessionpin.SessionKeyLen]byte,
	inputTokens int,
) error {
	log := observability.FromContext(ctx)
	role := roleForTier(catalog.TierFor(env.Model()))

	// Formatted as a routing marker (✦ **Weave Router** → …\n\n) so
	// StripRoutingMarkerFromMessages strips it from later inbound requests;
	// otherwise it'd persist in history and leak router internals upstream.
	var msg string
	if cmd.Clear {
		if s.pinStore != nil && installationID != uuid.Nil {
			if err := s.expireSessionPin(ctx, installationID, sessionKey, role, "user_unforced"); err != nil {
				log.Error("/unforce-model: pin store upsert failed", "err", err)
				return err
			}
		}
		msg = "✦ **Weave Router** → force-model cleared · resuming automatic model selection\n\n"
		if env.SourceFormat() == translate.FormatOpenAI {
			msg = "Weave Router: force-model cleared; resuming automatic model selection"
		}
		// Debug not Info: fires on every command use, not a major business event.
		log.Debug("/unforce-model: session pin cleared",
			"session_key_hex", fmt.Sprintf("%x", sessionKey),
			"role", role,
		)
	} else if canonicalModel, provider, known := resolveForceModel(cmd.Model); !known {
		// Not in the catalog (e.g. truncated "/force-model gpt-") — reject
		// rather than pin something we can't honor; prior pin left untouched.
		log.Info("/force-model: rejected unknown model",
			"input_model", cmd.Model,
			"session_key_hex", fmt.Sprintf("%x", sessionKey),
			"role", role,
		)
		msg = fmt.Sprintf("✦ **Weave Router** → force-model: %q isn't a recognized model · keeping automatic routing. Use a full model ID, e.g. moonshotai/kimi-k3, deepseek/deepseek-v4-flash, or z-ai/glm-5.2.\n\n", cmd.Model)
		if env.SourceFormat() == translate.FormatOpenAI {
			msg = fmt.Sprintf("Weave Router: force-model: %q isn't a recognized model; keeping automatic routing. Use a full model ID, e.g. moonshotai/kimi-k3, deepseek/deepseek-v4-flash, or z-ai/glm-5.2.", cmd.Model)
		}
	} else if binding, reason := s.forcedModelBinding(ctx, canonicalModel, provider); reason != "" {
		// Exclusions outrank the force. Pinning anyway would look accepted and
		// then serve something else every turn; the prior pin is left untouched.
		log.Warn("/force-model: rejected excluded model",
			"input_model", cmd.Model,
			"canonical_model", canonicalModel,
			"provider", provider,
			"reason", reason,
			"session_key_hex", fmt.Sprintf("%x", sessionKey),
			"role", role,
		)
		msg = fmt.Sprintf("✦ **Weave Router** → force-model rejected: %s · keeping automatic routing. Ask an admin to allow the provider, or force a model from one that is permitted.\n\n", reason)
		if env.SourceFormat() == translate.FormatOpenAI {
			msg = fmt.Sprintf("Weave Router: force-model rejected: %s; keeping automatic routing. Ask an admin to allow the provider, or force a model from one that is permitted.", reason)
		}
	} else {
		if err := s.setForceModelPin(ctx, sessionKey, role, installationID, canonicalModel, binding); err != nil {
			log.Error("/force-model: pin store upsert failed", "err", err)
			return err
		}
		msg = fmt.Sprintf("✦ **Weave Router** → force-model applied: %s (%s) · Use /unforce-model to clear\n\n", canonicalModel, binding)
		if env.SourceFormat() == translate.FormatOpenAI {
			msg = fmt.Sprintf("Weave Router: force-model applied: %s (%s). Use /unforce-model to clear.", canonicalModel, binding)
		}
		log.Debug("/force-model: session pin set",
			"input_model", cmd.Model,
			"canonical_model", canonicalModel,
			"provider", binding,
			"session_key_hex", fmt.Sprintf("%x", sessionKey),
			"role", role,
		)
	}

	switch env.SourceFormat() {
	case translate.FormatOpenAI:
		return writeSyntheticOpenAIResponse(w, env, msg, inputTokens)
	default:
		return writeSyntheticAnthropicResponse(w, env, msg, inputTokens)
	}
}

// writeSyntheticAnthropicResponse writes a minimal Anthropic Messages API
// response without hitting an upstream, handling both streaming and
// non-streaming shapes.
func writeSyntheticAnthropicResponse(w http.ResponseWriter, env *translate.RequestEnvelope, text string, inputTokens int) error {
	msgID := fmt.Sprintf("msg_router_cmd_%x", time.Now().UnixNano())
	if env.Stream() {
		return writeSyntheticAnthropicSSE(w, msgID, text, inputTokens)
	}
	return writeSyntheticAnthropicJSON(w, msgID, text, inputTokens)
}

func writeSyntheticAnthropicJSON(w http.ResponseWriter, msgID, text string, inputTokens int) error {
	resp := map[string]any{
		"id":            msgID,
		"type":          "message",
		"role":          "assistant",
		"model":         "weave-router",
		"stop_reason":   "end_turn",
		"stop_sequence": nil,
		"content": []any{
			map[string]any{"type": "text", "text": text},
		},
		"usage": map[string]any{
			"input_tokens":  inputTokens,
			"output_tokens": len(text) / 4,
		},
	}
	body, err := json.Marshal(resp)
	if err != nil {
		return fmt.Errorf("marshal synthetic response: %w", err)
	}
	w.Header().Set("Content-Type", "application/json")
	_, writeErr := w.Write(body)
	return writeErr
}

func writeSyntheticAnthropicSSE(w http.ResponseWriter, msgID, text string, inputTokens int) error {
	w.Header().Set("Content-Type", "text/event-stream")
	flusher, _ := w.(http.Flusher)
	bw := bufio.NewWriterSize(w, 4096)

	outTokens := len(text) / 4

	events := []string{
		sseEvent("message_start", mustMarshalJSON(map[string]any{
			"type": "message_start",
			"message": map[string]any{
				"id": msgID, "type": "message", "role": "assistant",
				"content": []any{}, "model": "weave-router",
				"stop_reason": nil, "stop_sequence": nil,
				"usage": map[string]any{"input_tokens": inputTokens, "output_tokens": 0},
			},
		})),
		sseEvent("content_block_start", mustMarshalJSON(map[string]any{
			"type": "content_block_start", "index": 0,
			"content_block": map[string]any{"type": "text", "text": ""},
		})),
		sseEvent("ping", `{"type":"ping"}`),
		sseEvent("content_block_delta", mustMarshalJSON(map[string]any{
			"type": "content_block_delta", "index": 0,
			"delta": map[string]any{"type": "text_delta", "text": text},
		})),
		sseEvent("content_block_stop", `{"type":"content_block_stop","index":0}`),
		sseEvent("message_delta", mustMarshalJSON(map[string]any{
			"type":  "message_delta",
			"delta": map[string]any{"stop_reason": "end_turn", "stop_sequence": nil},
			"usage": map[string]any{"output_tokens": outTokens},
		})),
		sseEvent("message_stop", `{"type":"message_stop"}`),
	}

	for _, ev := range events {
		bw.WriteString(ev)
	}
	if err := bw.Flush(); err != nil {
		return err
	}
	if flusher != nil {
		flusher.Flush()
	}
	return nil
}

// writeSyntheticOpenAIResponse writes a minimal OpenAI Chat Completions
// response without hitting an upstream, handling both streaming and
// non-streaming shapes.
func writeSyntheticOpenAIResponse(w http.ResponseWriter, env *translate.RequestEnvelope, text string, inputTokens int) error {
	respID := fmt.Sprintf("chatcmpl_router_cmd_%x", time.Now().UnixNano())
	if env.Stream() {
		return writeSyntheticOpenAISSE(w, respID, text, inputTokens)
	}
	return writeSyntheticOpenAIJSON(w, respID, text, inputTokens)
}

func writeSyntheticOpenAIJSON(w http.ResponseWriter, respID, text string, inputTokens int) error {
	outTokens := len(text) / 4
	resp := map[string]any{
		"id":      respID,
		"object":  "chat.completion",
		"created": time.Now().Unix(),
		"model":   "weave-router",
		"choices": []any{
			map[string]any{
				"index": 0,
				"message": map[string]any{
					"role":    "assistant",
					"content": text,
				},
				"finish_reason": "stop",
			},
		},
		"usage": map[string]any{
			"prompt_tokens":     inputTokens,
			"completion_tokens": outTokens,
			"total_tokens":      inputTokens + outTokens,
		},
	}
	body, err := json.Marshal(resp)
	if err != nil {
		return fmt.Errorf("marshal synthetic openai response: %w", err)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, writeErr := w.Write(body)
	return writeErr
}

func writeSyntheticOpenAISSE(w http.ResponseWriter, respID, text string, inputTokens int) error {
	w.Header().Set("Content-Type", "text/event-stream")
	w.WriteHeader(http.StatusOK)
	flusher, _ := w.(http.Flusher)
	bw := bufio.NewWriterSize(w, 4096)
	created := time.Now().Unix()
	outTokens := len(text) / 4
	chunkStart := mustMarshalJSON(map[string]any{
		"id":      respID,
		"object":  "chat.completion.chunk",
		"created": created,
		"model":   "weave-router",
		"choices": []any{
			map[string]any{
				"index": 0,
				"delta": map[string]any{
					"role":    "assistant",
					"content": text,
				},
				"finish_reason": nil,
			},
		},
	})
	chunkStop := mustMarshalJSON(map[string]any{
		"id":      respID,
		"object":  "chat.completion.chunk",
		"created": created,
		"model":   "weave-router",
		"choices": []any{
			map[string]any{
				"index":         0,
				"delta":         map[string]any{},
				"finish_reason": "stop",
			},
		},
		"usage": map[string]any{
			"prompt_tokens":     inputTokens,
			"completion_tokens": outTokens,
			"total_tokens":      inputTokens + outTokens,
		},
	})
	events := []string{
		openAISSEData(chunkStart),
		openAISSEData(chunkStop),
		openAISSEData("[DONE]"),
	}
	for _, ev := range events {
		bw.WriteString(ev)
	}
	if err := bw.Flush(); err != nil {
		return err
	}
	if flusher != nil {
		flusher.Flush()
	}
	return nil
}

func sseEvent(eventType, data string) string {
	return "event: " + eventType + "\ndata: " + data + "\n\n"
}

func openAISSEData(data string) string {
	return "data: " + data + "\n\n"
}

func mustMarshalJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(b)
}
