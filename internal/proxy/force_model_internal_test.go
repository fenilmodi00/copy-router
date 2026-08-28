package proxy

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"workweave/router/internal/providers"
	"workweave/router/internal/router"
	"workweave/router/internal/router/catalog"
	"workweave/router/internal/router/sessionpin"
	"workweave/router/internal/translate"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveForceModel(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		wantID       string
		wantProvider string
		wantKnown    bool
	}{
		// Catalog matches: provider comes from the primary binding (aiand).
		{
			name:         "catalog kimi-k3",
			input:        "moonshotai/kimi-k3",
			wantID:       "moonshotai/kimi-k3",
			wantProvider: providers.ProviderAiand,
			wantKnown:    true,
		},
		{
			name:         "catalog gemma",
			input:        "google/gemma-4-31b-it",
			wantID:       "google/gemma-4-31b-it",
			wantProvider: providers.ProviderAiand,
			wantKnown:    true,
		},
		{
			name:         "catalog qwen",
			input:        "qwen/qwen3.6-27b",
			wantID:       "qwen/qwen3.6-27b",
			wantProvider: providers.ProviderAiand,
			wantKnown:    true,
		},
		{
			name:         "catalog qwen — bare suffix match",
			input:        "qwen3.6-27b",
			wantID:       "qwen/qwen3.6-27b",
			wantProvider: providers.ProviderAiand,
			wantKnown:    true,
		},
		{
			name:         "alias gpt",
			input:        "gpt",
			wantID:       "openai/gpt-oss-120b",
			wantProvider: providers.ProviderAiand,
			wantKnown:    true,
		},
		{
			name:         "alias gpt hyphen minor version",
			input:        "gpt-5-5",
			wantID:       "openai/gpt-oss-120b",
			wantProvider: providers.ProviderAiand,
			wantKnown:    true,
		},
		{
			name:         "native openai prefix",
			input:        "openai/gpt-oss-120b",
			wantID:       "openai/gpt-oss-120b",
			wantProvider: providers.ProviderAiand,
			wantKnown:    true,
		},
		{
			name:         "native openai prefix with version alias",
			input:        "openai/gpt-5.6",
			wantID:       "openai/gpt-oss-120b",
			wantProvider: providers.ProviderAiand,
			wantKnown:    true,
		},
		{
			name:         "native openai prefix with model alias",
			input:        "openai/luna",
			wantID:       "openai/gpt-oss-120b",
			wantProvider: providers.ProviderAiand,
			wantKnown:    true,
		},
		{
			name:         "native openai prefix rejects cross-provider alias",
			input:        "openai/claude",
			wantID:       "zai-org/glm-5.2",
			wantProvider: providers.ProviderAiand,
			wantKnown:    true, // alias retargets onto aiand catalog
		},
		{
			name:         "alias claude",
			input:        "claude",
			wantID:       "zai-org/glm-5.2",
			wantProvider: providers.ProviderAiand,
			wantKnown:    true,
		},
		{
			name:         "alias opus",
			input:        "opus",
			wantID:       "zai-org/glm-5.2",
			wantProvider: providers.ProviderAiand,
			wantKnown:    true,
		},
		{
			name:         "alias opus dotted version",
			input:        "opus-4.8",
			wantID:       "zai-org/glm-5.2",
			wantProvider: providers.ProviderAiand,
			wantKnown:    true,
		},
		{
			name:         "alias claude-fable-5",
			input:        "claude-fable-5",
			wantID:       "moonshotai/kimi-k3",
			wantProvider: providers.ProviderAiand,
			wantKnown:    true,
		},
		{
			name:         "alias claude-opus-4-8",
			input:        "claude-opus-4-8",
			wantID:       "zai-org/glm-5.2",
			wantProvider: providers.ProviderAiand,
			wantKnown:    true,
		},
		{
			name:         "alias claude-opus-5",
			input:        "claude-opus-5",
			wantID:       "zai-org/glm-5.2",
			wantProvider: providers.ProviderAiand,
			wantKnown:    true,
		},
		{
			name:         "alias claude-sonnet-5",
			input:        "claude-sonnet-5",
			wantID:       "moonshotai/kimi-k2.7",
			wantProvider: providers.ProviderAiand,
			wantKnown:    true,
		},
		{
			name:         "alias claude-haiku-4-5",
			input:        "claude-haiku-4-5",
			wantID:       "deepseek-ai/deepseek-v4-flash",
			wantProvider: providers.ProviderAiand,
			wantKnown:    true,
		},
		{
			name:         "alias claude-sonnet-4-6",
			input:        "claude-sonnet-4-6",
			wantID:       "deepseek-ai/deepseek-v4-pro",
			wantProvider: providers.ProviderAiand,
			wantKnown:    true,
		},
		{
			name:         "alias mixed case and whitespace",
			input:        "  Gemini  ",
			wantID:       "google/gemma-4-31b-it",
			wantProvider: providers.ProviderAiand,
			wantKnown:    true,
		},
		{
			name:         "alias qwen",
			input:        "qwen",
			wantID:       "qwen/qwen3.6-27b",
			wantProvider: providers.ProviderAiand,
			wantKnown:    true,
		},
		{
			name:         "canonical qwen/qwen3.6-27b with vendor prefix",
			input:        "qwen/qwen3.6-27b",
			wantID:       "qwen/qwen3.6-27b",
			wantProvider: providers.ProviderAiand,
			wantKnown:    true,
		},
		{
			name:         "dash spelling qwen/qwen-3.8-max",
			input:        "qwen/qwen-3.8-max",
			wantID:       "qwen/qwen3.6-27b",
			wantProvider: providers.ProviderAiand,
			wantKnown:    true,
		},
		{
			name:         "dash spelling qwen-3.8-max",
			input:        "qwen-3.8-max",
			wantID:       "qwen/qwen3.6-27b",
			wantProvider: providers.ProviderAiand,
			wantKnown:    true,
		},
		{
			name:         "dash spelling qwen-3.8",
			input:        "qwen-3.8",
			wantID:       "qwen/qwen3.6-27b",
			wantProvider: providers.ProviderAiand,
			wantKnown:    true,
		},
		// Heuristic fallback: not in the catalog, so known is false.
		{
			name:         "heuristic openai — gpt-6 not in catalog",
			input:        "gpt-6",
			wantID:       "gpt-6",
			wantProvider: providers.ProviderOpenAI,
			wantKnown:    false,
		},
		{
			name:         "heuristic openai — o3",
			input:        "o3",
			wantID:       "o3",
			wantProvider: providers.ProviderOpenAI,
			wantKnown:    false,
		},
		{
			name:         "heuristic openrouter — unknown slash model",
			input:        "mistral/mistral-small-2603",
			wantID:       "mistral/mistral-small-2603",
			wantProvider: providers.ProviderOpenRouter,
			wantKnown:    false,
		},
		{
			name:         "unknown native openai prefix",
			input:        "openai/gpt-6",
			wantID:       "gpt-6",
			wantProvider: providers.ProviderOpenAI,
			wantKnown:    false,
		},
		{
			name:         "heuristic anthropic — unknown bareword",
			input:        "totally-not-a-model",
			wantID:       "totally-not-a-model",
			wantProvider: providers.ProviderAnthropic,
			wantKnown:    false,
		},
		{
			name:         "truncated gpt- is not known",
			input:        "gpt-",
			wantID:       "gpt-",
			wantProvider: providers.ProviderOpenAI,
			wantKnown:    false,
		},
		{
			name:         "spaced model name is not known",
			input:        "qwen 3.8",
			wantID:       "qwen 3.8",
			wantProvider: providers.ProviderAnthropic,
			wantKnown:    false,
		},
		{
			name:         "spaced alias is not known",
			input:        "qwen max",
			wantID:       "qwen max",
			wantProvider: providers.ProviderAnthropic,
			wantKnown:    false,
		},
		{
			name:         "model name with a trailing prompt is not known",
			input:        "gpt-5 help me debug this",
			wantID:       "gpt-5 help me debug this",
			wantProvider: providers.ProviderOpenAI,
			wantKnown:    false,
		},
		{
			name:         "prefix of a real id is not known",
			input:        "claude-sonnet-4",
			wantID:       "claude-sonnet-4",
			wantProvider: providers.ProviderAnthropic,
			wantKnown:    false,
		},
		{
			// bare mimo is unknown (xiaomi/mimo* retired from aiand catalog).
			name:         "fragment of a bare name is not known",
			input:        "mimo",
			wantID:       "mimo",
			wantProvider: providers.ProviderAnthropic,
			wantKnown:    false,
		},
		{
			name:         "xhigh-style alias strips effort",
			input:        "claude-opus-5:xhigh",
			wantID:       "zai-org/glm-5.2",
			wantProvider: providers.ProviderAiand,
			wantKnown:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotID, gotProvider, gotKnown := resolveForceModel(tt.input)
			assert.Equal(t, tt.wantID, gotID, "canonical id")
			assert.Equal(t, tt.wantProvider, gotProvider, "provider")
			assert.Equal(t, tt.wantKnown, gotKnown, "known")
		})
	}
}

