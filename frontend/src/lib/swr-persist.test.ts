import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { persistedCacheProvider } from "@/lib/swr-persist";

const STORAGE_KEY = "weave-router.swr-cache";

function seed(entries: unknown[]) {
  window.localStorage.setItem(STORAGE_KEY, JSON.stringify(entries));
}

function stored(): [string, { t: number; data: unknown; error: unknown }][] {
  return JSON.parse(
    window.localStorage.getItem(STORAGE_KEY) ?? "[]",
  ) as [string, { t: number; data: unknown; error: unknown }][];
}

describe("persistedCacheProvider", () => {
  beforeEach(() => {
    window.localStorage.clear();
    vi.useFakeTimers();
  });

  afterEach(() => {
    vi.useRealTimers();
    window.localStorage.clear();
  });

  it("restores unexpired entries so a remount reads cached data instantly", () => {
    const t = Date.now();
    seed([["@/catalog", { t, data: [{ id: "m1" }], error: undefined }]]);

    const cache = persistedCacheProvider();
    const state = cache.get("@/catalog") as { data?: { id: string }[] } | undefined;

    expect(state?.data).toEqual([{ id: "m1" }]);
  });

  it("drops entries older than the TTL so a refresh never paints stale data", () => {
    // 15 minutes old — beyond the 10-minute TTL.
    const old = Date.now() - 15 * 60_000;
    seed([["@/catalog", { t: old, data: [{ id: "stale" }], error: undefined }]]);

    const cache = persistedCacheProvider();

    expect(cache.get("@/catalog")).toBeUndefined();
  });

  it("writes through to localStorage after a throttled delay, not per set", () => {
    const cache = persistedCacheProvider();

    cache.set("@/catalog", { data: [{ id: "m1" }] });
    cache.set("@/config", { data: { cluster_version: "v0.78" } });
    // Before the flush delay nothing is on disk.
    expect(window.localStorage.getItem(STORAGE_KEY)).toBeNull();

    vi.advanceTimersByTime(5_000);

    const entries = stored();
    expect(entries).toHaveLength(2);
    const byKey = new Map(entries);
    expect((byKey.get("@/catalog")?.data as { id: string }[])[0]?.id).toBe("m1");
    expect((byKey.get("@/config")?.data as { cluster_version: string }).cluster_version).toBe(
      "v0.78",
    );
  });

  it("persists only durable state fields, never transient validation flags", () => {
    const cache = persistedCacheProvider();

    cache.set("@/catalog", {
      data: [{ id: "m1" }],
      isValidating: true,
      isLoading: true,
      // SWR's internal original-key metadata, dropped on persist.
      ...{ _k: "internal" },
    } as never);
    vi.advanceTimersByTime(5_000);
    const entry = stored()[0]![1];
    expect(entry.data).toEqual([{ id: "m1" }]);
    // Transient flags and internal metadata never reach disk.
    expect(entry).not.toHaveProperty("isValidating");
    expect(entry).not.toHaveProperty("isLoading");
    expect(entry).not.toHaveProperty("_k");
  });

  it("degrades to an empty cache on a corrupt payload", () => {
    window.localStorage.setItem(STORAGE_KEY, "{{{not json");

    const cache = persistedCacheProvider();

    expect(cache.get("@/catalog")).toBeUndefined();
    expect(Array.from(cache.keys())).toEqual([]);
  });
});
