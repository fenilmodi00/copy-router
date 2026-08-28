"use client";

import { Text } from "@/components/atoms/Text";
import { Card } from "@/components/molecules/Card";
import { Cpu } from "lucide-react";

export function PlaygroundEmptyState({ modelLabel }: { modelLabel: string }) {
	return (
		<div className="flex flex-1 items-center justify-center p-8">
			<Card size="sm" className="max-w-md border-border-darker text-center shadow-sm">
				<Card.Content className="flex flex-col items-center gap-3 py-8">
					<div className="flex size-10 items-center justify-center rounded-lg border border-border bg-muted">
						<Cpu className="size-5 text-muted-foreground" />
					</div>
					<Text variant="h4" as="p">
						{modelLabel}
					</Text>
					<Text className="text-sm text-muted-foreground">
						Send a message to run the router.
					</Text>
				</Card.Content>
			</Card>
		</div>
	);
}
