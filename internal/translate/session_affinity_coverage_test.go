package translate_test

import (
	"net/http"
	"testing"

	"workweave/router/internal/providers"
	"workweave/router/internal/translate"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// applySessionAffinity (internal/translate/emit_openai.go) collapsed to a
// single mechanism when the per-provider branches went away: any
// OpenAI-compat target with a real session key gets the generic
// x-session-affinity header; everything else gets no hint. These tests pin
// that contract.

func emitWithAffinity(t *testing.T, provider, affinity string) (http.Header, map[string]any) {
	t.Helper()
	env, err := translate.ParseAnthropic(anthropicSrc())
	require.NoError(t, err)

	out, err := env.PrepareOpenAI(nil, translate.EmitOptions{
		TargetModel:     "some/model",
		TargetProvider:  provider,
		SessionAffinity: affinity,
	})
	require.NoError(t, err)

	var body map[string]any
	return out.Headers, body
}

// TestSessionAffinity_GenericHeaderForOpenAICompat pins the fallback: an
// OpenAI-compat target carrying a real session key gets the generic
// x-session-affinity header, and only that — no other provider-specific
// affinity knob survives the prune.
func TestSessionAffinity_GenericHeaderForOpenAICompat(t *testing.T) {
	headers, _ := emitWithAffinity(t, providers.ProviderAiand, affinityKey)
	assert.Equal(t, affinityKey, headers.Get("x-session-affinity"))
	assert.Empty(t, headers.Get("x-session-id"))
	assert.Empty(t, headers.Get("x-grok-conv-id"))
}

func TestSessionAffinity_EmptyKeyIsNoOp(t *testing.T) {
	// An empty key must not hint the request: a bare header would herd
	// prefix-less sessions onto one synthetic affinity lane.
	headers, _ := emitWithAffinity(t, providers.ProviderAiand, "")
	assert.Empty(t, headers.Get("x-session-affinity"))
}

func TestSessionAffinity_NonOpenAICompatGetsNoHeader(t *testing.T) {
	// Unregistered (non-OpenAI-compat) families get no hint — the header
	// would be an unknown field to them.
	headers, _ := emitWithAffinity(t, "test-noncompat-provider", affinityKey)
	assert.Empty(t, headers.Get("x-session-affinity"))
}
