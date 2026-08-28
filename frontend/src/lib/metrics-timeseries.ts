import type { Granularity } from "@/components/DashboardPageFilters/useDashboardFilters";
import type { TimeseriesBucket } from "@/lib/api";

const EMPTY_BUCKET: Omit<TimeseriesBucket, "bucket"> = {
  requested_cost_usd: 0,
  actual_cost_usd: 0,
  total_tokens: 0,
  request_count: 0,
};

function bucketStepMs(granularity: Granularity): number {
  switch (granularity) {
    case "week":
      return 7 * 24 * 3600_000;
    case "day":
      return 24 * 3600_000;
    default:
      return 3600_000;
  }
}

/** Align `d` to the start of its metrics bucket in UTC (matches Postgres date_trunc). */
export function truncateToBucketUTC(d: Date, granularity: Granularity): Date {
  const out = new Date(
    Date.UTC(d.getUTCFullYear(), d.getUTCMonth(), d.getUTCDate(), d.getUTCHours(), 0, 0, 0),
  );
  if (granularity === "day") {
    out.setUTCHours(0, 0, 0, 0);
  } else if (granularity === "week") {
    out.setUTCHours(0, 0, 0, 0);
    const day = (out.getUTCDay() + 6) % 7;
    out.setUTCDate(out.getUTCDate() - day);
  }
  return out;
}

/**
 * Expands sparse API buckets into a contiguous UTC series for the selected
 * window. Postgres only emits rows for buckets with activity; KPI sparklines
 * need the zero-filled hours/days in between or the polyline collapses flat.
 */
export function denseTimeseriesBuckets(
  sparse: TimeseriesBucket[],
  granularity: Granularity,
  fromISO: string,
  toISO: string,
): TimeseriesBucket[] {
  const step = bucketStepMs(granularity);
  const fromMs = truncateToBucketUTC(new Date(fromISO), granularity).getTime();
  const toMs = truncateToBucketUTC(new Date(toISO), granularity).getTime();
  if (!Number.isFinite(fromMs) || !Number.isFinite(toMs) || fromMs > toMs) {
    return sparse;
  }

  const byBucket = new Map<number, TimeseriesBucket>();
  for (const b of sparse) {
    const t = truncateToBucketUTC(new Date(b.bucket), granularity).getTime();
    const prev = byBucket.get(t);
    if (prev == null) {
      byBucket.set(t, b);
      continue;
    }
    byBucket.set(t, {
      bucket: prev.bucket,
      requested_cost_usd: prev.requested_cost_usd + b.requested_cost_usd,
      actual_cost_usd: prev.actual_cost_usd + b.actual_cost_usd,
      total_tokens: (prev.total_tokens ?? 0) + (b.total_tokens ?? 0),
      request_count: (prev.request_count ?? 0) + (b.request_count ?? 0),
    });
  }

  const out: TimeseriesBucket[] = [];
  for (let t = fromMs; t <= toMs; t += step) {
    const hit = byBucket.get(t);
    out.push(
      hit ?? {
        bucket: new Date(t).toISOString(),
        ...EMPTY_BUCKET,
      },
    );
  }
  return out.length > 0 ? out : sparse;
}
