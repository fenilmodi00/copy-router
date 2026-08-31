import type { Cache } from "swr";

/**
 * localStorage persistence for the SWR cache, following the official
 * provider pattern (https://swr.vercel.app/docs/advanced/cache) with two
 * additions SWR does not ship: entry TTL and write throttling.
 *
 * - On boot, expired entries are dropped at restore time, so a refresh
 *   never paints data older than TTL.
 * - Writes are throttled and snapshot-based (serialize the whole Map), so
 *   there are no per-set write storms.
 * - On restore we keep only `data` and `error` — the transient flags
 *   (`isValidating`, `isLoading`, `_k`) are meaningless across reloads.
 */

const STORAGE_KEY = "weave-router.swr-cache";
const TTL_MS = 10 * 60_000;
const FLUSH_DELAY_MS = 5_000;

/** What one cache entry looks like on disk: TTL stamp plus the durable SWR state fields. */
interface PersistedEntry {
  t: number;
  data: unknown;
  error: unknown;
}

/** A single SWR cache entry in memory: SWR stores State objects, not raw data. */
type SwrState = NonNullable<ReturnType<Cache["get"]>>;

/**
 * Reads localStorage and rebuilds the cache Map. Entries older than TTL are
 * dropped; corrupt payloads degrade to an empty cache.
 */
function readPersisted(): Map<string, SwrState> {
  const out = new Map<string, SwrState>();
  if (typeof window === "undefined") return out;
  try {
    const raw = window.localStorage.getItem(STORAGE_KEY);
    if (!raw) return out;
    const parsed = JSON.parse(raw) as [string, PersistedEntry][];
    if (!Array.isArray(parsed)) return out;
    const now = Date.now();
    for (const [key, entry] of parsed) {
      if (typeof key !== "string") continue;
      if (entry == null || typeof entry.t !== "number" || now - entry.t >= TTL_MS) continue;
      out.set(key, { data: entry.data, error: entry.error });
    }
  } catch {
    // Corrupt payload or storage disabled — start empty.
  }
  return out;
}

/**
 * Builds the app's SWR cache provider: restores persisted entries into a
 * fresh Map, mirrors writes into it, and flushes a TTL-stamped snapshot to
 * localStorage on a throttled delay and before unload.
 */
export function persistedCacheProvider(): Cache {
  if (typeof window === "undefined") {
    return new Map<string, SwrState>() as unknown as Cache;
  }

  const map = readPersisted();
  let flushTimer: ReturnType<typeof setTimeout> | null = null;

  const flush = () => {
    flushTimer = null;
    try {
      const now = Date.now();
      const entries: [string, PersistedEntry][] = [];
      for (const [key, state] of map) {
        if (state == null) continue;
        entries.push([key, { t: now, data: state.data, error: state.error }]);
      }
      window.localStorage.setItem(STORAGE_KEY, JSON.stringify(entries));
    } catch {
      // Quota exceeded or storage disabled — persistence silently degrades.
    }
  };

  const scheduleFlush = () => {
    if (flushTimer != null) return;
    flushTimer = setTimeout(flush, FLUSH_DELAY_MS);
  };

  window.addEventListener("beforeunload", flush);

  // Map-like object satisfying SWR's Cache contract while notifying the
  // persistence layer on every write.
  const cache: Cache<Map<string, SwrState>> = {
    get: key => map.get(key),
    set: (key, value) => {
      map.set(key, value);
      scheduleFlush();
      return cache;
    },
    delete: key => {
      const had = map.delete(key);
      scheduleFlush();
      return had;
    },
    keys: () => map.keys(),
  };

  return cache;
}