// The bare-name table must stay unambiguous as models are added: a tail shared
// by two models, or one that collides with a full ID or an alias, would make a
// bare name resolve to an arbitrary winner — the silent-wrong-model failure
// exact matching exists to prevent. Such tails are dropped, not guessed.
func TestBareCatalogNames_Unambiguous(t *testing.T) {
	tails := make(map[string][]string)
	for _, m := range catalog.Models {
		if _, tail, ok := strings.Cut(m.ID, "/"); ok && len(m.Providers) > 0 {
			tails[tail] = append(tails[tail], m.ID)
		}
	}

	for tail, owners := range tails {
		mapped, listed := bareCatalogNames[tail]
		_, isFullID := catalog.ByID(tail)
		_, aliased := forceModelAliases[tail]

		if len(owners) > 1 || isFullID || aliased {
			assert.False(t, listed,
				"ambiguous tail %q (owners=%v full_id=%v aliased=%v) must not be a bare name",
				tail, owners, isFullID, aliased)
			continue
		}
		require.True(t, listed, "unambiguous tail %q must be reachable without its vendor prefix", tail)
		assert.Equal(t, owners[0], mapped)
	}

	// Every entry must name a real, servable model.
	for tail, id := range bareCatalogNames {
		m, ok := catalog.ByID(id)
		require.True(t, ok, "bare name %q maps to unknown model %q", tail, id)
		assert.NotEmpty(t, m.Providers, "bare name %q maps to unservable model %q", tail, id)
	}
}

