"use client";

import { Page } from "@/components/Page";
import { ControlPanel } from "@/components/playground/ControlPanel";
import { PlaygroundComposer } from "@/components/playground/Composer";
import { MessageList } from "@/components/playground/MessageList";
import { PlaygroundEmptyState } from "@/components/playground/PlaygroundEmptyState";
import { PlaygroundHeader } from "@/components/playground/PlaygroundHeader";
import { PlaygroundToolbar } from "@/components/playground/PlaygroundToolbar";
import { PlaygroundViewCode } from "@/components/playground/PlaygroundViewCode";
import { useCatalog } from "@/lib/data-cache";
import { runPlaygroundChat } from "@/lib/playground-chat";
import {
	newSessionID,
	usePlaygroundStore,
	type PlaygroundMessage,
} from "@/lib/playground-store";
import { useCallback, useEffect, useMemo, useState } from "react";

function chatMessages(messages: PlaygroundMessage[]) {
	return messages
		.filter((m) => m.role === "user" || m.role === "assistant")
		.map((m) => ({
			role: m.role as "user" | "assistant",
			content: m.content,
		}));
}

export default function PlaygroundPage() {
	const catalogQ = useCatalog();
	const messages = usePlaygroundStore((s) => s.messages);
	const status = usePlaygroundStore((s) => s.status);
	const model = usePlaygroundStore((s) => s.model);
	const params = usePlaygroundStore((s) => s.params);
	const setModel = usePlaygroundStore((s) => s.setModel);
	const setParams = usePlaygroundStore((s) => s.setParams);
	const setSessionID = usePlaygroundStore((s) => s.setSessionID);
	const appendUser = usePlaygroundStore((s) => s.appendUser);
	const appendAssistantDelta = usePlaygroundStore((s) => s.appendAssistantDelta);
	const attachStreamingDecision = usePlaygroundStore((s) => s.attachStreamingDecision);
	const beginStreaming = usePlaygroundStore((s) => s.beginStreaming);
	const endStreaming = usePlaygroundStore((s) => s.endStreaming);
	const abort = usePlaygroundStore((s) => s.abort);
	const retryLastUser = usePlaygroundStore((s) => s.retryLastUser);
	const hydrate = usePlaygroundStore((s) => s.hydrate);

	const [showDetails, setShowDetails] = useState(true);
	const [viewCode, setViewCode] = useState(false);

	useEffect(() => {
		hydrate();
		const state = usePlaygroundStore.getState();
		if (!state.sessionID) {
			setSessionID(newSessionID());
		}
	}, [hydrate, setSessionID]);

	const modelLabel = model ?? "auto route";
	const transcript = useMemo(() => chatMessages(messages), [messages]);

	const runChat = useCallback(async () => {
		const ac = beginStreaming();
		const snapshot = usePlaygroundStore.getState();

		await runPlaygroundChat({
			messages: chatMessages(snapshot.messages),
			model: snapshot.model,
			params: snapshot.params,
			models: catalogQ.data ?? [],
			sessionID: snapshot.sessionID,
			signal: ac.signal,
			onDelta: (delta) => appendAssistantDelta(delta),
			onDecision: (decision) => attachStreamingDecision(decision),
			onError: (error) => {
				usePlaygroundStore.getState().appendSystemOrError({
					id: Math.random().toString(36).slice(2, 11),
					role: "assistant",
					content: "",
					decision: null,
					error,
					createdAt: Date.now(),
				});
				endStreaming("error");
			},
			onDone: () => endStreaming("idle"),
		});
	}, [appendAssistantDelta, attachStreamingDecision, beginStreaming, catalogQ.data, endStreaming]);

	const send = useCallback(
		async (text: string) => {
			appendUser(text);
			await runChat();
		},
		[appendUser, runChat],
	);

	const handleRetry = useCallback(() => {
		retryLastUser();
		void runChat();
	}, [retryLastUser, runChat]);

	const handleCopyModel = useCallback(() => {
		const value = model ?? "auto";
		void navigator.clipboard.writeText(value);
	}, [model]);

	return (
		<Page
			className="items-stretch overflow-hidden"
			header={<PlaygroundHeader />}
			subheader={
				<PlaygroundToolbar
					models={catalogQ.data ?? []}
					model={model}
					onModelChange={setModel}
					showDetails={showDetails}
					onShowDetailsChange={setShowDetails}
					viewCode={viewCode}
					onViewCodeChange={setViewCode}
					onCopy={handleCopyModel}
				/>
			}
		>
			<div className="flex h-full w-full max-w-none">
				<div className="flex min-h-0 min-w-0 flex-1 flex-col">
					{viewCode ? (
						<PlaygroundViewCode
						model={model}
						params={params}
						messages={transcript}
						models={catalogQ.data ?? []}
					/>
					) : null}
					{messages.length === 0 ? (
						<PlaygroundEmptyState modelLabel={modelLabel} />
					) : (
						<MessageList
							messages={messages}
							onRetry={handleRetry}
							showDetails={showDetails}
						/>
					)}
					<PlaygroundComposer
						status={status}
						onSubmit={(text) => void send(text)}
						onStop={abort}
					/>
				</div>
				<ControlPanel
					params={params}
					onChange={setParams}
					model={model}
					models={catalogQ.data ?? []}
				/>
			</div>
		</Page>
	);
}
