"use client";

import type { PlaygroundMessage } from "@/lib/playground-store";
import { useEffect, useRef } from "react";
import { MessageBubble } from "./MessageBubble";

export function MessageList({
	messages,
	onRetry,
	showDetails = true,
}: {
	messages: PlaygroundMessage[];
	onRetry: () => void;
	showDetails?: boolean;
}) {
	const endRef = useRef<HTMLDivElement>(null);

	useEffect(() => {
		endRef.current?.scrollIntoView?.({ behavior: "smooth" });
	}, [messages]);

	if (messages.length === 0) return null;

	return (
		<div className="flex flex-1 flex-col gap-4 overflow-y-auto p-4">
			{messages.map((message) => (
				<MessageBubble
					key={message.id}
					message={message}
					onRetry={onRetry}
					showDetails={showDetails}
				/>
			))}
			<div ref={endRef} />
		</div>
	);
}
