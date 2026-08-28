// Shared number formatting for the dashboard. formatUSD/formatNumber were
// previously inlined per-chart; they are promoted here so the KPI cards,
// leaderboard, and compare verdicts share one implementation (DRY).
export function formatUSD(v: number): string {
  if (v === 0) return "$0.00";
  if (Number.isNaN(v)) return "—";
  if (Math.abs(v) < 0.001) return `$${v.toFixed(4)}`;
  return `$${v.toFixed(2)}`;
}

/** formatSavingsUSD renders a positive savings delta without rounding to $0.00. */
export function formatSavingsUSD(v: number): string {
  if (!Number.isFinite(v) || v <= 0) return "$0.00";
  const abs = Math.abs(v);
  if (abs < 0.005) return "<$0.01";
  if (abs < 0.01) return `$${abs.toFixed(3)}`;
  return formatUSD(abs);
}

export function formatNumber(v: number): string {
  if (Number.isNaN(v)) return "—";
  if (v >= 1_000_000) return `${(v / 1_000_000).toFixed(1)}M`;
  if (v >= 1_000) return `${(v / 1_000).toFixed(1)}K`;
  return String(v);
}

const averageFormatter = new Intl.NumberFormat(undefined, {
  minimumFractionDigits: 0,
  maximumFractionDigits: 2,
});

/** formatAverage renders per-request or ratio figures with at most two decimal places. */
export function formatAverage(v: number): string {
  if (Number.isNaN(v)) return "—";
  return averageFormatter.format(v);
}

export function formatContext(v: number): string {
  if (Number.isNaN(v) || v <= 0) return "—";
  if (v >= 1_000_000) return `${(v / 1_000_000).toFixed(1)}M`;
  if (v >= 1_000) return `${(v / 1_000).toFixed(1)}K`;
  return `${v}`;
}

// ai& floats prices as strings ("0.15"); keep the decimals exact for verdict math.
export function toNumber(v: string): number {
  const n = Number(v);
  return Number.isFinite(n) ? n : 0;
}