// An alias must win over a bare catalog name, so a deliberate alias can never
// be shadowed by an incidental tail collision.
func TestBareCatalogNames_AliasesTakePrecedence(t *testing.T) {
	for alias := range forceModelAliases {
		_, shadowed := bareCatalogNames[alias]
		assert.False(t, shadowed, "alias %q must not also be a bare-name entry", alias)
	}
}

// Family aliases (grok, xai) retarget to kimi-k2.7 on aiand.
func TestResolveForceModel_GrokFamilyAlias(t *testing.T) {
	for _, input := range []string{"grok", "xai"} {
		t.Run(input, func(t *testing.T) {
			gotID, gotProvider, gotKnown := resolveForceModel(input)
			assert.Equal(t, "moonshotai/kimi-k2.7", gotID, "canonical id")
			assert.Equal(t, providers.ProviderAiand, gotProvider, "provider")
			assert.True(t, gotKnown, "known")
		})
	}
}

// An explicit :level suffix must survive resolution to its catalog model.
func TestResolveForceModel_EffortSuffixPreserved(t *testing.T) {
	gotID, _, gotKnown, gotEffort := resolveForceModelWithEffort("moonshotai/kimi-k2.7:high")
	assert.Equal(t, "moonshotai/kimi-k2.7", gotID, "canonical id")
	assert.True(t, gotKnown, "known")
	assert.Equal(t, "high", gotEffort, "effort")

	_, _, _, noneEffort := resolveForceModelWithEffort("deepseek-ai/deepseek-v4-flash:none")
	assert.Equal(t, "none", noneEffort, ":none suffix must strip as effort none")

	_, _, _, maxEffort := resolveForceModelWithEffort("zai-org/glm-5.2:max")
	assert.Equal(t, "max", maxEffort)
}

