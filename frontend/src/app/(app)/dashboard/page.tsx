"use client";

import { Text } from "@/components/atoms/Text";
import { ChartCard } from "@/components/ChartCard";
import { PopularityLeaderboard } from "@/components/charts/PopularityLeaderboard";
import {
  DashboardPageFilters,
  useDashboardFilters,
} from "@/components/DashboardPageFilters";
import { Card } from "@/components/molecules/Card";
import { Sparkline } from "@/components/molecules/Sparkline";
import { Page } from "@/components/Page";
import { PageHeader } from "@/components/PageHeader";
import { ResponsiveGrid } from "@/components/ResponsiveGrid";
import { RouterOnboarding } from "@/components/RouterOnboarding";
import {
  api,
  type AiandModel,
  type MetricsDetailRow,
  type MetricsSummary,
  type ModelBreakdownBucket,
  type TimeseriesBucket,
} from "@/lib/api";
import { cn } from "@/lib/cn";
import { formatNumber, formatUSD } from "@/lib/format";
import { useRouter } from "next/navigation";
import { useEffect, useMemo, useState } from "react";

// "checking" suppresses a flash of either surface until the onboarding probe
// lands.
type OnboardingState = "checking" | "needed" | "done";

// Set when the user chooses "Skip to dashboard". Persisted rather than held in
// memory so a refresh doesn't drop them back into a flow they opted out of;
// the server-side flag still takes over for good once a request is served.
const SKIP_ONBOARDING_KEY = "weave-router.onboarding-skipped";

function onboardingSkipped(): boolean {
  if (typeof window === "undefined") return false;
  try {
    return window.localStorage.getItem(SKIP_ONBOARDING_KEY) === "true";
  } catch {
    // Private-mode/blocked storage: treat as not skipped rather than throwing
    // on the render path.
    return false;
  }
}

function rememberOnboardingSkipped() {
  try {
    window.localStorage.setItem(SKIP_ONBOARDING_KEY, "true");
  } catch {
    // Non-fatal: the skip just won't survive this reload.
  }
}

