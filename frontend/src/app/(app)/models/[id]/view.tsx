"use client";

import { Badge } from "@/components/atoms/Badge";
import { MetricToggle } from "@/components/atoms/MetricToggle";
import { Skeleton } from "@/components/atoms/Skeleton";
import { Card } from "@/components/molecules/Card";
import { Text } from "@/components/atoms/Text";
import { Tooltip } from "@/components/molecules/Tooltip";
import { ChartCard } from "@/components/ChartCard";
import { Page } from "@/components/Page";
import { PageHeader } from "@/components/PageHeader";
import { ResponsiveGrid } from "@/components/ResponsiveGrid";
import { ModelBreakdownChart, ModelBreakdownMetric } from "@/components/charts/ModelBreakdownChart";
import { useDashboardFilters } from "@/components/DashboardPageFilters";
import {
  modelWindowStats,
  useCatalog,
  useMetricsModelBreakdown,
  type ModelWindow,
} from "@/lib/data-cache";
import { formatContext, formatNumber, formatUSD, toNumber } from "@/lib/format";
import { tierForContextWindow } from "@/lib/tier";
import { useCompareBasket } from "@/lib/compare-basket-store";
import { usePathname, useParams } from "next/navigation";
import Link from "next/link";
import { useMemo, useState } from "react";

function normalizeId(id: string): string {
  return id.replace(/~/g, "/");
}

export default function ModelDetailView(props: { params?: { id?: string } }) {
  const params = useParams<{ id?: string }>();
  const pathname = usePathname();

  const rawId = props?.params?.id ?? params.id;
  const finalId =
    rawId && rawId !== "__none__"
      ? normalizeId(rawId)
      : normalizeId(decodeURIComponent(pathname.split("/").filter(Boolean).pop() ?? ""));

  return <ModelDetailPage id={finalId} key={finalId} />;
}

function ModelDetailPage({ id }: { id: string }) {
  const dashboardFilters = useDashboardFilters();
  const { fromISO, toISO, granularity } = dashboardFilters.filters;

  const catalogQ = useCatalog();
  const chartQ = useMetricsModelBreakdown(granularity, fromISO, toISO);

  // One 30d hour-breakdown fans into all three MiniStat windows (was 3× fetches).
  // Hour buckets keep the 24h column honest; day granularity would flatten
  // same-calendar-day usage into a single point.
  const windowTo = useMemo(() => new Date(), []);
  const windowFromISO = useMemo(
    () => new Date(windowTo.getTime() - 30 * 24 * 3600_000).toISOString(),
    [windowTo],
  );
  const windowsQ = useMetricsModelBreakdown("hour", windowFromISO, windowTo.toISOString());

  const [metric, setMetric] = useState<ModelBreakdownMetric>("requests");
  const basket = useCompareBasket();

  const all = catalogQ.data ?? [];
  const model = all.find(m => m.id === id) ?? null;
  const catalogError =
    catalogQ.error instanceof Error
      ? catalogQ.error.message
      : catalogQ.error
        ? "Catalog unavailable."
        : null;
  const missing =
    !catalogQ.isLoading && catalogQ.data != null && model == null
      ? `Model "${id}" is not in the live ai& catalog.`
      : null;
  const metricsError =
    chartQ.error instanceof Error
      ? chartQ.error.message
      : chartQ.error
        ? "Metrics unavailable."
        : null;
  const error = catalogError ?? missing ?? metricsError;

  const buckets =
    chartQ.data?.buckets?.filter(b => b.decision_model === id) ?? [];

  const windowStats = useMemo(
    () => modelWindowStats(windowsQ.data?.buckets ?? [], id, windowTo),
    [windowsQ.data, id, windowTo],
  );

  const sameTier = useMemo(() => {
    if (model == null) return [];
    const tier = tierForContextWindow(model.context_window);
    return all
      .filter(m => m.id !== model.id && tierForContextWindow(m.context_window) === tier)
      .slice(0, 3);
  }, [model, all]);

  if (error && model == null) return renderError(error);
  if (catalogQ.isLoading && model == null) {
    return (
      <Page
        header={
          <PageHeader
            left={
              <Text variant="h4" as="h2">
                Model
              </Text>
            }
          />
        }
      >
        <Page.Section>
          <ResponsiveGrid>
            <Card.Loading className="w-40" />
            <Card.Loading className="w-40" />
            <Card.Loading className="w-40" />
            <Card.Loading className="w-40" />
          </ResponsiveGrid>
          <Card.Loading />
        </Page.Section>
      </Page>
    );
  }
  if (model == null) return renderError(missing ?? "Model not found.");

  return (
    <Page
      header={
        <PageHeader
          left={
            <Text variant="h4" as="h2" className="whitespace-nowrap">
              {model.provider} / {model.id}
            </Text>
          }
        />
      }
    >
      <Page.Section>
        <div className="flex flex-row flex-wrap items-start gap-4">
          <Tooltip content={model.id} side="right">
            <span className="inline-flex items-center gap-2 text-sm text-muted-foreground">
              model id <code className="text-xs">{model.id}</code>
            </span>
          </Tooltip>
          <Badge.Tier tier={tierForContextWindow(model.context_window)} />
          {model.capabilities.map(cap => (
            <Badge.Capability key={cap} name={cap} />
          ))}
          <button
            type="button"
            onClick={() => basket.add(model.id)}
            className="rounded-md border border-primary/30 px-2 py-1 text-2xs text-primary hover:bg-primary/10"
          >
            {basket.ids.includes(model.id) ? "In basket" : "Compare +"}
          </button>
        </div>

        <ResponsiveGrid>
          <MiniCard label="Context" value={formatContext(model.context_window)} />
          <MiniCard label="Input/1M" value={formatUSD(toNumber(model.input_per_1m))} />
          <MiniCard label="Output/1M" value={formatUSD(toNumber(model.output_per_1m))} />
          <MiniCard label="Cached/1M" value={formatUSD(toNumber(model.cached_input_per_1m))} />
          <MiniCard label="Effort default" value={model.reasoning_effort_default} />
        </ResponsiveGrid>

        <ChartCard
          title="Usage"
          subtitle="Requests, spend, or cost-per-1K per bucket for this model."
          topRight={
            <MetricToggle
              options={[
                { value: "requests", label: "Requests" },
                { value: "spend", label: "Spend" },
                { value: "cost_per_1k", label: "Cost/1K" },
              ]}
              value={metric}
              onChange={v => setMetric(v as ModelBreakdownMetric)}
            />
          }
        >
          {chartQ.isLoading && buckets.length === 0 ? (
            <Card.Loading className="border-0 shadow-none" />
          ) : buckets.length === 0 ? (
            <EmptyChart />
          ) : (
            <ModelBreakdownChart buckets={buckets} granularity={granularity} metric={metric} />
          )}
        </ChartCard>

        <Card>
          <Card.Header>
            <Card.Title variant="h4">24h / 7d / 30d</Card.Title>
            <Card.Description>Mini-statistics over three windows for this model.</Card.Description>
          </Card.Header>
          <Card.Content className="flex flex-row flex-wrap gap-4">
            {(["24h", "7d", "30d"] as const).map(range => (
              <MiniStatCard
                key={range}
                range={range}
                stat={windowsQ.isLoading ? null : windowStats[range]}
              />
            ))}
          </Card.Content>
        </Card>

        {sameTier.length > 0 && (
          <Card>
            <Card.Header>
              <Card.Title variant="h4">Compare with…</Card.Title>
              <Card.Description>Other models in the same tier.</Card.Description>
            </Card.Header>
            <Card.Content className="flex flex-row flex-wrap gap-2">
              {sameTier.map(m => (
                <button
                  key={m.id}
                  type="button"
                  onClick={() => basket.add(m.id)}
                  className="rounded-md border border-border px-2 py-1 text-xs hover:bg-foreground/5"
                >
                  {m.id}
                </button>
              ))}
            </Card.Content>
          </Card>
        )}
      </Page.Section>
    </Page>
  );
}

