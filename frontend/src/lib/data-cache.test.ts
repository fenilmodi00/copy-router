import { describe, expect, it } from "vitest";

import type { ModelBreakdownBucket } from "@/lib/api";
import {
  aggregateModelTotals,
  modelWindowStats,
  queryKeys,
} from "@/lib/data-cache";

describe("queryKeys", () => {
  it("keeps catalog and range-scoped metrics keys distinct so remounts share catalog but not windows", () => {
    // Behavioral: same catalog identity across callers; different windows must
    // not collide or a filter change would serve the wrong timeseries.
    expect(queryKeys.catalog).not.toEqual(
      queryKeys.metricsSummary("a", "b") as unknown as readonly string[],
    );
    expect(queryKeys.metricsSummary("a", "b")).not.toEqual(
      queryKeys.metricsSummary("a", "c"),
    );
    expect(queryKeys.metricsTimeseries("hour", "a", "b")).not.toEqual(
      queryKeys.metricsTimeseries("day", "a", "b"),
    );
    expect(queryKeys.metricsModelBreakdown("hour", "a", "b")).not.toEqual(
      queryKeys.metricsSummary("a", "b"),
    );
  });
});

describe("aggregateModelTotals", () => {
  it("sums per-model tokens/cost/requests from model-breakdown buckets", () => {
    const buckets: ModelBreakdownBucket[] = [
      {
        bucket: "2026-08-27T00:00:00Z",
        decision_model: "deepseek-ai/deepseek-v4-flash",
        request_count: 3,
        total_tokens: 1000,
        actual_cost_usd: 0.1,
      },
      {
        bucket: "2026-08-27T01:00:00Z",
        decision_model: "deepseek-ai/deepseek-v4-flash",
        request_count: 2,
        total_tokens: 500,
        actual_cost_usd: 0.05,
      },
      {
        bucket: "2026-08-27T01:00:00Z",
        decision_model: "z-ai/glm-5.2",
        request_count: 1,
        total_tokens: 200,
        actual_cost_usd: 0.2,
      },
    ];
    const totals = aggregateModelTotals(buckets);
    expect(totals).toHaveLength(2);
    expect(totals[0]).toMatchObject({
      id: "deepseek-ai/deepseek-v4-flash",
      tokens: 1500,
      requests: 5,
    });
    expect(totals[0]!.costUsd).toBeCloseTo(0.15, 10);
    expect(totals[1]).toEqual({
      id: "z-ai/glm-5.2",
      label: "z-ai/glm-5.2",
      tokens: 200,
      costUsd: 0.2,
      requests: 1,
    });
  });
});

describe("modelWindowStats", () => {
  it("fans 24h/7d/30d totals out of one breakdown without hour-vs-day unit bugs", () => {
    const now = new Date("2026-08-27T12:00:00.000Z");
    const buckets: ModelBreakdownBucket[] = [
      // 12h ago — inside 24h
      {
        bucket: "2026-08-27T00:00:00.000Z",
        decision_model: "m1",
        request_count: 10,
        total_tokens: 100,
        actual_cost_usd: 1,
      },
      // 3 days ago — inside 7d and 30d, not 24h
      {
        bucket: "2026-08-24T12:00:00.000Z",
        decision_model: "m1",
        request_count: 5,
        total_tokens: 50,
        actual_cost_usd: 0.5,
      },
      // 20 days ago — only 30d
      {
        bucket: "2026-08-07T12:00:00.000Z",
        decision_model: "m1",
        request_count: 2,
        total_tokens: 20,
        actual_cost_usd: 0.2,
      },
      // other model — ignored
      {
        bucket: "2026-08-27T00:00:00.000Z",
        decision_model: "other",
        request_count: 99,
        total_tokens: 999,
        actual_cost_usd: 9,
      },
    ];

    const stats = modelWindowStats(buckets, "m1", now);
    expect(stats["24h"]).toEqual({ requests: 10, tokens: 100, cost: 1 });
    expect(stats["7d"]).toEqual({ requests: 15, tokens: 150, cost: 1.5 });
    expect(stats["30d"]).toEqual({ requests: 17, tokens: 170, cost: 1.7 });
  });
});
