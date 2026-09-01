import { useEffect } from "react";
import useSWR, { SWRConfiguration, useSWRConfig } from "swr";

import {
  api,
  type AiandModel,
  type APIKey,
  type ModelBreakdownBucket,
} from "@/lib/api";

type MetricsGranularity = "hour" | "day" | "week";

/**
 * Metric keys are quantized to 5-minute buckets instead of embedding the
 * exact from/to timestamps: two mounts of the same range within one bucket
 * share a cache entry (instant open on remount), while the fetcher still
 * receives exact timestamps so data stays fresh. The rolling window also
 * bounds staleness — after 5 minutes the key rolls over to a new entry.
 */
export const KEY_QUANTUM_MS = 5 * 60_000;

export function quantizeKeyTime(iso: string): number {
  const t = Date.parse(iso);
  if (Number.isNaN(t)) return 0;
  return Math.floor(t / KEY_QUANTUM_MS) * KEY_QUANTUM_MS;
}

/** Shared SWR key factories — same key ⇒ same cache entry across remounts. */
export const queryKeys = {
  catalog: ["catalog"] as const,
  onboarding: ["onboarding"] as const,
  config: ["config"] as const,
  keys: ["keys"] as const,
  excludedModels: ["excluded-models"] as const,
  routingPreferences: ["routing-preferences"] as const,
  metricsSummary: (rangeId: string, toISO: string) =>
    ["metrics", "summary", rangeId, quantizeKeyTime(toISO)] as const,
  metricsTimeseries: (granularity: MetricsGranularity, rangeId: string, toISO: string) =>
    ["metrics", "timeseries", granularity, rangeId, quantizeKeyTime(toISO)] as const,
  metricsModelBreakdown: (granularity: MetricsGranularity, rangeId: string, toISO: string) =>
    ["metrics", "model-breakdown", granularity, rangeId, quantizeKeyTime(toISO)] as const,
};

const swrDefaults: SWRConfiguration = {
  revalidateOnFocus: true,
  keepPreviousData: true,
  dedupingInterval: 2_000,
};

export interface ModelTotal {
  id: string;
  label: string;
  tokens: number;
  costUsd: number;
  requests: number;
}

/** Fold model-breakdown buckets into per-model totals (replaces details fan-in). */
export function aggregateModelTotals(buckets: ModelBreakdownBucket[]): ModelTotal[] {
  const byModel = new Map<string, { tokens: number; costUsd: number; requests: number }>();
  for (const b of buckets) {
    const key = b.decision_model || "(unknown)";
    const cur = byModel.get(key) ?? { tokens: 0, costUsd: 0, requests: 0 };
    cur.tokens += b.total_tokens;
    cur.costUsd += b.actual_cost_usd;
    cur.requests += b.request_count;
    byModel.set(key, cur);
  }
  return [...byModel.entries()]
    .map(([id, v]) => ({ id, label: id, ...v }))
    .sort((a, b) => b.tokens - a.tokens);
}

export type ModelWindow = "24h" | "7d" | "30d";

export interface WindowStat {
  requests: number;
  tokens: number;
  cost: number;
}

const WINDOW_MS: Record<ModelWindow, number> = {
  "24h": 24 * 3600_000,
  "7d": 7 * 24 * 3600_000,
  "30d": 30 * 24 * 3600_000,
};

/** Slice one 30d day-breakdown into 24h / 7d / 30d totals for a CatalogModelID. */
export function modelWindowStats(
  buckets: ModelBreakdownBucket[],
  modelId: string,
  now: Date = new Date(),
): Record<ModelWindow, WindowStat> {
  const empty = (): WindowStat => ({ requests: 0, tokens: 0, cost: 0 });
  const out: Record<ModelWindow, WindowStat> = {
    "24h": empty(),
    "7d": empty(),
    "30d": empty(),
  };
  const nowMs = now.getTime();
  for (const b of buckets) {
    if (b.decision_model !== modelId) continue;
    const t = Date.parse(b.bucket);
    if (Number.isNaN(t)) continue;
    const age = nowMs - t;
    for (const w of Object.keys(WINDOW_MS) as ModelWindow[]) {
      if (age >= 0 && age <= WINDOW_MS[w]) {
        out[w].requests += b.request_count;
        out[w].tokens += b.total_tokens;
        out[w].cost += b.actual_cost_usd;
      }
    }
  }
  return out;
}

export function useCatalog(config?: SWRConfiguration) {
  return useSWR(
    queryKeys.catalog,
    async () => {
      const res = await api.aiandModels.list();
      return (res.data ?? []) as AiandModel[];
    },
    { ...swrDefaults, ...config },
  );
}