// /v1/router/models on ai& deploys lists UpstreamIDs. Force-model must accept
// those exact strings and pin the catalog row — without renaming UpstreamID.
func TestResolveForceModel_AcceptsUpstreamRegistryIDs(t *testing.T) {
	tests := []struct {
		input  string
		wantID string
	}{
		{"deepseek-ai/deepseek-v4-flash", "deepseek-ai/deepseek-v4-flash"},
		{"zai-org/glm-5.2", "zai-org/glm-5.2"},
		// Legacy alias input resolves to the canonical row via catalog.aliases.
		{"zai-org/glm-5.2", "zai-org/glm-5.2"},
		{"moonshotai/kimi-k2.7-code", "moonshotai/kimi-k2.7"},
		{"moonshotai/kimi-k3", "moonshotai/kimi-k3"},
		{"motif-technologies/motif-3", "motif-technologies/motif-3"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			gotID, gotProvider, gotKnown := resolveForceModel(tt.input)
			assert.True(t, gotKnown, "known")
			assert.Equal(t, tt.wantID, gotID, "canonical catalog id")
			assert.Equal(t, providers.ProviderAiand, gotProvider, "primary binding")
		})
	}
}

// resolveRequestedModel decides force intent from the inbound `model` field
// and the x-weave-force-model header. The precedence contract under test:
// /force-model command > model field > header; model="auto" defers to the
// header; a resolvable field beats a conflicting header; unknown or excluded
// values fail with the typed error; the written pin carries the canonical
// model + provider; and ForceEffort rides along via the request context.
func TestResolveRequestedModel_PrecedenceAndPin(t *testing.T) {
	newSvc := func(store *recordingPinStore) *Service {
		return &Service{pinStore: store}
	}
	parsedBody := func(t *testing.T, body string) *translate.RequestEnvelope {
		env, err := translate.ParseAnthropic([]byte(body))
		require.NoError(t, err)
		return env
	}
	// Moonshotai kimi-k3 (alias "fable") is catalog-resolvable on aiand.
	const kimiK3 = "moonshotai/kimi-k3"
	tests := []struct {
		name      string
		body      string
		header    string
		wantID    string // "" = no force expected
		wantRole  string
		bodyModel string
		wantErr   error // nil = success
	}{
		{
			name:   "model auto with empty header routes normally",
			body:   `{"model":"auto","messages":[{"role":"user","content":"hi"}]}`,
			wantID: "",
		},
		{
			name:   "empty model with empty header routes normally",
			body:   `{"model":"","messages":[{"role":"user","content":"hi"}]}`,
			wantID: "",
		},
		{
			name:      "resolvable model field forces canonical id",
			body:      `{"model":"` + kimiK3 + `","messages":[{"role":"user","content":"hi"}]}`,
			wantID:    kimiK3,
			wantRole:  sessionpin.DefaultRole + "_high",
			bodyModel: kimiK3,
		},
		{
			name:      "alias model field forces canonical id",
			body:      `{"model":"fable","messages":[{"role":"user","content":"hi"}]}`,
			wantID:    kimiK3,
			wantRole:  sessionpin.DefaultRole,
			bodyModel: "fable",
		},
		{
			name:      "bare tail model field forces canonical id",
			body:      `{"model":"kimi-k3","messages":[{"role":"user","content":"hi"}]}`,
			wantID:    kimiK3,
			wantRole:  sessionpin.DefaultRole,
			bodyModel: "kimi-k3",
		},
		{
			name:      "model field beats conflicting header",
			body:      `{"model":"` + kimiK3 + `","messages":[{"role":"user","content":"hi"}]}`,
			header:    "opus",
			wantID:    kimiK3,
			wantRole:  sessionpin.DefaultRole + "_high",
			bodyModel: kimiK3,
		},
		{
			name:     "model auto defers to header",
			body:     `{"model":"auto","messages":[{"role":"user","content":"hi"}]}`,
			header:   "fable",
			wantID:   kimiK3,
			wantRole: sessionpin.DefaultRole,
		},
		{
			name:     "model auto defers to header effort",
			body:     `{"model":"auto","messages":[{"role":"user","content":"hi"}]}`,
			header:   "fable:high",
			wantID:   kimiK3,
			wantRole: sessionpin.DefaultRole,
		},
		{
			name:   "unknown model field routes without forcing",
			body:   `{"model":"gpt-4o","messages":[{"role":"user","content":"hi"}]}`,
			wantID: "",
		},
		{
			name:    "unknown header fails the request",
			body:    `{"model":"auto","messages":[{"role":"user","content":"hi"}]}`,
			header:  "not-a-model-xyz",
			wantErr: ErrForcedModelUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &recordingPinStore{}
			svc := newSvc(store)
			env := parsedBody(t, tt.body)
			bodyModel := env.Model()
			if tt.bodyModel != "" {
				bodyModel = tt.bodyModel
			}
			httpReq := httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
			if tt.header != "" {
				httpReq.Header.Set(ForceModelHeader, tt.header)
			}
			ctx := context.WithValue(context.Background(), InstallationIDContextKey{}, uuid.New().String())
			insID := uuid.New()
			got, err := svc.applyForceModel(ctx, httpReq, env, insID, DeriveSessionKey(env, "key-1"))
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr, "expected typed error")
				assert.Empty(t, store.upserts, "no pin must be written on a failed resolution")
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantID, got, "resolved force model")

			if tt.wantID == "" {
				assert.Empty(t, store.upserts, "auto/empty model must not write a pin")
				return
			}
			require.Len(t, store.upserts, 1, "a forcing turn must write exactly one pin")
			pin := store.upserts[0]
			assert.Equal(t, tt.wantID, pin.Model, "pin model is canonical")
			assert.Equal(t, providers.ProviderAiand, pin.Provider, "pin provider is the primary binding")
			assert.Equal(t, translate.ReasonUserForceModel, pin.Reason, "pin reason is user-forced")
			assert.Equal(t, tt.wantRole, pin.Role, "pin role follows the requested model tier")

			// Resolving force intent must never rewrite the client's model field:
			// env.Model() still reads the original value, and the effective model
			// rides separately on the pin and router.Request.ForceModel.
			assert.Equal(t, bodyModel, env.Model(), "the envelope model field must be left untouched")
		})
	}
}

