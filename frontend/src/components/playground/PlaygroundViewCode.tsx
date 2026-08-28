"use client";

import { CopyBlock } from "@/components/CopyBlock";
import { Text } from "@/components/atoms/Text";
import { KEY_PLACEHOLDER, usageSnippet } from "@/lib/usageSnippets";
import { routerOrigin } from "@/lib/installCommands";
import type { AiandModel } from "@/lib/api";
import type { PlaygroundModel, PlaygroundParams } from "@/lib/playground-store";
import { buildChatBody } from "@/lib/playground-chat";

export function PlaygroundViewCode({
	model,
	params,
	messages,
	models,
}: {
	model: PlaygroundModel;
	params: PlaygroundParams;
	messages: Array<{ role: "user" | "assistant"; content: string }>;
	models: AiandModel[];
}) {
	const origin = routerOrigin();
	const body = buildChatBody(messages, model, params, models);
	const snippet =
		origin === ""
			? ""
			: usageSnippet("curl", model ? "force" : "auto", {
					origin,
					apiKey: KEY_PLACEHOLDER,
					forceModel: model ?? undefined,
				});

	return (
		<div className="border-b border-border bg-muted/30 px-4 py-3">
			<Text className="mb-2 text-2xs text-muted-foreground">
				Equivalent request body for the current playground settings:
			</Text>
			<pre className="mb-3 max-h-32 overflow-auto rounded-lg border border-border bg-background p-3 font-mono text-2xs">
				<code>{JSON.stringify(body, null, 2)}</code>
			</pre>
			{origin !== "" ? (
				<CopyBlock value={snippet} title="Copy curl snippet" />
			) : (
				<Text className="text-2xs text-muted-foreground">Preparing…</Text>
			)}
		</div>
	);
}
