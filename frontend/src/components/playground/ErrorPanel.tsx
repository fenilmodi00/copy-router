"use client";

import { Text } from "@/components/atoms/Text";
import { Button } from "@/components/molecules/Button";
import { Intent } from "@/components/types";
import type { PlaygroundError } from "@/lib/playground-store";

export function ErrorPanel({
	error,
	onRetry,
}: {
	error: PlaygroundError;
	onRetry: () => void;
}) {
	return (
		<div
			role="alert"
			className="rounded-lg border border-danger/30 bg-danger/5 p-4 text-sm"
		>
			<Text variant="h4" as="p" className="text-danger">
				{error.type}
			</Text>
			<p className="mt-1 text-muted-foreground">{error.message}</p>
			<Button
				type="button"
				intent={Intent.Danger}
				className="mt-3"
				size="sm"
				onClick={onRetry}
			>
				Retry
			</Button>
		</div>
	);
}