// TestResolveRequestedModel_KeepsEnvModelByteForByte asserts the same
// invariant at the raw-byte level: after resolution the envelope's serialized
// body still contains the client's original model value.
func TestResolveRequestedModel_KeepsEnvModelByteForByte(t *testing.T) {
	store := &recordingPinStore{}
	svc := &Service{pinStore: store}
	body := []byte(`{"model":"fable[1m]","messages":[{"role":"user","content":"hi"}]}`)
	env, err := translate.ParseAnthropic(body)
	require.NoError(t, err)
	httpReq := httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	ctx := context.WithValue(context.Background(), InstallationIDContextKey{}, uuid.New().String())
	got, err := svc.applyForceModel(ctx, httpReq, env, uuid.New(), DeriveSessionKey(env, "key-1"))
	require.NoError(t, err)
	assert.Equal(t, "moonshotai/kimi-k3", got)

	// The resolver stripped [1m] only for its own lookup; the body itself is
	// still the exact byte string the client sent.
	assert.Equal(t, "fable[1m]", env.Model(), "env.Model() must not be rewritten")
	assert.NotNil(t, httpReq.Header)
}

// TestResolveRequestedModel_EffortSuffixMergesOverride guards the effort
// plumbing: a `:level` suffix on the model field must land on
// router.Overrides.ForceEffort in the request context so routingKnobsForRequest
// picks it up, and the pin must keep the canonical (suffix-stripped) model.
func TestResolveRequestedModel_EffortSuffixMergesOverride(t *testing.T) {
	store := &recordingPinStore{}
	svc := &Service{pinStore: store}
	env, err := translate.ParseAnthropic([]byte(`{"model":"zai-org/glm-5.2:high","messages":[{"role":"user","content":"hi"}]}`))
	require.NoError(t, err)

	httpReq := httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	ctx := context.WithValue(context.Background(), InstallationIDContextKey{}, uuid.New().String())
	got, err := svc.applyForceModel(ctx, httpReq, env, uuid.New(), DeriveSessionKey(env, "key-1"))
	require.NoError(t, err)

	assert.Equal(t, "zai-org/glm-5.2", got, "canonical model, suffix stripped")
	require.Len(t, store.upserts, 1)
	assert.Equal(t, "zai-org/glm-5.2", store.upserts[0].Model, "pin stores canonical model, not the effort-qualified input")

	knobs := router.RoutingKnobsFromContext(httpReq.Context())
	require.NotNil(t, knobs, "effort knob must be merged onto the request context")
	assert.Equal(t, "high", knobs.ForceEffort, ":high suffix becomes ForceEffort")
}

