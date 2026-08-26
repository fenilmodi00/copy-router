package postgres

import (
	"testing"
	"time"

	"workweave/router/internal/proxy"
	"workweave/router/internal/sqlc"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/assert"
)

func TestTelemetryConvertersCarryCacheWriteRead(t *testing.T) {
	ts := time.Unix(1_700_000_000, 0).UTC()
	bucket := func(w, r int64) sqlc.GetTelemetryTimeseriesHourlyRow {
		return sqlc.GetTelemetryTimeseriesHourlyRow{
			Bucket:           pgtype.Timestamptz{Time: ts, Valid: true},
			RequestedCostUsd: 2_000_000,
			ActualCostUsd:    1_000_000,
			CacheWriteTokens: w,
			CacheReadTokens:  r,
		}
	}
	want := proxy.TelemetryBucket{
		Bucket:           ts,
		RequestedCostUSD: 2.0,
		ActualCostUSD:    1.0,
		CacheWriteTokens: 300,
		CacheReadTokens:  700,
	}
	for name, got := range map[string]proxy.TelemetryBucket{
		"hourly":       telemetryBucketFromHourlyRow(bucket(300, 700)),
		"daily":        telemetryBucketFromDailyRow(sqlc.GetTelemetryTimeseriesDailyRow{ // same 4 fields
			Bucket: bucket(300, 700).Bucket, RequestedCostUsd: bucket(300, 700).RequestedCostUsd,
			ActualCostUsd: bucket(300, 700).ActualCostUsd, CacheWriteTokens: 300, CacheReadTokens: 700,
		}),
	} {
		assert.Equal(t, want, got, "%s converter must carry the cache token SUMs", name)
	}

	wantModel := proxy.TelemetryModelBucket{
		Bucket:           ts,
		DecisionModel:    "deepseek-ai/deepseek-v4-flash",
		RequestCount:     10,
		TotalTokens:      55_000,
		ActualCostUSD:    0.5,
		CacheWriteTokens: 400,
		CacheReadTokens:  900,
	}
	gotModel := modelBucketFromHourlyRow(sqlc.GetTelemetryModelBreakdownHourlyRow{
		Bucket:           pgtype.Timestamptz{Time: ts, Valid: true},
		DecisionModel:    "deepseek-ai/deepseek-v4-flash",
		RequestCount:     10,
		TotalTokens:      55_000,
		ActualCostUsd:    500_000,
		CacheWriteTokens: 400,
		CacheReadTokens:  900,
	})
	assert.Equal(t, wantModel, gotModel, "model-bucket converter must carry the cache token SUMs")
}

func TestTelemetrySummaryCarriesCacheWriteRead(t *testing.T) {
	row := sqlc.GetTelemetrySummaryRow{
		RequestCount:          10,
		TotalTokens:           55_000,
		TotalRequestedCostUsd: 2_000_000,
		TotalActualCostUsd:    1_000_000,
		TotalSavingsUsd:       1_000_000,
		CacheWriteTokens:      12_345,
		CacheReadTokens:       67_890,
	}
	got := summaryFromRow(row)
	assert.Equal(t, int64(12_345), got.CacheWriteTokens)
	assert.Equal(t, int64(67_890), got.CacheReadTokens)
}