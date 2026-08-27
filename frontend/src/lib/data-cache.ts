import useSWR, { SWRConfiguration, mutate as swrMutate } from "swr";

import {
  api,
  type AiandModel,
  type APIKey,
  type ModelBreakdownBucket,
} from "@/lib/api";

type MetricsGranularity = "hour" | "day" | "week";

/** Shared SWR key factories — same key ⇒ same cache entry across remounts. */
export const queryKeys = {
  catalog: ["catalog"] as const,
  onboarding: ["onboarding"] as const,
  config: ["config"] as const,
  keys: ["keys"] as const,
  excludedModels: ["excluded-models"] as const,
  excludedProviders: ["excluded-providers"] as const,
  routingPreferences: ["routing-preferences"] as const,
  metricsSummary: (from: string, to: string) =>
    ["metrics", "summary", from, to] as const,
  metricsTimeseries: (granularity: MetricsGranularity, from: string, to: string) =>
    ["metrics", "timeseries", granularity, from, to] as const,
  metricsModelBreakdown: (granularity: MetricsGranularity, from: string, to: string) =>
    ["metrics", "model-breakdown", granularity, from, to] as const,
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

export function useMetricsSummary(fromISO: string, toISO: string, config?: SWRConfiguration) {
  return useSWR(
    queryKeys.metricsSummary(fromISO, toISO),
    () => api.metrics.summary(fromISO, toISO),
    { ...swrDefaults, ...config },
  );
}

export function useMetricsTimeseries(
  granularity: MetricsGranularity,
  fromISO: string,
  toISO: string,
  config?: SWRConfiguration,
) {
  return useSWR(
    queryKeys.metricsTimeseries(granularity, fromISO, toISO),
    () => api.metrics.timeseries(granularity, fromISO, toISO),
    { ...swrDefaults, ...config },
  );
}

export function useMetricsModelBreakdown(
  granularity: MetricsGranularity,
  fromISO: string,
  toISO: string,
  config?: SWRConfiguration,
) {
  return useSWR(
    queryKeys.metricsModelBreakdown(granularity, fromISO, toISO),
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

export function useExcludedProviders(config?: SWRConfiguration) {
  return useSWR(queryKeys.excludedProviders, () => api.excludedProviders.get(), {
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

export function invalidateKeys() {
  return swrMutate(queryKeys.keys);
}

export function invalidateExcludedModels() {
  return swrMutate(queryKeys.excludedModels);
}

export function invalidateExcludedProviders() {
  return swrMutate(queryKeys.excludedProviders);
}

export function invalidateRoutingPreferences() {
  return swrMutate(queryKeys.routingPreferences);
}

