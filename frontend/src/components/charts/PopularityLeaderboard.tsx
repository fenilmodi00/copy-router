"use client";

import { Text } from "@/components/atoms/Text";
import { formatNumber, formatUSD } from "@/lib/format";
import { cn } from "@/lib/cn";

export interface LeaderboardRow {
  id: string;
  label: string;
  tokens: number;
  costUsd: number;
}

// Cross-provider popularity: top-N models by tokens processed on this install,
// horizontal bars — the view ai&'s own console can't render. Every row drills
// to /models/[id].
export function PopularityLeaderboard({
  rows,
  limit = 5,
  onSelect,
  className,
}: {
  rows: LeaderboardRow[];
  limit?: number;
  onSelect: (id: string) => void;
  className?: string;
}) {
  const max = Math.max(...rows.map(r => r.tokens), 1);
  const top = rows.slice(0, limit);
  if (top.length === 0) {
    return <div className="text-2xs text-muted-foreground">No usage in this period.</div>;
  }
  return (
    <div className={cn("flex flex-col gap-2", className)}>
      {top.map((r, i) => (
        <button
          key={r.id}
          type="button"
          onClick={() => onSelect(r.id)}
          className="group flex items-center gap-3 rounded-md px-1 py-0.5 text-left hover:bg-foreground/5"
        >
          <span className="w-4 shrink-0 text-2xs text-muted-foreground">{i + 1}</span>
          <span className="min-w-0 flex-1">
            <span className="flex items-center justify-between gap-2">
              <span className="truncate text-xs font-medium">{r.label}</span>
              <span className="shrink-0 text-2xs text-muted-foreground tabular-nums">
                {formatNumber(r.tokens)} tok
              </span>
            </span>
            <span className="relative mt-0.5 block h-1.5 w-full overflow-hidden rounded-full bg-foreground/5">
              <span
                className="absolute inset-y-0 left-0 rounded-full bg-primary/70"
                style={{ width: `${(r.tokens / max) * 100}%` }}
              />
            </span>
            <span className="text-2xs text-muted-foreground">{formatUSD(r.costUsd)}</span>
          </span>
        </button>
      ))}
    </div>
  );
}