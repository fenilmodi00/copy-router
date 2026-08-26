"use client";

import { Badge } from "@/components/atoms/Badge";
import { Text } from "@/components/atoms/Text";
import { Card } from "@/components/molecules/Card";
import { Page } from "@/components/Page";
import { PageHeader } from "@/components/PageHeader";
import { api, type AiandModel } from "@/lib/api";
import { formatContext, formatUSD, toNumber } from "@/lib/format";
import { tierForContextWindow } from "@/lib/tier";
import { useCompareBasket, CAP, dedupeAndCap } from "@/lib/compare-basket-store";
import { cachedVerdict, plainVerdict } from "@/lib/compare-verdict";
import { useEffect, useMemo, useState } from "react";
import Link from "next/link";

export default function ComparePage() {
  const basket = useCompareBasket();
  const [catalog, setCatalog] = useState<AiandModel[] | null>(null);

  // Hydrate a shared ?ids=a,b,c,d URL (up to the basket cap) once, on mount.
  // add() enforces the cap on each insert, so pushing in order preserves the
  // URL's priority for any over-cap payload.
  useEffect(() => {
    if (typeof window === "undefined") return;
    const params = new URLSearchParams(window.location.search);
    const raw = (params.get("ids") ?? "").split(",").filter(Boolean);
    const ordered = dedupeAndCap(raw, CAP);
    if (ordered.length > 0) ordered.forEach(id => basket.add(id));
    basket.setHydrated(true);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  useEffect(() => {
    api.aiandModels
      .list()
      .then(res => setCatalog(res.data ?? []))
      .catch(() => setCatalog([]));
  }, []);

  const models = useMemo(() => {
    const byId = new Map((catalog ?? []).map(m => [m.id, m]));
    return basket.ids.map(id => byId.get(id)).filter((m): m is AiandModel => m != null);
  }, [catalog, basket.ids]);

  const verdicts = useMemo(() => models.map(m => plainVerdict(m.input_per_1m, m.output_per_1m)), [models]);
  const cached = useMemo(
    () => models.map(m => cachedVerdict(m.input_per_1m, m.output_per_1m, m.cached_input_per_1m)),
    [models],
  );

  // Green-tint the cheapest 3 on each verdict column (sentinel keeps the set
  // disjoint from real indices).
  const cheapestNoCache = useMemo(() => {
    const byVerdict = verdicts.map((v, i) => ({ v, i })).sort((a, b) => a.v - b.v);
    return new Set(byVerdict.slice(0, 3).map(x => x.i));
  }, [verdicts]);
  const cheapestCached = useMemo(() => {
    if (cached.length === 0) return new Set<number>();
    const min = Math.min(...cached);
    return new Set(cached.map((v, i) => (v === min ? i : -1)).filter(i => i >= 0));
  }, [cached]);

  if (!basket.hydrated) return null;

  return (
    <Page
      header={
        <PageHeader
          left={
            <Text variant="h4" as="h2">
              Compare models
            </Text>
          }
        />
      }
    >
      <Page.Section>
        {models.length === 0 ? (
          <div className="rounded-lg border border-border bg-muted p-8 text-center text-sm text-muted-foreground">
            No models selected. Add up to 4 models from a model page or a
            shareable ?ids= URL.
          </div>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-xs">
              <thead>
                <tr className="text-left text-2xs uppercase tracking-wider text-muted-foreground">
                  <th className="px-3 py-2 font-medium">Attribute</th>
                  {models.map(m => (
                    <th key={m.id} className="px-3 py-2 font-medium">
                      <Link href={`/models/${m.id.replace(/\//g, "~")}`} className="hover:text-primary">
                        {m.id}
                      </Link>
                      <button
                        type="button"
                        onClick={() => basket.remove(m.id)}
                        className="ml-2 text-muted-foreground hover:text-danger"
                        aria-label={`Remove ${m.id}`}
                      >
                        ✕
                      </button>
                    </th>
                  ))}
                </tr>
              </thead>
              <tbody>
                <CompareRow label="Provider" cells={models.map(m => m.provider)} />
                <CompareRow
                  label="Capabilities"
                  cells={models.map(m => (
                    <span key={m.id} className="flex flex-wrap gap-1">
                      {m.capabilities.map(cap => (
                        <Badge.Capability key={cap} name={cap} />
                      ))}
                    </span>
                  ))}
                />
                <CompareRow
                  label="Tier"
                  cells={models.map(m => <Badge.Tier key={m.id} tier={tierForContextWindow(m.context_window)} />)}
                />
                <CompareRow label="Context" cells={models.map(m => formatContext(m.context_window))} />
                <CompareRow label="Reasoning efforts" cells={models.map(m => m.reasoning_efforts.join(" / "))} />
                <CompareRow
                  label="Input/1M"
                  cells={models.map(m => formatUSD(toNumber(m.input_per_1m)))}
                />
                <CompareRow
                  label="Cached/1M"
                  cells={models.map(m => formatUSD(toNumber(m.cached_input_per_1m)))}
                />
                <CompareRow label="Output/1M" cells={models.map(m => formatUSD(toNumber(m.output_per_1m)))} />
                <CompareRow
                  label="Sample cost (15K in + 35K out)"
                  cells={models.map((m, i) => (
                    <span key={m.id} className={cheapestNoCache.has(i) ? "text-success" : ""}>
                      {formatUSD(verdicts[i])}
                    </span>
                  ))}
                />
                <CompareRow
                  label="Sample cost @ 70% cache hit"
                  cells={models.map((m, i) => (
                    <span key={m.id} className={cheapestCached.has(i) ? "text-success" : ""}>
                      {formatUSD(cached[i])}
                    </span>
                  ))}
                />
              </tbody>
            </table>
          </div>
        )}

        {basket.ids.length > 0 && (
          <button
            type="button"
            onClick={() => basket.clear()}
            className="text-2xs text-muted-foreground hover:text-foreground"
          >
            Clear comparison
          </button>
        )}
      </Page.Section>
    </Page>
  );
}

// Cells positionally match the header's model columns, so keys are the row's
// ordinal per model id; stable within a render.
function CompareRow<T extends React.ReactNode>({ label, cells }: { label: string; cells: T[] }) {
  return (
    <tr className="border-t border-border/50">
      <td className="px-3 py-2 font-medium text-muted-foreground">{label}</td>
      {cells.map((cell, i) => (
        <td key={i} className="px-3 py-2">
          {cell}
        </td>
      ))}
    </tr>
  );
}