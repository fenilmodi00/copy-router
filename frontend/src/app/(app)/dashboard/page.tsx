"use client";

import { Text } from "@/components/atoms/Text";
import { Skeleton } from "@/components/atoms/Skeleton";
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
  aggregateModelTotals,
  useCatalog,
  useMetricsModelBreakdown,
  useMetricsSummary,
  useMetricsTimeseries,
  useOnboarding,
} from "@/lib/data-cache";
import { cn } from "@/lib/cn";
import { formatNumber, formatUSD } from "@/lib/format";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useMemo, useState } from "react";

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

function resolveOnboarding(
  data: { first_request_served_at: string | null } | undefined,
  error: unknown,
  isLoading: boolean,
): OnboardingState {
  if (error) return "done";
  if (isLoading || data == null) return "checking";
  const served = data.first_request_served_at != null;
  return served || onboardingSkipped() ? "done" : "needed";
}

export default function DashboardPage() {
  const dashboardFilters = useDashboardFilters();
  const { fromISO, toISO, granularity } = dashboardFilters.filters;
  const router = useRouter();
  const [skipOverride, setSkipOverride] = useState(false);

  // Onboarding + metrics run in parallel (safe): a needed-onboarding install
  // still gets a cheap empty metrics response; an established one paints KPIs
  // without waiting on the onboarding round-trip.
  const onboardingQ = useOnboarding();
  const summaryQ = useMetricsSummary(fromISO, toISO);
  const timeseriesQ = useMetricsTimeseries(granularity, fromISO, toISO);
  const breakdownQ = useMetricsModelBreakdown(granularity, fromISO, toISO);
  // Catalog shares the singleton key with Models / Compare / detail.
  useCatalog();

  const onboarding: OnboardingState = skipOverride
    ? "done"
    : resolveOnboarding(onboardingQ.data, onboardingQ.error, onboardingQ.isLoading);

  const summary = summaryQ.data ?? null;
  const buckets = timeseriesQ.data?.buckets ?? [];
  const modelBuckets = breakdownQ.data?.buckets ?? [];

  const metricsError =
    summaryQ.error ?? timeseriesQ.error ?? breakdownQ.error ?? null;
  const metricsLoading =
    onboarding === "done" &&
    (summaryQ.isLoading || timeseriesQ.isLoading || breakdownQ.isLoading) &&
    summary == null;

  const avgTokensPerReq =
    summary == null || summary.request_count === 0
      ? 0
      : summary.total_tokens / summary.request_count;
  const cacheReadTokens = summary?.cache_read_tokens ?? 0;
  const cacheWriteTokens = summary?.cache_write_tokens ?? 0;
  // Without a 1000-row details fetch, approximate hit mix from cache token
  // counters on the summary (read / read+write).
  const cacheHitRate =
    cacheReadTokens + cacheWriteTokens > 0
      ? (cacheReadTokens / (cacheReadTokens + cacheWriteTokens)) * 100
      : null;

  const modelTotals = useMemo(
    () => aggregateModelTotals(modelBuckets),
    [modelBuckets],
  );

  if (onboarding === "checking") {
    return <DashboardSkeleton />;
  }
  if (onboarding === "needed") {
    return (
      <RouterOnboarding
        onComplete={() => setSkipOverride(true)}
        onSkip={() => {
          rememberOnboardingSkipped();
          setSkipOverride(true);
        }}
      />
    );
  }

  if (metricsError && summary == null) {
    const message =
      metricsError instanceof Error ? metricsError.message : "Failed to load metrics.";
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
            {message}
          </div>
        </Page.Section>
      </Page>
    );
  }

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
        {metricsLoading ? (
          <DashboardMetricsSkeleton />
        ) : (
          <>
            <ResponsiveGrid>
              <MetricCard
                className={ResponsiveGrid.Small}
                label="Tokens"
                value={summary == null ? "—" : formatNumber(summary.total_tokens)}
                sub={summary == null ? undefined : `${formatNumber(avgTokensPerReq)} avg / req`}
                sparkline={buckets.length ? buckets.map(b => b.total_tokens ?? 0) : []}
              />
              <MetricCard
                className={ResponsiveGrid.Small}
                label="Requests"
                value={summary == null ? "—" : formatNumber(summary.request_count)}
                sub={
                  summary == null
                    ? undefined
                    : `actual ${formatUSD(summary.total_actual_cost_usd)}`
                }
                sparkline={buckets.length ? buckets.map(b => b.request_count ?? 0) : []}
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
                sub={
                  cacheWriteTokens + cacheReadTokens === 0
                    ? "no cached usage yet"
                    : "write+read tokens"
                }
              />
            </ResponsiveGrid>

            <ResponsiveGrid>
              <ChartCard
                className={ResponsiveGrid.Medium}
                title="Popularity"
                subtitle="Top models by tokens processed on this install."
              >
                <PopularityLeaderboard
                  rows={modelTotals.map(t => ({
                    id: t.id,
                    label: t.label,
                    tokens: t.tokens,
                    costUsd: t.costUsd,
                  }))}
                  limit={5}
                  onSelect={id => router.push(`/models/${id.replace(/\//g, "~")}`)}
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
                            <Link
                              href={`/models/${r.id.replace(/\//g, "~")}`}
                              className="hover:text-primary"
                            >
                              {r.label}
                            </Link>
                          </td>
                          <td className="py-1.5 pr-2 text-right tabular-nums">
                            {formatNumber(r.tokens)}
                          </td>
                          <td className="py-1.5 text-right tabular-nums">
                            {formatUSD(r.costUsd)}
                          </td>
                        </tr>
                      ))}
                  </tbody>
                </table>
              </ChartCard>
            </ResponsiveGrid>
          </>
        )}
      </Page.Section>
    </Page>
  );
}

function DashboardSkeleton() {
  return (
    <Page
      header={
        <PageHeader
          left={
            <Text variant="h4" as="h2">
              Overview
            </Text>
          }
        />
      }
    >
      <Page.Section>
        <DashboardMetricsSkeleton />
      </Page.Section>
    </Page>
  );
}

function DashboardMetricsSkeleton() {
  return (
    <>
      <ResponsiveGrid>
        <Card.Loading className={ResponsiveGrid.Small} />
        <Card.Loading className={ResponsiveGrid.Small} />
        <Card.Loading className={ResponsiveGrid.Small} />
        <Card.Loading className={ResponsiveGrid.Small} />
      </ResponsiveGrid>
      <ResponsiveGrid>
        <Card.Loading className={ResponsiveGrid.Medium} />
        <Card.Loading className={ResponsiveGrid.Medium} />
      </ResponsiveGrid>
    </>
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

function MetricCard({
  className,
  label,
  value,
  sub,
  accent = "default",
  sparkline,
}: MetricCardProps) {
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
