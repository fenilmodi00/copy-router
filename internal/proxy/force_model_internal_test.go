package proxy

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"workweave/router/internal/providers"
	"workweave/router/internal/router"
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
			name:         "catalog qwen",
			input:        "qwen/qwen3.8-27b",
			wantID:       "qwen/qwen3.8-27b",
			wantProvider: providers.ProviderAiand,
			wantKnown:    true,
		},
		{
			name:         "bare suffix no longer resolves without alias",
			input:        "qwen3.8-27b",
			wantID:       "qwen3.8-27b",
			wantProvider: providers.ProviderAiand,
			wantKnown:    false,
		},
		{
			name:         "native openai prefix",
			input:        "deepseek-ai/deepseek-v4-flash",
			wantID:       "deepseek-ai/deepseek-v4-flash",
			wantProvider: providers.ProviderAiand,
			wantKnown:    true,
		},
		{
			name:         "canonical qwen/qwen3.8-27b with vendor prefix",
			input:        "qwen/qwen3.8-27b",
			wantID:       "qwen/qwen3.8-27b",
			wantProvider: providers.ProviderAiand,
			wantKnown:    true,
		},
		{
			name:         "retired qwen3.6 id resolves via catalog alias",
			input:        "qwen/qwen3.6-27b",
			wantID:       "qwen/qwen3.8-27b",
			wantProvider: providers.ProviderAiand,
			wantKnown:    true,
		},
		{
			name:         "dash spelling no longer resolves without alias",
			input:        "qwen-3.8-max",
			wantID:       "qwen-3.8-max",
			wantProvider: providers.ProviderAiand,
			wantKnown:    false,
		},
		{
			name:         "dash spelling no longer resolves without alias",
			input:        "qwen-3.8",
			wantID:       "qwen-3.8",
			wantProvider: providers.ProviderAiand,
			wantKnown:    false,
		},
		// Heuristic fallback: not in the catalog, so known is false.
		{
			name:         "heuristic openai — gpt-6 not in catalog",
			input:        "gpt-6",
			wantID:       "gpt-6",
			wantProvider: providers.ProviderAiand,
			wantKnown:    false,
		},
		{
			name:         "heuristic openai — o3",
			input:        "o3",
			wantID:       "o3",
			wantProvider: providers.ProviderAiand,
			wantKnown:    false,
		},
		{
			name:         "heuristic openrouter — unknown slash model",
			input:        "mistral/mistral-small-2603",
			wantID:       "mistral/mistral-small-2603",
			wantProvider: providers.ProviderAiand,
			wantKnown:    false,
		},
		{
			name:         "unknown native openai prefix",
			input:        "openai/gpt-6",
			wantID:       "gpt-6",
			wantProvider: providers.ProviderAiand,
			wantKnown:    false,
		},
		{
			name:         "heuristic anthropic — unknown bareword",
			input:        "totally-not-a-model",
			wantID:       "totally-not-a-model",
			wantProvider: providers.ProviderAiand,
			wantKnown:    false,
		},
		{
			name:         "truncated gpt- is not known",
			input:        "gpt-",
			wantID:       "gpt-",
			wantProvider: providers.ProviderAiand,
			wantKnown:    false,
		},
		{
			name:         "spaced model name is not known",
			input:        "qwen 3.8",
			wantID:       "qwen 3.8",
			wantProvider: providers.ProviderAiand,
			wantKnown:    false,
		},
		{
			name:         "spaced alias is not known",
			input:        "qwen max",
			wantID:       "qwen max",
			wantProvider: providers.ProviderAiand,
			wantKnown:    false,
		},
		{
			name:         "model name with a trailing prompt is not known",
			input:        "gpt-5 help me debug this",
			wantID:       "gpt-5 help me debug this",
			wantProvider: providers.ProviderAiand,
			wantKnown:    false,
		},
		{
			name:         "prefix of a real id is not known",
			input:        "claude-sonnet-4",
			wantID:       "claude-sonnet-4",
			wantProvider: providers.ProviderAiand,
			wantKnown:    false,
		},
		{
			// bare mimo is unknown (xiaomi/mimo* retired from aiand catalog).
			name:         "fragment of a bare name is not known",
			input:        "mimo",
			wantID:       "mimo",
			wantProvider: providers.ProviderAiand,
			wantKnown:    false,
		},
		{
			name:         "xhigh effort strips from catalog model",
			input:        "zai-org/glm-5.3:xhigh",
			wantID:       "zai-org/glm-5.3",
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

// An explicit :level suffix must survive resolution to its catalog model.
func TestResolveForceModel_EffortSuffixPreserved(t *testing.T) {
	gotID, _, gotKnown, gotEffort := resolveForceModelWithEffort("moonshotai/kimi-k2.7:high")
	assert.Equal(t, "moonshotai/kimi-k2.7", gotID, "canonical id")
	assert.True(t, gotKnown, "known")
	assert.Equal(t, "high", gotEffort, "effort")

	_, _, _, noneEffort := resolveForceModelWithEffort("deepseek-ai/deepseek-v4-flash:none")
	assert.Equal(t, "none", noneEffort, ":none suffix must strip as effort none")
	_, _, _, maxEffort := resolveForceModelWithEffort("zai-org/glm-5.3:max")
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
		{"zai-org/glm-5.3", "zai-org/glm-5.3"},
		// Legacy alias input resolves to the canonical row via catalog.aliases.
		{"z-ai/glm-5.2", "zai-org/glm-5.3"},
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
			name:   "unknown alias routes without forcing",
			body:   `{"model":"fable","messages":[{"role":"user","content":"hi"}]}`,
			wantID: "",
		},
		{
			name:   "unknown bare tail routes without forcing",
			body:   `{"model":"kimi-k3","messages":[{"role":"user","content":"hi"}]}`,
			wantID: "",
		},
		{
			name:      "model field beats conflicting header",
			body:      `{"model":"` + kimiK3 + `","messages":[{"role":"user","content":"hi"}]}`,
			wantID:    kimiK3,
			wantRole:  sessionpin.DefaultRole + "_high",
			bodyModel: kimiK3,
		},
		{
			name:   "unknown header routes without forcing",
			body:   `{"model":"auto","messages":[{"role":"user","content":"hi"}]}`,
			wantID: "",
		},
		{
			name:   "unknown header effort routes without forcing",
			body:   `{"model":"auto","messages":[{"role":"user","content":"hi"}]}`,
			wantID: "",
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
	body := []byte(`{"model":"moonshotai/kimi-k3[1m]","messages":[{"role":"user","content":"hi"}]}`)
	env, err := translate.ParseAnthropic(body)
	require.NoError(t, err)
	httpReq := httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	ctx := context.WithValue(context.Background(), InstallationIDContextKey{}, uuid.New().String())
	got, err := svc.applyForceModel(ctx, httpReq, env, uuid.New(), DeriveSessionKey(env, "key-1"))
	require.NoError(t, err)
	assert.Equal(t, "moonshotai/kimi-k3", got)

	// The resolver stripped [1m] only for its own lookup; the body itself is
	// still the exact byte string the client sent.
	assert.Equal(t, "moonshotai/kimi-k3[1m]", env.Model(), "env.Model() must not be rewritten")
	assert.NotNil(t, httpReq.Header)
}

// TestResolveRequestedModel_EffortSuffixMergesOverride guards the effort
// plumbing: a `:level` suffix on the model field must land on
// router.Overrides.ForceEffort in the request context so routingKnobsForRequest
// picks it up, and the pin must keep the canonical (suffix-stripped) model.
func TestResolveRequestedModel_EffortSuffixMergesOverride(t *testing.T) {
	store := &recordingPinStore{}
	svc := &Service{pinStore: store}
	env, err := translate.ParseAnthropic([]byte(`{"model":"zai-org/glm-5.3:low","messages":[{"role":"user","content":"hi"}]}`))
	require.NoError(t, err)

	httpReq := httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	ctx := context.WithValue(context.Background(), InstallationIDContextKey{}, uuid.New().String())
	got, err := svc.applyForceModel(ctx, httpReq, env, uuid.New(), DeriveSessionKey(env, "key-1"))
	require.NoError(t, err)

	assert.Equal(t, "zai-org/glm-5.3", got, "canonical model, suffix stripped")
	require.Len(t, store.upserts, 1)
	assert.Equal(t, "zai-org/glm-5.3", store.upserts[0].Model, "pin stores canonical model, not the effort-qualified input")

	knobs := router.RoutingKnobsFromContext(httpReq.Context())
	require.NotNil(t, knobs, "effort knob must be merged onto the request context")
	assert.Equal(t, "low", knobs.ForceEffort, ":low suffix becomes ForceEffort")
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
	env, err := translate.ParseAnthropic([]byte(`{"model":"moonshotai/kimi-k3","messages":[{"role":"user","content":"hi"}]}`))
	require.NoError(t, err)
	httpReq := httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	ctx := context.WithValue(context.Background(), InstallationIDContextKey{}, uuid.New().String())
	ctx, cancel := context.WithCancel(ctx)
	cancel()
	got, err := svc.applyForceModel(ctx, httpReq, env, uuid.New(), DeriveSessionKey(env, "key-1"))
	require.NoError(t, err)
	assert.Equal(t, "moonshotai/kimi-k3", got, "canonical force model returned without a pin store")
}

// TestResolveForceModel_GLM53CanonicalAlias pins the GLM canonicalization
// contract: zai-org/glm-5.3 is the canonical catalog id going forward, and the
// legacy GLM-5.2 spellings stay recognized as backward-compat aliases so stored
// session pins and frozen training artifacts keep resolving. The legacy names
// must resolve to the new canonical, never to themselves.
func TestResolveForceModel_GLM53CanonicalAlias(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantID    string
		wantKnown bool
	}{
		{
			name:      "canonical id resolves to itself",
			input:     "zai-org/glm-5.3",
			wantID:    "zai-org/glm-5.3",
			wantKnown: true,
		},
		{
			name:      "legacy zai-org spelling resolves to canonical",
			input:     "zai-org/glm-5.2",
			wantID:    "zai-org/glm-5.3",
			wantKnown: true,
		},
		{
			name:      "legacy z-ai spelling resolves to canonical",
			input:     "z-ai/glm-5.2",
			wantID:    "zai-org/glm-5.3",
			wantKnown: true,
		},
		{
			name:      "claude alias now unknown",
			input:     "claude",
			wantID:    "claude",
			wantKnown: false,
		},
		{
			name:      "opus alias now unknown",
			input:     "opus",
			wantID:    "opus",
			wantKnown: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotID, gotProvider, gotKnown := resolveForceModel(tt.input)
			assert.Equal(t, tt.wantID, gotID, "canonical id")
			assert.Equal(t, tt.wantKnown, gotKnown, "known")
			if tt.wantKnown {
				assert.Equal(t, providers.ProviderAiand, gotProvider, "primary binding is aiand")
			}
		})
	}
}
