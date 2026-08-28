import { describe, expect, it } from "vitest";
import { savingsSubline } from "./metrics-summary";

describe("savingsSubline", () => {
  it("shows savings when requested cost exceeds actual", () => {
    const line = savingsSubline(
      {
        request_count: 10,
        total_tokens: 1000,
        total_requested_cost_usd: 2.35,
        total_actual_cost_usd: 1.17,
        total_savings_usd: 1.18,
      },
      [],
    );
    expect(line.text).toBe("$1.18 saved vs requested (50%)");
    expect(line.accent).toBe("success");
  });

  it("does not claim savings when requested and actual match", () => {
    const line = savingsSubline(
      {
        request_count: 5,
        total_tokens: 500,
        total_requested_cost_usd: 0.01,
        total_actual_cost_usd: 0.01,
        total_savings_usd: 0,
      },
      [],
    );
    expect(line.text).toBe("same as requested pricing");
    expect(line.accent).toBe("default");
  });

  it("falls back to bucket-requested sums when summary requested is zero", () => {
    const line = savingsSubline(
      {
        request_count: 3,
        total_tokens: 300,
        total_requested_cost_usd: 0,
        total_actual_cost_usd: 0.01,
        total_savings_usd: 0,
      },
      [
        {
          bucket: "2026-08-27T00:00:00.000Z",
          requested_cost_usd: 0.05,
          actual_cost_usd: 0.01,
        },
      ],
    );
    expect(line.text).toBe("$0.04 saved vs requested (80%)");
    expect(line.accent).toBe("success");
  });

  it("shows percent for sub-cent savings instead of rounding to $0.00", () => {
    const line = savingsSubline(
      {
        request_count: 137,
        total_tokens: 50000,
        total_requested_cost_usd: 0.013305,
        total_actual_cost_usd: 0.008517,
        total_savings_usd: 0.004788,
      },
      [],
    );
    expect(line.text).toBe("<$0.01 saved vs requested (36%)");
    expect(line.accent).toBe("success");
  });
});
