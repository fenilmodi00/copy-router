import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { MessageBubble } from "./MessageBubble";
import type { PlaygroundMessage } from "@/lib/playground-store";

function assistantMessage(decision: PlaygroundMessage["decision"]): PlaygroundMessage {
	return {
		id: "a1",
		role: "assistant",
		content: "Hello from the router.",
		decision,
		error: null,
		createdAt: Date.now(),
	};
}

describe("MessageBubble", () => {
	it("shows the served model badge on auto-route replies", () => {
		render(
			<MessageBubble
				message={assistantMessage({
					model: "deepseek-ai/deepseek-v4-flash",
					provider: "aiand",
					reason: "cluster pick",
					requestedCostUsd: 0.01,
					actualCostUsd: 0.008,
					id: "req-1",
					cacheSavingsUsd: 0,
				})}
				showDetails={false}
			/>,
		);

		expect(screen.getByText("deepseek-ai/deepseek-v4-flash")).toBeInTheDocument();
		expect(screen.getByText("aiand")).toBeInTheDocument();
		expect(screen.queryByText(/cluster pick/)).not.toBeInTheDocument();
	});
});
