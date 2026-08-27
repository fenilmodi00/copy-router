package proxy

import (
	"context"
	"crypto/sha256"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"workweave/router/internal/providers"
	"workweave/router/internal/router/policy"
	"workweave/router/internal/router/sessionpin"
	"workweave/router/internal/translate"
)

const (
	struggleLowModel  = "deepseek-ai/deepseek-v4-flash"
	struggleMidModel  = "deepseek-ai/deepseek-v4-pro"
	struggleHighModel = "moonshotai/kimi-k3"
)

// fakeRosterSource serves a fixed per-cluster arm roster.
type fakeRosterSource struct {
	clusters map[string][]string
}

func (f fakeRosterSource) ClusterRoster(context.Context) (policy.RosterSnapshot, error) {
	return policy.RosterSnapshot{Clusters: f.clusters}, nil
}

// recordingStruggleStore captures escalation events and serves the budget count.
type recordingStruggleStore struct {
	mu     sync.Mutex
	events []StruggleEscalationEvent
	count  int64
}

func (r *recordingStruggleStore) InsertStruggleEscalationEvent(_ context.Context, p StruggleEscalationEvent) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, p)
	return nil
}

func (r *recordingStruggleStore) CountStruggleEscalationEvents(context.Context, []byte, string) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.count, nil
}

func struggleTestKey(seed byte) [sessionpin.SessionKeyLen]byte {
	var key [sessionpin.SessionKeyLen]byte
	sum := sha256.Sum256([]byte{seed})
	copy(key[:], sum[:])
	return key
}

// strugglingPin is a pin past the early operating point (turns and wall).
func strugglingPin(model, group string) sessionpin.Pin {
	return sessionpin.Pin{
		Model:         model,
		PolicyGroup:   group,
		Reason:        "hmm_sticky",
		TurnCount:     struggleEarlyTurns,
		FirstPinnedAt: time.Now().Add(-struggleEarlyWall - time.Minute),
	}
}

func newStruggleEscalationSvc(pins *stubPinStore, events *recordingStruggleStore, clusters map[string][]string) *Service {
	return NewService(nil, nil, nil, false, nil, pins, false, providers.ProviderAiand, struggleLowModel, nil).
		WithStruggleEscalationConfig(true, 0).
		WithStruggleEscalationStore(events).
		WithStruggleEscalationRoster(NewStruggleRoster(fakeRosterSource{clusters: clusters}))
}

func TestClustersAbove(t *testing.T) {
	assert.Equal(t, []string{"balanced", "medium", "high", "maximum"}, clustersAbove("fast"))
	assert.Equal(t, []string{"balanced", "medium", "high", "maximum"}, clustersAbove("explore"))
	assert.Equal(t, []string{"maximum"}, clustersAbove("high"))
	assert.Nil(t, clustersAbove("maximum"), "the top rung has nothing above it")
	assert.Nil(t, clustersAbove("not-a-cluster"))
}

func TestEscalationTarget_PrefersTheClusterAbove(t *testing.T) {
	roster := NewStruggleRoster(fakeRosterSource{clusters: map[string][]string{
		"balanced": {struggleLowModel, struggleMidModel},
		"high":     {struggleHighModel},
	}})

	target, cluster, err := roster.EscalationTarget(
		context.Background(), "balanced", struggleLowModel, nil, func(string) bool { return true },
	)

	require.NoError(t, err)
	assert.Equal(t, struggleHighModel, target, "a struggling session must move up, not sideways")
	assert.Equal(t, "high", cluster)
}

func TestEscalationTarget_FallsBackSidewaysWhenNothingAboveIsDispatchable(t *testing.T) {
	roster := NewStruggleRoster(fakeRosterSource{clusters: map[string][]string{
		"balanced": {struggleLowModel, struggleMidModel},
		"high":     {struggleHighModel},
	}})

	target, cluster, err := roster.EscalationTarget(
		context.Background(), "balanced", struggleLowModel, nil,
		func(model string) bool { return model != struggleHighModel },
	)

	require.NoError(t, err)
	assert.Equal(t, struggleMidModel, target)
	assert.Equal(t, "balanced", cluster)
}

func TestEscalationTarget_TopClusterMovesSideways(t *testing.T) {
	roster := NewStruggleRoster(fakeRosterSource{clusters: map[string][]string{
		"maximum": {struggleHighModel, struggleMidModel},
	}})

	target, cluster, err := roster.EscalationTarget(
		context.Background(), "maximum", struggleHighModel, nil, func(string) bool { return true },
	)

	require.NoError(t, err)
	assert.Equal(t, struggleMidModel, target)
	assert.Equal(t, "maximum", cluster)
}

func TestHandleStruggleEscalation_PinsTheClusterAbove(t *testing.T) {
	pins := newStubPinStore()
	pins.getFound = true
	pins.getPin = strugglingPin(struggleLowModel, "balanced")
	events := &recordingStruggleStore{}
	svc := newStruggleEscalationSvc(pins, events, map[string][]string{
		"balanced": {struggleLowModel, struggleMidModel},
		"high":     {struggleHighModel},
	})

	svc.handleStruggleEscalation(context.Background(), uuid.New(), struggleTestKey(1), "default")

	require.Len(t, pins.upserts, 1)
	assert.Equal(t, struggleHighModel, pins.upserts[0].Model)
	assert.Equal(t, "high", pins.upserts[0].PolicyGroup, "the pin must carry the cluster it was escalated into")
	assert.Equal(t, translate.ReasonStruggleEscalation, pins.upserts[0].Reason)

	require.Len(t, events.events, 1)
	assert.Equal(t, struggleActionUpCluster, events.events[0].Action)
	assert.Equal(t, struggleHighModel, events.events[0].EscalationTarget)
}

func TestHandleStruggleEscalation_SidewaysWhenTheClusterAboveCannotServe(t *testing.T) {
	pins := newStubPinStore()
	pins.getFound = true
	pins.getPin = strugglingPin(struggleLowModel, "balanced")
	events := &recordingStruggleStore{}
	svc := newStruggleEscalationSvc(pins, events, map[string][]string{
		"balanced": {struggleLowModel, struggleMidModel},
	})

	svc.handleStruggleEscalation(context.Background(), uuid.New(), struggleTestKey(2), "default")

	require.Len(t, pins.upserts, 1)
	assert.Equal(t, struggleMidModel, pins.upserts[0].Model)
	assert.Equal(t, "balanced", pins.upserts[0].PolicyGroup)

	require.Len(t, events.events, 1)
	assert.Equal(t, struggleActionSideways, events.events[0].Action)
}
