import { describe, expect, it } from "vitest";
import { denseTimeseriesBuckets, truncateToBucketUTC } from "./metrics-timeseries";

describe("denseTimeseriesBuckets", () => {
  it("fills zero buckets between sparse activity so sparklines have variance", () => {
    const sparse = [
      {
        bucket: "2026-08-27T00:00:00.000Z",
        requested_cost_usd: 2,
        actual_cost_usd: 1,
        total_tokens: 40_000,
        request_count: 8,
      },
      {
        bucket: "2026-08-27T01:00:00.000Z",
        requested_cost_usd: 0.35,
        actual_cost_usd: 0.17,
        total_tokens: 7_000,
        request_count: 2,
      },
    ];

    const dense = denseTimeseriesBuckets(
      sparse,
      "hour",
      "2026-08-26T15:30:00.000Z",
      "2026-08-27T15:30:00.000Z",
    );

    expect(dense.length).toBeGreaterThan(2);
    const tokens = dense.map(b => b.total_tokens ?? 0);
    expect(new Set(tokens).size).toBeGreaterThan(1);
    expect(tokens.reduce((a, b) => a + b, 0)).toBe(47_000);
  });

  it("aligns bucket keys to UTC hour boundaries", () => {
    expect(truncateToBucketUTC(new Date("2026-08-27T01:42:00.000Z"), "hour").toISOString()).toBe(
      "2026-08-27T01:00:00.000Z",
    );
  });
});
