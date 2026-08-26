// Central palette for capability + tier badges so all catalog surfaces render
// the same color grammar (one source of truth, like the Chart color scales).
export const CAPABILITY_COLORS: Record<string, string> = {
  vision: "text-violet-400",
  video: "text-violet-400",
  document: "text-sky-400",
  reasoning: "text-amber-400",
  tool_calling: "text-emerald-400",
};

export const TIER_COLORS: Record<string, string> = {
  low: "text-primary",
  mid: "text-amber-400",
  high: "text-danger",
};

export function capabilityColor(cap: string): string {
  return CAPABILITY_COLORS[cap] ?? "text-muted-foreground";
}