"use client";

import { Badge } from "@/components/atoms/Badge";
import { ModelSelectorPill, type ModelDescriptor } from "@/components/DashboardPageFilters/ModelSelectorPill";
import { useDashboardFilters } from "@/components/DashboardPageFilters";
import { Page } from "@/components/Page";
import { PageHeader } from "@/components/PageHeader";
import { Text } from "@/components/atoms/Text";
import { Card } from "@/components/molecules/Card";
import { api, type AiandModel, type ModelBreakdownBucket } from "@/lib/api";
import { toNumber, formatContext, formatUSD } from "@/lib/format";
import { tierForContextWindow, type ModelTier } from "@/lib/tier";
import { useEffect, useMemo, useState } from "react";

type SortKey = "popular" | "price_asc" | "price_desc" | "context_desc" | "newest";

const SORT_OPTIONS: { value: SortKey; label: string }[] = [
  { value: "popular", label: "Popular" },
  { value: "price_asc", label: "Price ↑" },
  { value: "price_desc", label: "Price ↓" },
  { value: "context_desc", label: "Context ↓" },
  { value: "newest", label: "Newest" },
];

const TIER_OPTIONS: ModelDescriptor[] = [
  { id: "low", label: "low" },
  { id: "mid", label: "mid" },
  { id: "high", label: "high" },
];

