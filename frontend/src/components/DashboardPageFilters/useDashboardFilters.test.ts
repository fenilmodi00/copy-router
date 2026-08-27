import { renderHook, act } from "@testing-library/react";
import { describe, expect, it, vi, afterEach } from "vitest";

import {
  DATE_RANGES,
  DEFAULT_RANGE_ID,
  useDashboardFilters,
} from "./useDashboardFilters";

describe("useDashboardFilters defaults", () => {
  afterEach(() => {
    vi.useRealTimers();
  });

  it("defaults to last-24h with hour granularity so KPI sparklines get hourly buckets", () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date("2026-08-27T15:30:00.000Z"));

    expect(DEFAULT_RANGE_ID).toBe("last-24h");
    const range = DATE_RANGES.find(r => r.id === "last-24h");
    expect(range?.defaultGranularity).toBe("hour");

    const { result } = renderHook(() => useDashboardFilters());
    expect(result.current.filters.range.id).toBe("last-24h");
    expect(result.current.filters.granularity).toBe("hour");

    const from = new Date(result.current.filters.fromISO);
    const to = new Date(result.current.filters.toISO);
    const spanMs = to.getTime() - from.getTime();
    // ~24h window ending at "now" — not a calendar day bucket that flattens
    // sparse same-day usage into a single point.
    expect(spanMs).toBeGreaterThanOrEqual(23 * 3600_000);
    expect(spanMs).toBeLessThanOrEqual(25 * 3600_000);
  });

  it("resets granularity to the range default when the range changes", () => {
    const { result } = renderHook(() => useDashboardFilters());
    act(() => result.current.setGranularity("day"));
    expect(result.current.filters.granularity).toBe("day");

    act(() => result.current.setRangeId("last-24h"));
    expect(result.current.filters.granularity).toBe("hour");
  });
});
