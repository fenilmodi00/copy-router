package proxy

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"workweave/router/internal/auth"
	"workweave/router/internal/providers"
	"workweave/router/internal/router/catalog"
	"workweave/router/internal/router/handover"
)

// auxTelemetryRepo captures the telemetry rows emitAuxiliaryInferenceTelemetry
// writes. fireTelemetry is async, so writes are mutex-guarded and read via
// waitForRows.
type auxTelemetryRepo struct {
	mu    sync.Mutex
	rows  []InsertTelemetryParams
	drain chan struct{}
}

func (r *auxTelemetryRepo) InsertRequestTelemetry(_ context.Context, p InsertTelemetryParams) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.rows = append(r.rows, p)
	if r.drain != nil {
		r.drain <- struct{}{}
	}
	return nil
}

func (r *auxTelemetryRepo) snapshot() []InsertTelemetryParams {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]InsertTelemetryParams, len(r.rows))
	copy(out, r.rows)
	return out
}

// waitForRows polls until n rows have landed or the deadline passes, returning
// whatever arrived. Callers assert on the result so a missing row fails with a
// content diff rather than a timeout.
func (r *auxTelemetryRepo) waitForRows(n int) []InsertTelemetryParams {
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if rows := r.snapshot(); len(rows) >= n {
			return rows
		}
		time.Sleep(5 * time.Millisecond)
	}
	return r.snapshot()
}

func (r *auxTelemetryRepo) GetTelemetrySummary(context.Context, string, time.Time, time.Time) (TelemetrySummary, error) {
	return TelemetrySummary{}, nil
}

func (r *auxTelemetryRepo) GetTelemetryTimeseries(context.Context, string, time.Time, time.Time, string) ([]TelemetryBucket, error) {
	return nil, nil
}

func (r *auxTelemetryRepo) GetTelemetrySummaryAll(context.Context, time.Time, time.Time) (TelemetrySummary, error) {
	return TelemetrySummary{}, nil
}

func (r *auxTelemetryRepo) GetTelemetryTimeseriesAll(context.Context, time.Time, time.Time, string) ([]TelemetryBucket, error) {
	return nil, nil
}

func (r *auxTelemetryRepo) GetTelemetryRows(context.Context, string, time.Time, time.Time, int32) ([]TelemetryRow, error) {
	return nil, nil
}

func (r *auxTelemetryRepo) GetTelemetryRowsAll(context.Context, time.Time, time.Time, int32) ([]TelemetryRow, error) {
	return nil, nil
}

func (r *auxTelemetryRepo) GetTelemetryModelBreakdown(context.Context, string, time.Time, time.Time, string) ([]TelemetryModelBucket, error) {
	return nil, nil
}

func (r *auxTelemetryRepo) GetTelemetryModelBreakdownAll(context.Context, time.Time, time.Time, string) ([]TelemetryModelBucket, error) {
	return nil, nil
}

func (r *auxTelemetryRepo) GetTelemetryBySessionSequence(context.Context, uuid.UUID, []byte, string, int) (TelemetryTurnResult, error) {
	return TelemetryTurnResult{}, nil
}

const (
	auxTestRequestID = "req_auxtest"
	auxTestOrgID     = "org_auxtest"
	// auxTestModel must exist in the router catalog: the point of the
	// cost assertions is that telemetry prices match catalog pricing.
	auxTestModel     = DefaultHandoverModel
	auxTestSessionID = "sess_auxtest"
)

// auxTestUsage is a summarizer usage with tokens in every bucket, so a
// dropped cache field shows up as a cost mismatch rather than a silent zero.
func auxTestUsage() handover.Usage {
	return handover.Usage{
		Model:         auxTestModel,
		Provider:      providers.ProviderAiand,
		InputTokens:   1200,
		OutputTokens:  90,
		CacheCreation: 300,
		CacheRead:     400,
	}
}

// auxTestService wires a Service with a capturing telemetry repo.
func auxTestService(t *testing.T) (*Service, *auxTelemetryRepo) {
	t.Helper()
	telemetryRepo := &auxTelemetryRepo{}
	return &Service{
		telemetry: telemetryRepo,
	}, telemetryRepo
}

// auxTestContext carries the installation + client identity the router's auth
// middleware would have stashed on a real request.
func auxTestContext(installationID uuid.UUID, sessionID string) context.Context {
	ctx := context.WithValue(context.Background(), InstallationIDContextKey{}, installationID.String())
	return context.WithValue(ctx, ClientIdentityContextKey{}, ClientIdentity{SessionID: sessionID})
}