export default function ModelsPage() {
  const dashboardFilters = useDashboardFilters("30d");
  const { fromISO, toISO, granularity } = dashboardFilters.filters;

  const [catalog, setCatalog] = useState<AiandModel[] | null>(null);
  const [popularityBuckets, setPopularityBuckets] = useState<ModelBreakdownBucket[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [q, setQ] = useState("");
  const [caps, setCaps] = useState<string[]>([]);
  const [providers, setProviders] = useState<string[]>([]);
  const [tiers, setTiers] = useState<ModelTier[]>([]);
  const [sort, setSort] = useState<SortKey>("popular");

  useEffect(() => {
    api.aiandModels
      .list()
      .then(res => setCatalog(res.data ?? []))
      .catch((err: unknown) =>
        setError(err instanceof Error ? err.message : "Failed to load the ai& catalog."));
  }, []);

  useEffect(() => {
    api.metrics
      .modelBreakdown(granularity, fromISO, toISO)
      .then(res => setPopularityBuckets(res.buckets ?? []))
      .catch(() => setPopularityBuckets([]));
  }, [granularity, fromISO, toISO]);

  const rows = catalog ?? [];

  const modelDescriptors: ModelDescriptor[] = useMemo(
    () => rows.map(m => ({ id: m.id, label: m.id })),
    [rows],
  );

  const allCaps = useMemo(() => [...new Set(rows.flatMap(m => m.capabilities))].sort(), [rows]);
  const allProviders = useMemo(() => [...new Set(rows.map(m => m.provider))].sort(), [rows]);

  // sort/filter pipeline
  const shown = useMemo(() => {
    const needle = q.trim().toLowerCase();
    const filtered = rows.filter(m => {
      const tier = tierForContextWindow(m.context_window);
      if (needle && !m.id.toLowerCase().includes(needle)) return false;
      if (caps.length && !caps.every(c => m.capabilities.includes(c))) return false;
      if (providers.length && !providers.includes(m.provider)) return false;
      if (tiers.length && !tiers.includes(tier)) return false;
      return true;
    });
    const sorted = [...filtered].sort((a, b) => {
      switch (sort) {
        case "price_asc":
          return toNumber(a.input_per_1m) - toNumber(b.input_per_1m);
        case "price_desc":
          return toNumber(b.input_per_1m) - toNumber(a.input_per_1m);
        case "context_desc":
          return b.context_window - a.context_window;
        case "newest": {
          if (a.created == null && b.created == null) return 0;
          if (a.created == null) return 1;
          if (b.created == null) return -1;
          return b.created - a.created;
        }
        default:
          return 0; // "popular" rank comes from usage; see below
      }
    });
    if (sort === "popular") {
      const used = new Map<string, number>();
      for (const b of popularityBuckets) {
        const key = b.decision_model;
        if (!used.has(key)) used.set(key, 0);
        used.set(key, used.get(key)! + b.total_tokens);
      }
      sorted.sort(
        (a, b) =>
          (used.get(b.id) ?? 0) - (used.get(a.id) ?? 0) ||
          (used.get(b.id) == null ? 1 : 0) - (used.get(a.id) == null ? 1 : 0) ||
          a.id.localeCompare(b.id),
      );
    }
    return sorted;
  }, [rows, q, caps, providers, tiers, sort, popularityBuckets]);

  return (
    <Page
      header={
        <PageHeader
          left={
            <Text variant="h4" as="h2" className="whitespace-nowrap">
              Models
            </Text>
          }
        />
      }
    >
      <Page.Section>
        {error != null && (
          <div className="rounded-lg border border-danger/30 bg-danger/5 p-4 text-sm text-danger">
            {error}
          </div>
        )}
        <div className="relative flex flex-row flex-wrap items-start gap-4">
          <input
            value={q}
            onChange={e => setQ(e.target.value)}
            placeholder="Search models…"
            className="h-8 rounded-lg border border-border bg-card px-3 text-xs"
          />
          <ModelSelectorPill
            models={allCaps.map(c => ({ id: c, label: c }))}
            selected={caps}
            onToggle={id =>
              setCaps(prev => (prev.includes(id) ? prev.filter(x => x !== id) : [...prev, id]))}
          />
          <ModelSelectorPill
            models={allProviders.map(p => ({ id: p, label: p }))}
            selected={providers}
            onToggle={id =>
              setProviders(prev => (prev.includes(id) ? prev.filter(x => x !== id) : [...prev, id]))}
          />
          <ModelSelectorPill
            models={TIER_OPTIONS}
            selected={tiers}
            onToggle={id =>
              setTiers(prev =>
                prev.includes(id as ModelTier) ? prev.filter(x => x !== id) : [...prev, id as ModelTier])}
          />
          <select
            value={sort}
            onChange={e => setSort(e.target.value as SortKey)}
            className="h-8 rounded-lg border border-border bg-card px-2 text-xs"
          >
            {SORT_OPTIONS.map(o => (
              <option key={o.value} value={o.value}>
                {o.label}
              </option>
            ))}
          </select>
        </div>

        <Card>
          <Card.Content className="overflow-x-auto p-0">
            <table className="w-full text-xs">
              <thead>
                <tr className="text-left text-2xs uppercase tracking-wider text-muted-foreground">
                  <th className="px-4 py-2 font-medium">Model</th>
                  <th className="px-4 py-2 font-medium">Provider</th>
                  <th className="px-4 py-2 font-medium">Context</th>
                  <th className="px-4 py-2 font-medium">Capabilities</th>
                  <th className="px-4 py-2 text-right font-medium">Input/1M</th>
                  <th className="px-4 py-2 text-right font-medium">Output/1M</th>
                  <th className="px-4 py-2 text-right font-medium">Cached/1M</th>
                  <th className="px-4 py-2 text-right font-medium">Currency</th>
                </tr>
              </thead>
              <tbody>
                {shown.length === 0 ? (
                  <tr>
                    <td colSpan={8} className="px-4 py-8 text-center text-muted-foreground">
                      No models match these filters.
                    </td>
                  </tr>
                ) : (
                  shown.map(m => {
                    const tier = tierForContextWindow(m.context_window);
                    return (
                      <tr key={m.id} className="border-t border-border/50 hover:bg-foreground/5">
                        <td className="px-4 py-2">
                          <a
                            href={`/models/${m.id.replace(/\//g, "~")}`}
                            className="font-medium hover:text-primary"
                            title={m.id}
                          >
                            {m.id}
                          </a>
                          <Badge.Tier tier={tier} />
                        </td>
                        <td className="px-4 py-2 text-muted-foreground">{m.provider}</td>
                        <td className="px-4 py-2 tabular-nums">{formatContext(m.context_window)}</td>
                        <td className="px-4 py-2">
                          <span className="flex flex-wrap gap-1">
                            {m.capabilities.map(cap => (
                              <Badge.Capability key={cap} name={cap} />
                            ))}
                          </span>
                        </td>
                        <td className="px-4 py-2 text-right tabular-nums">{formatUSD(toNumber(m.input_per_1m) / 1e6 * 1)}</td>
                        <td className="px-4 py-2 text-right tabular-nums">{formatUSD(toNumber(m.output_per_1m) / 1e6 * 1)}</td>
                        <td className="px-4 py-2 text-right tabular-nums">{formatUSD(toNumber(m.cached_input_per_1m) / 1e6 * 1)}</td>
                        <td className="px-4 py-2 text-right uppercase">{m.currency}</td>
                      </tr>
                    );
                  })
                )}
              </tbody>
            </table>
          </Card.Content>
        </Card>
      </Page.Section>
    </Page>
  );
}