export default function DashboardPage() {
  const dashboardFilters = useDashboardFilters("30d");
  const { fromISO, toISO, granularity, range } = dashboardFilters.filters;
  const router = useRouter();

  const [summary, setSummary] = useState<MetricsSummary | null>(null);
  const [buckets, setBuckets] = useState<TimeseriesBucket[]>([]);
  const [modelBuckets, setModelBuckets] = useState<ModelBreakdownBucket[]>([]);
  const [detailRows, setDetailRows] = useState<MetricsDetailRow[]>([]);
  const [catalog, setCatalog] = useState<AiandModel[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [onboarding, setOnboarding] = useState<OnboardingState>("checking");

  // A router that has never served a request has nothing to chart, so a fresh
  // install lands in onboarding instead of on six empty charts. The gate is the
  // installation-level first_request_served_at, not a key's last_used_at:
  // that flag survives rotation, so rotating the key that served the first
  // request can't send an established install back through onboarding.
  useEffect(() => {
    let cancelled = false;
    api.onboarding
      .get()
      .then(res => {
        if (cancelled) return;
        const served = res.first_request_served_at != null;
        setOnboarding(served || onboardingSkipped() ? "done" : "needed");
      })
      // Non-fatal: on a failed probe show the dashboard rather than trapping
      // an established install in onboarding.
      .catch(() => {
        if (!cancelled) setOnboarding("done");
      });
    return () => {
      cancelled = true;
    };
  }, []);

  useEffect(() => {
    if (onboarding !== "done") return;
    let cancelled = false;
    setError(null);
    Promise.all([
      api.metrics.summary(fromISO, toISO),
      api.metrics.timeseries(granularity, fromISO, toISO),
      api.metrics.modelBreakdown(granularity, fromISO, toISO),
      api.metrics.details(fromISO, toISO, 1000),
      api.aiandModels.list(),
    ])
      .then(([s, ts, mb, det, catalog]) => {
        if (cancelled) return;
        setSummary(s);
        setBuckets(ts.buckets ?? []);
        setModelBuckets(mb.buckets ?? []);
        setDetailRows(det.rows ?? []);
        setCatalog(catalog.data ?? []);
      })
      .catch((err: unknown) => {
        if (cancelled) return;
        setError(err instanceof Error ? err.message : "Failed to load metrics.");
      });
    return () => {
      cancelled = true;
    };
  }, [fromISO, toISO, granularity, onboarding]);

  if (onboarding === "checking") return null;
  if (onboarding === "needed") {
    return (
      <RouterOnboarding
        onComplete={() => setOnboarding("done")}
        onSkip={() => {
          rememberOnboardingSkipped();
          setOnboarding("done");
        }}
      />
    );
  }

  if (error) {
    return (
      <Page
        header={
          <PageHeader
            left={
              <Text
                variant="h4"
                as="h2"
                className="flex flex-row items-center gap-1 whitespace-nowrap"
              >
                Overview
              </Text>
            }
          />
        }
      >
        <Page.Section>
          <div className="rounded-lg border border-danger/30 bg-danger/5 p-4 text-sm text-danger">
            {error}
          </div>
        </Page.Section>
      </Page>
    );
  }

  const savingsRate =
    summary == null || summary.total_requested_cost_usd === 0
      ? 0
      : (summary.total_savings_usd / summary.total_requested_cost_usd) * 100;
  const avgTokensPerReq =
    summary == null || summary.request_count === 0
      ? 0
      : summary.total_tokens / summary.request_count;
  const cacheReadTokens = summary?.cache_read_tokens ?? 0;
  const cacheWriteTokens = summary?.cache_write_tokens ?? 0;
  const totalInputTokens = detailRows.reduce((acc, r) => acc + r.input_tokens, 0);
  const cacheHitRate =
    totalInputTokens > 0 ? (cacheReadTokens / totalInputTokens) * 100 : null;

  // Per-model totals in the selected range (grouped from detail rows — the
  // telemetry details API is the cheapest server-side per-row source we already
  // have; the model-breakdown buckets give per-bucket, not totals).
  const modelTotals = useMemo(() => {
    const byModel = new Map<string, { tokens: number; costUsd: number; requests: number }>();
    for (const r of detailRows) {
      const key = r.decision_model || "(unknown)";
      const cur = byModel.get(key) ?? { tokens: 0, costUsd: 0, requests: 0 };
      cur.tokens += r.input_tokens + r.output_tokens;
      cur.costUsd += r.actual_cost_usd;
      cur.requests += 1;
      byModel.set(key, cur);
    }
    return [...byModel.entries()]
      .map(([id, v]) => ({ id, label: id, ...v }))
      .sort((a, b) => b.tokens - a.tokens);
  }, [detailRows]);

  return (
    <Page
      header={
        <PageHeader
          left={
            <Text
              variant="h4"
              as="h2"
              className="flex flex-row items-center gap-1 whitespace-nowrap"
            >
              Overview
            </Text>
          }
        />
      }
    >
      <Page.Section>
        <DashboardPageFilters result={dashboardFilters} />
        <ResponsiveGrid>
          <MetricCard
            className={ResponsiveGrid.Small}
            label="Tokens"
            value={summary == null ? "—" : formatNumber(summary.total_tokens)}
            sub={summary == null ? undefined : `${formatNumber(avgTokensPerReq)} avg / req`}
            sparkline={buckets.length ? buckets.map(b => b.actual_cost_usd) : []}
          />
          <MetricCard
            className={ResponsiveGrid.Small}
            label="Requests"
            value={summary == null ? "—" : formatNumber(summary.request_count)}
            sub={summary == null ? undefined : `actual ${formatUSD(summary.total_actual_cost_usd)}`}
            sparkline={buckets.map(b => b.requested_cost_usd)}
          />
          <MetricCard
            className={ResponsiveGrid.Small}
            label="Actual cost"
            value={summary == null ? "—" : formatUSD(summary.total_actual_cost_usd)}
            sub={
              summary == null
                ? undefined
                : `${formatUSD(Math.abs(summary.total_savings_usd))} saved vs requested`
            }
            sparkline={buckets.map(b => b.actual_cost_usd)}
          />
          <MetricCard
            className={ResponsiveGrid.Small}
            label="Cache hit rate"
            value={cacheHitRate == null ? "—%" : `${cacheHitRate.toFixed(1)}%`}
            sub={cacheWriteTokens + cacheReadTokens === 0 ? "no cached usage yet" : "write+read tokens"}
          />
        </ResponsiveGrid>

        <ResponsiveGrid>
          <ChartCard
            className={ResponsiveGrid.Medium}
            title="Popularity"
            subtitle="Top models by tokens processed on this install."
          >
            <PopularityLeaderboard
              rows={modelTotals.map(t => ({ id: t.id, label: t.label, tokens: t.tokens, costUsd: t.costUsd }))}
              limit={5}
              onSelect={id => router.push(`/models/${encodeURIComponent(id)}`)}
            />
          </ChartCard>

          <ChartCard
            className={ResponsiveGrid.Medium}
            title="Top models by spend"
            subtitle="Who's eating the actual-cost budget in the selected range."
          >
            <table className="w-full text-xs">
              <thead>
                <tr className="text-left text-2xs uppercase tracking-wider text-muted-foreground">
                  <th className="py-1 pr-2 font-medium">Model</th>
                  <th className="py-1 pr-2 text-right font-medium">Tokens</th>
                  <th className="py-1 text-right font-medium">Spend</th>
                </tr>
              </thead>
              <tbody>
                {modelTotals
                  .slice()
                  .sort((a, b) => b.costUsd - a.costUsd)
                  .slice(0, 8)
                  .map(r => (
                    <tr key={r.id} className="border-t border-border/50">
                      <td className="py-1.5 pr-2">
                        <a href={`/models/${encodeURIComponent(r.id)}`} className="hover:text-primary">
                          {r.label}
                        </a>
                      </td>
                      <td className="py-1.5 pr-2 text-right tabular-nums">{formatNumber(r.tokens)}</td>
                      <td className="py-1.5 text-right tabular-nums">{formatUSD(r.costUsd)}</td>
                    </tr>
                  ))}
              </tbody>
            </table>
          </ChartCard>
        </ResponsiveGrid>
      </Page.Section>
    </Page>
  );
}

interface MetricCardProps {
  className?: string;
  label: string;
  value: string;
  sub?: string;
  accent?: "default" | "success" | "danger" | "info";
  sparkline?: number[];
}

function MetricCard({ className, label, value, sub, accent = "default", sparkline }: MetricCardProps) {
  const accentClass =
    accent === "success"
      ? "text-success"
      : accent === "danger"
        ? "text-danger"
        : accent === "info"
          ? "text-primary"
          : "text-foreground";

  return (
    <Card size="sm" className={className}>
      <Card.Header>
        <Text className="text-2xs font-medium uppercase tracking-wider text-muted-foreground">
          {label}
        </Text>
      </Card.Header>
      <Card.Content>
        <div className="flex items-end justify-between gap-2">
          <Text
            className={cn(
              "font-display text-2xl font-semibold tabular-nums tracking-tight",
              accentClass,
            )}
          >
            {value}
          </Text>
          {sparkline != null && sparkline.length > 0 && <Sparkline data={sparkline} />}
        </div>
        {sub != null && <Text className="mt-1 text-2xs text-muted-foreground">{sub}</Text>}
      </Card.Content>
    </Card>
  );
}