// TestEmitAuxiliaryInferenceTelemetryTagsSessionAndCost is the contract the
// public session-cost endpoint depends on: a summarizer call produces a
// telemetry row carrying the SAME client session id as the turn that
// triggered it, with actual cost populated.
func TestEmitAuxiliaryInferenceTelemetryTagsSessionAndCost(t *testing.T) {
	s, telemetryRepo := auxTestService(t)
	installationID := uuid.New()
	usage := auxTestUsage()

	s.emitAuxiliaryInferenceTelemetry(auxTestContext(installationID, auxTestSessionID),
		auxTestRequestID, auxSuffixHandoverSummary, auxTestOrgID, usage)

	rows := telemetryRepo.waitForRows(1)
	require.Len(t, rows, 1, "an auxiliary call must write exactly one telemetry row")
	row := rows[0]

	assert.Equal(t, auxTestSessionID, row.SessionID,
		"the auxiliary row must carry the client session id, else session cost undercounts it")
	assert.Equal(t, SpanTypeAuxiliaryInference, row.SpanType,
		"auxiliary calls must not masquerade as router.upstream served turns")
	assert.Equal(t, installationID.String(), row.InstallationID)
	assert.Equal(t, auxTestRequestID+auxSuffixHandoverSummary, row.RequestID,
		"the suffix must match the historical ledger request-id convention so rows stay distinct")
	assert.Equal(t, auxTestRequestID, row.TraceID,
		"the trace id ties the auxiliary call back to the turn that triggered it")

	pricing, ok := catalog.PrimaryPriceFor(auxTestModel)
	require.True(t, ok, "the test model must be priced in the catalog")
	wantInput := catalog.EffectiveInputCost(usage.InputTokens, usage.CacheCreation, usage.CacheRead,
		pricing.InputUSDPer1M, pricing, usage.Provider)
	wantOutput := catalog.EffectiveOutputCost(usage.OutputTokens, pricing.OutputUSDPer1M)
	assert.Greater(t, wantInput+wantOutput, 0.0, "the fixture must produce a non-zero cost")
	assert.InDelta(t, wantInput, row.ActualInputCostUSD, 1e-12)
	assert.InDelta(t, wantOutput, row.ActualOutputCostUSD, 1e-12)
	assert.InDelta(t, wantInput, row.RequestedInputCostUSD, 1e-12,
		"requested mirrors actual: the router added this call, so it earns no savings credit")
	assert.InDelta(t, wantOutput, row.RequestedOutputCostUSD, 1e-12)

	assert.Equal(t, int32(usage.InputTokens), row.InputTokens)
	assert.Equal(t, int32(usage.OutputTokens), row.OutputTokens)
	require.NotNil(t, row.CacheCreationTokens)
	assert.Equal(t, int32(usage.CacheCreation), *row.CacheCreationTokens)
	require.NotNil(t, row.CacheReadTokens)
	assert.Equal(t, int32(usage.CacheRead), *row.CacheReadTokens)
}

// TestEmitAuxiliaryInferenceTelemetrySkipsNonCalls proves a skipped or failed
// summarizer writes nothing: a zero-token row would inflate a session's
// request_count with a call that never happened.
func TestEmitAuxiliaryInferenceTelemetrySkipsNonCalls(t *testing.T) {
	cases := []struct {
		name  string
		usage handover.Usage
	}{
		{name: "summarizer never ran", usage: handover.Usage{}},
		{name: "no model reported", usage: handover.Usage{InputTokens: 100, OutputTokens: 10}},
		{name: "no tokens consumed", usage: handover.Usage{Model: auxTestModel, Provider: providers.ProviderAiand}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s, telemetryRepo := auxTestService(t)

			s.emitAuxiliaryInferenceTelemetry(auxTestContext(uuid.New(), auxTestSessionID),
				auxTestRequestID, auxSuffixHandoverSummary, auxTestOrgID, tc.usage)

			// Give the async telemetry write a chance to land if it were fired.
			time.Sleep(50 * time.Millisecond)
			assert.Empty(t, telemetryRepo.snapshot(), "no upstream call means no telemetry row")
		})
	}
}

// TestEmitAuxiliaryInferenceTelemetrySkipsWithoutInstallation proves an
// unauthenticated path writes no telemetry: the telemetry table's
// installation_id is NOT NULL.
func TestEmitAuxiliaryInferenceTelemetrySkipsWithoutInstallation(t *testing.T) {
	s, telemetryRepo := auxTestService(t)

	ctx := context.WithValue(context.Background(), ClientIdentityContextKey{},
		ClientIdentity{SessionID: auxTestSessionID})
	s.emitAuxiliaryInferenceTelemetry(ctx, auxTestRequestID, auxSuffixHandoverSummary, auxTestOrgID, auxTestUsage())

	time.Sleep(50 * time.Millisecond)
	assert.Empty(t, telemetryRepo.snapshot(), "no installation means no row to attribute")
}

// TestEmitAuxiliaryInferenceTelemetryAttributesUser proves the row carries
// the resolved router user id when one is on the context, so per-user cost
// breakdowns include router-originated calls.
func TestEmitAuxiliaryInferenceTelemetryAttributesUser(t *testing.T) {
	s, telemetryRepo := auxTestService(t)
	installationID := uuid.New()

	ctx := auxTestContext(installationID, auxTestSessionID)
	ctx = context.WithValue(ctx, auth.UserIDContextKey{}, "user_auxtest")

	s.emitAuxiliaryInferenceTelemetry(ctx, auxTestRequestID, auxSuffixHandoverSummary, auxTestOrgID, auxTestUsage())

	rows := telemetryRepo.waitForRows(1)
	require.Len(t, rows, 1)
	assert.Equal(t, "user_auxtest", rows[0].RouterUserID,
		"the auxiliary row must attribute the user so per-user cost sums include it")
}