// TestResolveRequestedModel_ClaudeCodeContextStripsVariant guards the [1m]
// strip: Claude Code's context-window variant must not prevent an otherwise
// resolvable model from forcing.
func TestResolveRequestedModel_ClaudeCodeContextStripsVariant(t *testing.T) {
	store := &recordingPinStore{}
	svc := &Service{pinStore: store}
	env, err := translate.ParseAnthropic([]byte(`{"model":"moonshotai/kimi-k3[1m]","messages":[{"role":"user","content":"hi"}]}`))
	require.NoError(t, err)
	httpReq := httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	ctx := context.WithValue(context.Background(), InstallationIDContextKey{}, uuid.New().String())
	got, err := svc.applyForceModel(ctx, httpReq, env, uuid.New(), DeriveSessionKey(env, "key-1"))
	require.NoError(t, err)
	assert.Equal(t, "moonshotai/kimi-k3", got, "variant tag stripped before resolution")
	require.Len(t, store.upserts, 1)
	assert.Equal(t, "moonshotai/kimi-k3", store.upserts[0].Model)
}

// TestResolveRequestedModel_ExcludedModelFails guards exclusion parity: the
// model field must hit the same exclusion 400 the header and /force-model do.
func TestResolveRequestedModel_ExcludedModelFails(t *testing.T) {
	store := &recordingPinStore{}
	svc := &Service{pinStore: store}
	env, err := translate.ParseAnthropic([]byte(`{"model":"deepseek-ai/deepseek-v4-flash","messages":[{"role":"user","content":"hi"}]}`))
	require.NoError(t, err)
	httpReq := httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	ctx := context.WithValue(context.Background(), InstallationIDContextKey{}, uuid.New().String())
	ctx = context.WithValue(ctx, InstallationExcludedModelsContextKey{}, []string{"deepseek-ai/deepseek-v4-flash"})

	_, err = svc.applyForceModel(ctx, httpReq, env, uuid.New(), DeriveSessionKey(env, "key-1"))
	require.ErrorIs(t, err, ErrForcedModelExcluded, "excluded model must be rejected")
	assert.Empty(t, store.upserts)
}

// TestResolveRequestedModel_NoPinStoreStillResolves guards the pinStore-nil
// path (self-hosted without pins): the resolver still returns the canonical
// force model without writing anything.
func TestResolveRequestedModel_NoPinStoreStillResolves(t *testing.T) {
	svc := &Service{}
	env, err := translate.ParseAnthropic([]byte(`{"model":"fable","messages":[{"role":"user","content":"hi"}]}`))
	require.NoError(t, err)
	httpReq := httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	ctx := context.WithValue(context.Background(), InstallationIDContextKey{}, uuid.New().String())
	ctx, cancel := context.WithCancel(ctx)
	cancel()
	got, err := svc.applyForceModel(ctx, httpReq, env, uuid.New(), DeriveSessionKey(env, "key-1"))
	require.NoError(t, err)
	assert.Equal(t, "moonshotai/kimi-k3", got, "canonical force model returned without a pin store")
}

// TestResolveForceModel_GLM52CanonicalAlias pins the GLM-5.2 canonicalization
// contract: zai-org/glm-5.2 is the canonical catalog id going forward, and
// zai-org/glm-5.2 stays recognized as a backward-compat alias so stored session
// pins and frozen training artifacts keep resolving. The legacy name must
// resolve to the new canonical, never to itself.
func TestResolveForceModel_GLM52CanonicalAlias(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantID    string
		wantKnown bool
	}{
		{
			name:      "canonical id resolves to itself",
			input:     "zai-org/glm-5.2",
			wantID:    "zai-org/glm-5.2",
			wantKnown: true,
		},
		{
			name:      "legacy id resolves to canonical",
			input:     "zai-org/glm-5.2",
			wantID:    "zai-org/glm-5.2",
			wantKnown: true,
		},
		{
			name:      "claude alias resolves to canonical",
			input:     "claude",
			wantID:    "zai-org/glm-5.2",
			wantKnown: true,
		},
		{
			name:      "opus alias resolves to canonical",
			input:     "opus",
			wantID:    "zai-org/glm-5.2",
			wantKnown: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotID, gotProvider, gotKnown := resolveForceModel(tt.input)
			assert.Equal(t, tt.wantID, gotID, "canonical id")
			assert.Equal(t, providers.ProviderAiand, gotProvider, "primary binding is aiand")
			assert.Equal(t, tt.wantKnown, gotKnown, "known")
		})
	}
}
