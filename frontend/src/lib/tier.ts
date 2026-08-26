// Tier is derived from the live catalog's context_window because the routing
// catalog's Tier enum is compile-time (not a display source-of-truth). The
// bands match the current catalog: low ≤ 131K, mid ≤ 262K, high = 1M.
export type ModelTier = "low" | "mid" | "high";

export function tierForContextWindow(ctx: number): ModelTier {
  if (ctx <= 131_072) return "low";
  if (ctx <= 262_144) return "mid";
  return "high";
}