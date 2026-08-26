import { Text } from "@/components/atoms/Text";
import { Card } from "@/components/molecules/Card";
import { cn } from "@/lib/cn";

// Cache hit rate as a big percent. `totalInputTokens <= 0` (including NaN —
// `NaN <= 0` is false, so guard both) renders the `—%` contract — no fake
// percentage on a fresh install (spec user story 19).
export function CacheHitGauge({
  cacheReadTokens,
  totalInputTokens,
  className,
}: {
  cacheReadTokens: number;
  totalInputTokens: number;
  className?: string;
}) {
  const rate = hitRate(cacheReadTokens, totalInputTokens);
  return (
    <Card size="sm" className={cn(className)}>
      <Card.Header>
        <Text className="text-2xs font-medium uppercase tracking-wider text-muted-foreground">
          Cache hit rate
        </Text>
      </Card.Header>
      <Card.Content className="flex items-center gap-4">
        {rate == null ? (
          <Text className="font-display text-2xl font-semibold">—%</Text>
        ) : (
          <Ring value={rate} />
        )}
      </Card.Content>
    </Card>
  );
}

function hitRate(cacheReadTokens: number, totalInputTokens: number): number | null {
  if (!Number.isFinite(totalInputTokens) || totalInputTokens <= 0) return null;
  return (cacheReadTokens / totalInputTokens) * 100;
}

function Ring({ value }: { value: number }) {
  const r = 28;
  const c = 2 * Math.PI * r;
  const pct = Math.max(0, Math.min(100, value));
  return (
    <div className="flex items-center gap-3">
      <svg width={72} height={72} viewBox="0 0 72 72" aria-hidden>
        <circle cx={36} cy={36} r={r} fill="none" strokeWidth={6} className="stroke-muted" />
        <circle
          cx={36}
          cy={36}
          r={r}
          fill="none"
          strokeWidth={6}
          strokeLinecap="round"
          className="stroke-success"
          strokeDasharray={`${(pct / 100) * c} ${c}`}
          transform="rotate(-90 36 36)"
        />
      </svg>
      <Text className="font-display text-2xl font-semibold tabular-nums">{pct.toFixed(0)}%</Text>
    </div>
  );
}