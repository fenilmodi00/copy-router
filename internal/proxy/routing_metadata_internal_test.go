package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"workweave/router/internal/router"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type routingMetadataTestRouter struct {
	decision router.Decision
}

func (r *routingMetadataTestRouter) Route(context.Context, router.Request) (router.Decision, error) {
	return r.decision, nil
}

func TestEmitOpenAIRoutingMetadataEvent(t *testing.T) {
	var buf bytes.Buffer
	err := emitOpenAIRoutingMetadataEvent(&buf, RoutingMetadataPayload{
		Model:            "deepseek-ai/deepseek-v4-flash",
		Provider:         "aiand",
		Reason:           "cluster:v-test",
		RequestedCostUSD: 0.01,
		ActualCostUSD:    0.008,
		ID:               "req-123",
	})
	require.NoError(t, err)

	out := buf.String()
	assert.Contains(t, out, "event: routing_metadata")
	assert.Contains(t, out, "deepseek-ai/deepseek-v4-flash")

	var parsed RoutingMetadataPayload
	line := strings.TrimPrefix(out, "event: routing_metadata\ndata: ")
	line = strings.TrimSpace(strings.ReplaceAll(line, "\n\n", ""))
	require.NoError(t, json.Unmarshal([]byte(line), &parsed))
	assert.Equal(t, "aiand", parsed.Provider)
}

func TestRoutingMetadataPayloadCanonicalizesModel(t *testing.T) {
	svc := NewService(
		&routingMetadataTestRouter{decision: router.Decision{Model: "z-ai/glm-5.2", Provider: "aiand"}},
		nil, nil, false, nil, nil, false, "", "", nil,
	)
	payload := routingMetadataPayload(svc, t.Context(), router.Decision{
		Model:    "z-ai/glm-5.2",
		Provider: "aiand",
		Reason:   "pin",
	}, "req-1", []byte(`{"model":"auto","messages":[{"role":"user","content":"hello"}],"max_tokens":1024}`), 1, 1024)
	assert.Equal(t, "zai-org/glm-5.2", payload.Model)
	assert.Equal(t, "pin", payload.Reason)
	assert.Greater(t, payload.RequestedCostUSD, 0.0)
	assert.Greater(t, payload.ActualCostUSD, 0.0)
}

func TestPlaygroundReasonShort_ClusterDump(t *testing.T) {
	got := PlaygroundReasonShort("cluster:v0.76 top_p=[9,10,11,15] model=deepseek-ai/deepseek-v4-flash provider=aiand")
	assert.Equal(t, "Auto-routed", got)
}
