import type { MetricsSummary, TimeseriesBucket } from "@/lib/api";
import { formatSavingsUSD, formatUSD } from "@/lib/format";

export interface SavingsSubline {
  text: string;
  accent: "default" | "success";
}

/** Derive requested spend, falling back to bucket sums when summary is zero. */
export function resolvedRequestedCostUsd(
  summary: MetricsSummary,
  buckets: TimeseriesBucket[],
): number {
  if (summary.total_requested_cost_usd > 0) return summary.total_requested_cost_usd;
  return buckets.reduce((sum, b) => sum + b.requested_cost_usd, 0);
}

/** Derive savings for the Actual-cost KPI subline. */
export function savingsSubline(
  summary: MetricsSummary,
  buckets: TimeseriesBucket[],
): SavingsSubline {
  const requested = resolvedRequestedCostUsd(summary, buckets);
  const actual = summary.total_actual_cost_usd;
  const savings = requested - actual;

  if (requested <= 0) {
    return { text: "requested cost not recorded yet", accent: "default" };
  }
  if (savings > 0.000_001) {
    const pct = Math.round((savings / requested) * 100);
    const pctSuffix = pct > 0 ? ` (${pct}%)` : "";
    return {
      text: `${formatSavingsUSD(savings)} saved vs requested${pctSuffix}`,
      accent: "success",
    };
  }
  if (savings < -0.000_001) {
    return { text: `${formatUSD(Math.abs(savings))} over requested`, accent: "default" };
  }
  return { text: "same as requested pricing", accent: "default" };
}
