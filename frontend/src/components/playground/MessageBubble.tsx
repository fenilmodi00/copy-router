"use client";

import { Badge } from "@/components/atoms/Badge";
import { Text } from "@/components/atoms/Text";
import { Card } from "@/components/molecules/Card";
import { formatUSD } from "@/lib/format";
import type { PlaygroundMessage } from "@/lib/playground-store";
import { DecisionInspector } from "./DecisionInspector";
import { ErrorPanel } from "./ErrorPanel";

function ServedModelBadge({ model, provider }: { model: string; provider: string }) {
	if (!model) return null;
	return (
		<div className="mb-2 flex flex-wrap items-center gap-2">
			<Badge>{provider || "router"}</Badge>
			<Text variant="p" className="text-xs font-medium text-foreground">
				{model}
			</Text>
		</div>
	);
}

export function MessageBubble({
	message,
	onRetry,
	showDetails = true,
}: {
	message: PlaygroundMessage;
	onRetry?: () => void;
	showDetails?: boolean;
}) {
	if (message.role === "user") {
		return (
			<div className="flex justify-end">
				<div className="max-w-[80%] rounded-lg bg-primary/10 px-3 py-2 text-sm">
					{message.content}
				</div>
			</div>
		);
	}

	if (message.role === "system") {
		return (
			<div className="text-center text-xs text-muted-foreground">{message.content}</div>
		);
	}

	return (
		<div className="flex justify-start">
			<div className="max-w-[80%] rounded-lg border border-border bg-card px-3 py-2 text-sm">
				{message.decision ? (
					<ServedModelBadge model={message.decision.model} provider={message.decision.provider} />
				) : null}
				{message.content ? <p className="whitespace-pre-wrap">{message.content}</p> : null}
				{message.decision && showDetails ? (
					<DecisionInspector decision={message.decision} />
				) : null}
				{message.error && onRetry ? (
					<div className="mt-2">
						<ErrorPanel error={message.error} onRetry={onRetry} />
					</div>
				) : null}
			</div>
		</div>
	);
}
