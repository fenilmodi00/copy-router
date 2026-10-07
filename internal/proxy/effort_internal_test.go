package proxy

import (
	"context"
	"testing"

	"weave-os/router/internal/providers"
	"weave-os/router/internal/router"
	"weave-os/router/internal/translate"

	"github.com/stretchr/testify/assert"
)

func TestResolveEffort_SourcesAndClamping(t *testing.T) {
	svc := NewService(nil, nil, nil, false, nil, nil, false,
		providers.ProviderAIAND, "zai-org/glm-5.3-flash", nil).WithEffortEscalation(true)

	for _, tc := range []struct {
		name         string
		model        string
		armEffort    string
		knob         string
		escalate     bool
		wantSelected string
		wantSent     string
		wantSource   string
		wantMismatch bool
	}{
		{
			// No roster model carries a forced per-model effort, so a bare
			// turn keeps adaptive effort: nothing selected, nothing sent.
			name:  "no arm or knob leaves adaptive effort untouched",
			model: "zai-org/glm-5.3-flash",
		},
		{
			name:         "arm level the target accepts",
			model:        "zai-org/glm-5.3",
			armEffort:    "medium",
			wantSelected: "medium",
			wantSent:     "medium",
			wantSource:   effortSourceArm,
		},
		{
			name:         "top-of-menu arm is served unclamped",
			model:        "deepseek-ai/deepseek-v4-flash",
			armEffort:    "high",
			wantSelected: "high",
			wantSent:     "high",
			wantSource:   effortSourceArm,
		},
		{
			name:         "user knob outranks the arm",
			model:        "zai-org/glm-5.3",
			armEffort:    "high",
			knob:         "low",
			wantSelected: "low",
			wantSent:     "low",
			wantSource:   effortSourceUser,
			wantMismatch: true,
		},
		{
			// No roster model has a per-model escalation default, so a failed
			// turn cannot raise the arm's level.
			name:         "escalation never downgrades a richer arm",
			model:        "qwen/qwen3.8-27b",
			armEffort:    "high",
			escalate:     true,
			wantSelected: "high",
			wantSent:     "high",
			wantSource:   effortSourceArm,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			if tc.knob != "" {
				ctx = router.WithRoutingKnobs(ctx, &router.Overrides{ForceEffort: tc.knob})
			}
			decision := router.Decision{Model: tc.model, Effort: tc.armEffort}

			got := svc.resolveEffort(ctx, decision, router.Lookup(tc.model), tc.escalate)

			assert.Equal(t, tc.wantSelected, got.Selected)
			assert.Equal(t, tc.wantSent, got.Sent)
			assert.Equal(t, tc.wantSource, got.Source)
			assert.Equal(t, router.CanonicalizeEffort(tc.armEffort), got.Arm)
			assert.Equal(t, tc.wantMismatch, got.Mismatch())
		})
	}
}

// The arm's level goes on ForceEffort so the per-model cap still applies
// downstream, while an escalation/model-policy level is a wire level only.
func TestEffortResolution_Apply(t *testing.T) {
	caps := router.Lookup("deepseek-ai/deepseek-v4-flash")

	opts := translate.EmitOptions{Capabilities: caps}
	effortResolutionFor(caps, "high", "high", effortSourceArm).apply(&opts)
	assert.Equal(t, "high", opts.ForceEffort)
	assert.Equal(t, "high", opts.ForceReasoningEffort)

	escalated := translate.EmitOptions{Capabilities: caps}
	effortResolutionFor(caps, "low", "high", effortSourceEscalation).apply(&escalated)
	assert.Empty(t, escalated.ForceEffort, "a wire level is not an arm cap input")
	assert.Equal(t, "high", escalated.ForceReasoningEffort)

	effortResolution{}.apply(&escalated)
	assert.Empty(t, escalated.ForceEffort)
	assert.Empty(t, escalated.ForceReasoningEffort)
}

// A rescue candidate never served the failed model's level, so keeping it
// would report an identity the sibling never served.
func TestRescueDecisionFor_DropsEffort(t *testing.T) {
	failed := router.Decision{
		Provider: providers.ProviderAIAND,
		Model:    "zai-org/glm-5.3-flash",
		Effort:   "high",
		Metadata: &router.RoutingMetadata{SelectedArmID: "zai-org/glm-5.3-flash:high"},
	}

	out := rescueDecisionFor(failed, "moonshotai/kimi-k3", providers.ProviderAIAND, ReasonSiblingFailover)

	assert.Empty(t, out.Effort)
	assert.Equal(t, "moonshotai/kimi-k3", out.ServedIdentity())
}