export function useOnboarding(config?: SWRConfiguration) {
  return useSWR(queryKeys.onboarding, () => api.onboarding.get(), {
    ...swrDefaults,
    ...config,
  });
}
export function useMetricsSummary(
  rangeId: string,
  fromISO: string,
  toISO: string,
  config?: SWRConfiguration,
) {
  return useSWR(
    queryKeys.metricsSummary(rangeId, toISO),
    () => api.metrics.summary(fromISO, toISO),
    { ...swrDefaults, ...config },
  );
}
export function useMetricsTimeseries(
  granularity: MetricsGranularity,
  rangeId: string,
  fromISO: string,
  toISO: string,
  config?: SWRConfiguration,
) {
  return useSWR(
    queryKeys.metricsTimeseries(granularity, rangeId, toISO),
    () => api.metrics.timeseries(granularity, fromISO, toISO),
    { ...swrDefaults, ...config },
  );
}

export function useMetricsModelBreakdown(
  granularity: MetricsGranularity,
  rangeId: string,
  fromISO: string,
  toISO: string,
  config?: SWRConfiguration,
) {
  return useSWR(
    queryKeys.metricsModelBreakdown(granularity, rangeId, toISO),
    () => api.metrics.modelBreakdown(granularity, fromISO, toISO),
    { ...swrDefaults, ...config },
  );
}

export function useConfig(config?: SWRConfiguration) {
  return useSWR(queryKeys.config, () => api.config.get(), { ...swrDefaults, ...config });
}

export function useKeys(config?: SWRConfiguration) {
  return useSWR(
    queryKeys.keys,
    async () => {
      const r = await api.keys.list();
      return (r.keys ?? []) as APIKey[];
    },
    { ...swrDefaults, ...config },
  );
}

export function useExcludedModels(config?: SWRConfiguration) {
  return useSWR(queryKeys.excludedModels, () => api.excludedModels.get(), {
    ...swrDefaults,
    ...config,
  });
}

export function useRoutingPreferences(config?: SWRConfiguration) {
  return useSWR(queryKeys.routingPreferences, () => api.routingPreferences.get(), {
    ...swrDefaults,
    ...config,
  });
}

/**
 * Refetches the key list after a mutation. Must be called from a component
 * tree under DataCacheProvider — uses `useSWRConfig().mutate`, which targets
 * the app's configured cache provider. The global `mutate` export only
 * reaches SWR's default Map and would silently miss hooks under our
 * custom provider.
 */
export function useInvalidateKeys() {
  const { mutate } = useSWRConfig();
  return () => mutate(queryKeys.keys);
}

/** See useInvalidateKeys. */
export function useInvalidateExcludedModels() {
  const { mutate } = useSWRConfig();
  return () => mutate(queryKeys.excludedModels);
}

/** See useInvalidateKeys. */
export function useInvalidateRoutingPreferences() {
  const { mutate } = useSWRConfig();
  return () => mutate(queryKeys.routingPreferences);
}

/**
 * Warms caches for routes the user is likely to open next (Models, Settings,
 * Settings/Routing). Runs once, after first paint, via requestIdleCallback
 * (setTimeout fallback for jsdom). Uses the provider-scoped `mutate` with
 * `revalidate: true`, so a later `useSWR` on the same key finds warm data
 * and still refreshes in the background.
 */
export function useBackgroundWarm() {
  const { mutate } = useSWRConfig();

  useEffect(() => {
    let cancelled = false;
    const warm = () => {
      if (cancelled) return;
      void Promise.all([
        mutate(
          queryKeys.catalog,
          async () => ((await api.aiandModels.list()).data ?? []) as AiandModel[],
          { revalidate: true },
        ),
        mutate(
          queryKeys.config,
          async () => api.config.get(),
          { revalidate: true },
        ),
        mutate(
          queryKeys.keys,
          async () => ((await api.keys.list()).keys ?? []) as APIKey[],
          { revalidate: true },
        ),
      ]).catch(() => {
        // Best-effort warming; failures surface when the page mounts its own hook.
      });
    };

    if (typeof window !== "undefined" && "requestIdleCallback" in window) {
      const id = window.requestIdleCallback(warm);
      return () => {
        cancelled = true;
        window.cancelIdleCallback(id);
      };
    }
    const id = setTimeout(warm, 50);
    return () => {
      cancelled = true;
      clearTimeout(id);
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps -- mutate is stable per provider
  }, [mutate]);
}
