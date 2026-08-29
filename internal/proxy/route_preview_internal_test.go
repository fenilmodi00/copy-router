package proxy

import (
	"context"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The /v1/route preview resolves force-model intent from the inbound `model`
// field through the exact resolver the dispatch path uses, but never writes a
// session pin. These tests exercise anthropicRoutingRequest / openAIRoutingRequest
// (the Service methods behind RouteAnthropicRequest / the Playground preview) to
// guarantee parity with applyForceModel on the dispatch side.
func TestAnthropicRoutingRequest_ModelFieldForcesPreview(t *testing.T) {
	svc := &Service{}
	ctx := context.WithValue(context.Background(), InstallationIDContextKey{}, uuid.New().String())

	req, err := svc.anthropicRoutingRequest(ctx, []byte(`{"model":"z-ai/glm-5.2","messages":[{"role":"user","content":"hi"}]}`), nil)
	require.NoError(t, err)
	assert.Equal(t, "zai-org/glm-5.3", req.ForceModel, "catalog alias 'z-ai/glm-5.2' forces the canonical id in preview")
	assert.Equal(t, "z-ai/glm-5.2", req.RequestedModel, "requested model keeps the raw field; ForceModel carries the force")
}

func TestAnthropicRoutingRequest_AutoLeavesPreviewUnforced(t *testing.T) {
	svc := &Service{}
	ctx := context.WithValue(context.Background(), InstallationIDContextKey{}, uuid.New().String())

	req, err := svc.anthropicRoutingRequest(ctx, []byte(`{"model":"auto","messages":[{"role":"user","content":"hi"}]}`), nil)
	require.NoError(t, err)
	assert.Equal(t, "", req.ForceModel, "model=auto must not force in preview")
}

func TestAnthropicRoutingRequest_UnknownClientModelRoutesNormally(t *testing.T) {
	svc := &Service{}
	ctx := context.WithValue(context.Background(), InstallationIDContextKey{}, uuid.New().String())

	req, err := svc.anthropicRoutingRequest(ctx, []byte(`{"model":"claude-sonnet-4-20250514","messages":[{"role":"user","content":"hi"}]}`), nil)
	require.NoError(t, err, "client passthrough aliases must not fail route preview")
	assert.Equal(t, "", req.ForceModel, "unknown client model must not force")
	assert.Equal(t, "claude-sonnet-4-20250514", req.RequestedModel)
}

func TestAnthropicRoutingRequest_UnknownModelWithForceHeaderFailsPreview(t *testing.T) {
	svc := &Service{}
	ctx := context.WithValue(context.Background(), InstallationIDContextKey{}, uuid.New().String())
	headers := http.Header{}
	headers.Set(ForceModelHeader, "gpt-4o")

	_, err := svc.anthropicRoutingRequest(ctx, []byte(`{"model":"auto","messages":[{"role":"user","content":"hi"}]}`), headers)
	require.ErrorIs(t, err, ErrForcedModelUnknown, "explicit header force on unknown model must fail like dispatch")
}

func TestAnthropicRoutingRequest_ContextVariantStrippedInPreview(t *testing.T) {
	svc := &Service{}
	ctx := context.WithValue(context.Background(), InstallationIDContextKey{}, uuid.New().String())

	req, err := svc.anthropicRoutingRequest(ctx, []byte(`{"model":"moonshotai/kimi-k3[1m]","messages":[{"role":"user","content":"hi"}]}`), nil)
	require.NoError(t, err)
	assert.Equal(t, "moonshotai/kimi-k3", req.ForceModel, "[1m] variant stripped before preview resolution")
}

func TestOpenAIRoutingRequest_ModelFieldForcesPreview(t *testing.T) {
	svc := &Service{}
	ctx := context.WithValue(context.Background(), InstallationIDContextKey{}, uuid.New().String())

	req, err := svc.openAIRoutingRequest(ctx, []byte(`{"model":"z-ai/glm-5.2:high","messages":[{"role":"user","content":"hi"}]}`), nil)
	require.NoError(t, err)
	assert.Equal(t, "zai-org/glm-5.3", req.ForceModel, "resolvable model with :level effort forces the canonical id in preview")
	assert.Equal(t, "z-ai/glm-5.2:high", req.RequestedModel, "requested model keeps the raw field with effort suffix")
}

func TestOpenAIRoutingRequest_AutoLeavesPreviewUnforced(t *testing.T) {
	svc := &Service{}
	ctx := context.WithValue(context.Background(), InstallationIDContextKey{}, uuid.New().String())

	req, err := svc.openAIRoutingRequest(ctx, []byte(`{"model":"auto","messages":[{"role":"user","content":"hi"}]}`), nil)
	require.NoError(t, err)
	assert.Equal(t, "", req.ForceModel)
}

// Preview resolution must not error on excluded models the way dispatch's
// applyForceModel does not either at resolve-time for unloaded contexts —
// exclusion checks happen on the dispatch side inside forcedModelBinding. The
// preview only maps the name to a catalog row; exclusion policy applies at
// decision time. Assert the catalog mapping is permissive here, exactly like
// the dispatch resolver's resolveForcedModel (which gatekeeps exclusions only
// when forcedModelBinding rejects).
func TestOpenAIRoutingRequest_UnknownClientModelRoutesNormally(t *testing.T) {
	svc := &Service{}
	ctx := context.WithValue(context.Background(), InstallationIDContextKey{}, uuid.New().String())

	req, err := svc.openAIRoutingRequest(ctx, []byte(`{"model":"claude-sonnet-4-20250514","messages":[{"role":"user","content":"hi"}]}`), nil)
	require.NoError(t, err)
	assert.Equal(t, "", req.ForceModel)
}

func TestOpenAIRoutingRequest_UnknownModelWithForceHeaderFailsPreview(t *testing.T) {
	svc := &Service{}
	ctx := context.WithValue(context.Background(), InstallationIDContextKey{}, uuid.New().String())
	headers := http.Header{}
	headers.Set(ForceModelHeader, "not-a-real-model-xyz")

	_, err := svc.openAIRoutingRequest(ctx, []byte(`{"model":"auto","messages":[{"role":"user","content":"hi"}]}`), headers)
	require.ErrorIs(t, err, ErrForcedModelUnknown)
}

// The preview build must produce a fully-formed router.Request with ForceModel
// still honored by the bandit/turnloop consumers — cheap structural sanity.
func TestAnthropicRoutingRequest_ForceModelCarriedInRequest(t *testing.T) {
	svc := &Service{}
	ctx := context.WithValue(context.Background(), InstallationIDContextKey{}, uuid.New().String())
	ctx = context.WithValue(ctx, ExternalIDContextKey{}, "org-1")

	req, err := svc.anthropicRoutingRequest(ctx, []byte(`{"model":"deepseek-ai/deepseek-v4-flash","messages":[{"role":"user","content":"hi"}]}`), nil)
	require.NoError(t, err)
	assert.Equal(t, "deepseek-ai/deepseek-v4-flash", req.ForceModel)
	assert.Equal(t, "deepseek-ai/deepseek-v4-flash", req.RequestedModel)
	assert.Equal(t, "org-1", req.OrganizationID)
	assert.NotNil(t, req.TranslationRequirements, "TranslationRequirements must be populated for a preview request")
}