function MiniCard({ label, value }: { label: string; value: string }) {
  return (
    <Card size="sm" className="w-40">
      <Card.Header>
        <Text className="text-2xs font-medium uppercase tracking-wider text-muted-foreground">
          {label}
        </Text>
      </Card.Header>
      <Card.Content>
        <Text className="font-display text-xl font-semibold tabular-nums">{value}</Text>
      </Card.Content>
    </Card>
  );
}

function MiniStatCard({
  range,
  stat,
}: {
  range: ModelWindow;
  stat: { requests: number; tokens: number; cost: number } | null;
}) {
  if (stat == null) {
    return (
      <Card size="sm" className="w-28">
        <Card.Content className="space-y-2 p-3">
          <Skeleton className="h-3 w-8" />
          <Skeleton className="h-3 w-16" />
          <Skeleton className="h-3 w-14" />
        </Card.Content>
      </Card>
    );
  }
  return (
    <div className="flex flex-col gap-0.5 rounded-lg border border-border p-3 text-2xs">
      <span className="text-muted-foreground">{range}</span>
      <span className="tabular-nums">{formatNumber(stat.requests)} req</span>
      <span className="tabular-nums">{formatNumber(stat.tokens)} tok</span>
      <span className="tabular-nums">{formatUSD(stat.cost)}</span>
    </div>
  );
}

function EmptyChart() {
  return (
    <div className="flex h-40 items-center justify-center text-2xs text-muted-foreground">
      No data for this period.
    </div>
  );
}

function renderError(message: string) {
  return (
    <Page
      header={
        <PageHeader
          left={
            <Text variant="h4" as="h2">
              Model
            </Text>
          }
        />
      }
    >
      <Page.Section>
        <div className="rounded-lg border border-danger/30 bg-danger/5 p-4 text-sm text-danger">
          {message}
          <Link href="/models" className="ml-2 underline">
            Back to Models
          </Link>
        </div>
      </Page.Section>
    </Page>
  );
}
