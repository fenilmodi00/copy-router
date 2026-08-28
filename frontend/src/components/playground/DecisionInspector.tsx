"use client";

import { Badge } from "@/components/atoms/Badge";
import { Text } from "@/components/atoms/Text";
import { Card } from "@/components/molecules/Card";
import { formatUSD } from "@/lib/format";
import { formatPlaygroundReason } from "@/lib/playground-reason";
import type { PlaygroundDecision } from "@/lib/playground-store";

export function DecisionInspector({ decision }: { decision: PlaygroundDecision }) {
	return (
		<Card size="sm" className="mt-2 border-border-darker bg-muted/40">
			<div className="flex flex-wrap items-center gap-2">
				<Badge>{decision.provider}</Badge>
				<Text variant="p" className="text-xs text-muted-foreground">
					{decision.model}
				</Text>
			</div>
			<Text variant="p" className="mt-2 text-xs text-muted-foreground">
				{formatPlaygroundReason(decision.reason, decision.model)}
			</Text>
			<div className="mt-2 flex flex-wrap gap-4 text-xs">
				<span>Requested: {formatUSD(decision.requestedCostUsd)}</span>
				<span>Actual: {formatUSD(decision.actualCostUsd)}</span>
				{decision.cacheSavingsUsd > 0 ? (
					<span>Cache savings: {formatUSD(decision.cacheSavingsUsd)}</span>
				) : null}
			</div>
		</Card>
	);
}
