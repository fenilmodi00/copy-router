import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { PlaygroundDecision } from "./playground-store";

const STORAGE_KEY = "weave-router.playground";

async function loadStore() {
	vi.resetModules();
	const mod = await import("./playground-store");
	return mod.usePlaygroundStore;
}

describe("playground-store", () => {
	beforeEach(() => {
		window.localStorage.clear();
		vi.useFakeTimers();
	});

	afterEach(() => {
		vi.useRealTimers();
	});

	it("hydrates from localStorage on boot", async () => {
		const stored = {
			messages: [
				{
					id: "m1",
					role: "user" as const,
					content: "hi",
					decision: null,
					error: null,
					createdAt: 1000,
				},
			],
			model: "zai-org/glm-5.2",
			sessionID: "sess_1",
		};
		window.localStorage.setItem(STORAGE_KEY, JSON.stringify(stored));

		const usePlaygroundStore = await loadStore();
		const store = usePlaygroundStore.getState();
		expect(store.messages).toHaveLength(1);
		expect(store.messages[0].id).toBe("m1");
		expect(store.model).toBe("zai-org/glm-5.2");
		expect(store.sessionID).toBe("sess_1");
		expect(store.status).toBe("idle");
	});

	it("appends user and assistant messages and tracks status transitions", async () => {
		const usePlaygroundStore = await loadStore();
		const store = usePlaygroundStore.getState();
		store.appendUser("hello");
		store.appendAssistantDelta(" world");
		store.appendSystemOrError({
			id: "sys1",
			role: "system",
			content: "note",
			decision: null,
			error: null,
			createdAt: Date.now(),
		});
		expect(usePlaygroundStore.getState().messages).toHaveLength(3);
		expect(usePlaygroundStore.getState().status).toBe("streaming");

		store.appendAssistantDelta(" more");
		expect(usePlaygroundStore.getState().messages[2].content).toBe(" world");
		expect(usePlaygroundStore.getState().messages[3].content).toBe(" more");
	});

	it("retryLastUser re-sends the last user message", async () => {
		const usePlaygroundStore = await loadStore();
		const store = usePlaygroundStore.getState();
		store.appendUser("first");
		store.appendAssistantDelta("resp");
		store.appendUser("second");

		store.retryLastUser();

		const messages = usePlaygroundStore.getState().messages;
		expect(messages[messages.length - 1].role).toBe("user");
		expect(messages[messages.length - 1].content).toBe("second");
	});

	it("retryLastUser is a no-op when there is no prior user message", async () => {
		const usePlaygroundStore = await loadStore();
		const store = usePlaygroundStore.getState();
		store.retryLastUser();
		expect(store.messages).toHaveLength(0);
	});

	it("attaches decision to assistant message via setDecision", async () => {
		const decision: PlaygroundDecision = {
			model: "zai-org/glm-5.2",
			provider: "zai",
			reason: "best fit",
			requestedCostUsd: 0.1,
			actualCostUsd: 0.05,
			id: "req_1",
			cacheSavingsUsd: 0.05,
		};
		const usePlaygroundStore = await loadStore();
		const store = usePlaygroundStore.getState();
		store.appendAssistantDelta("hello");
		const assistantMsg = usePlaygroundStore.getState().messages[0];
		store.setDecision(assistantMsg.id, decision);

		expect(usePlaygroundStore.getState().messages[0].decision).toEqual(decision);
	});

	it("buffers routing_metadata when it arrives before the first assistant delta", async () => {
		const decision: PlaygroundDecision = {
			model: "deepseek-ai/deepseek-v4-flash",
			provider: "aiand",
			reason: "cluster pick",
			requestedCostUsd: 0.01,
			actualCostUsd: 0.008,
			id: "req_early",
			cacheSavingsUsd: 0,
		};
		const usePlaygroundStore = await loadStore();
		const store = usePlaygroundStore.getState();
		store.attachStreamingDecision(decision);
		expect(usePlaygroundStore.getState().pendingDecision).toEqual(decision);
		expect(usePlaygroundStore.getState().messages).toHaveLength(0);

		store.appendAssistantDelta("Hello");
		expect(usePlaygroundStore.getState().messages[0]?.decision).toEqual(decision);
		expect(usePlaygroundStore.getState().pendingDecision).toBeNull();
	});

	it("setDecision only affects the matching message id", async () => {
		const decision: PlaygroundDecision = {
			model: "m",
			provider: "p",
			reason: "r",
			requestedCostUsd: 0,
			actualCostUsd: 0,
			id: "id1",
			cacheSavingsUsd: 0,
		};
		const usePlaygroundStore = await loadStore();
		const store = usePlaygroundStore.getState();
		store.appendAssistantDelta("a");
		store.appendSystemOrError({
			id: "sys1",
			role: "system",
			content: "break",
			decision: null,
			error: null,
			createdAt: Date.now(),
		});
		store.appendAssistantDelta("b");
		store.setDecision(usePlaygroundStore.getState().messages[2].id, decision);

		expect(usePlaygroundStore.getState().messages[2].decision).toEqual(decision);
		expect(usePlaygroundStore.getState().messages[0].decision).toBeNull();
	});

	it("clear() wipes the conversation", async () => {
		const usePlaygroundStore = await loadStore();
		const store = usePlaygroundStore.getState();
		store.appendUser("hi");
		store.appendAssistantDelta("hi");
		store.clear();

		expect(usePlaygroundStore.getState().messages).toHaveLength(0);
		expect(usePlaygroundStore.getState().status).toBe("idle");
	});

	it("corrupt localStorage payload -> empty state", async () => {
		window.localStorage.setItem(STORAGE_KEY, ":not:json");
		const usePlaygroundStore = await loadStore();
		const store = usePlaygroundStore.getState();
		expect(store.messages).toHaveLength(0);
		expect(store.model).toBeNull();
		expect(store.sessionID).toBe("");
	});

	it("PlaygroundDecision without cacheSavingsUsd hydrates to 0", async () => {
		const stored = {
			messages: [
				{
					id: "m1",
					role: "assistant" as const,
					content: "hi",
					decision: {
						model: "m",
						provider: "p",
						reason: "r",
						requestedCostUsd: 0,
						actualCostUsd: 0,
						id: "id1",
					},
					error: null,
					createdAt: 1000,
				},
			],
			model: null,
			sessionID: "sess_1",
		};
		window.localStorage.setItem(STORAGE_KEY, JSON.stringify(stored));
		const usePlaygroundStore = await loadStore();
		usePlaygroundStore.getState().hydrate();

		const msg = usePlaygroundStore.getState().messages[0];
		expect(msg.decision?.cacheSavingsUsd).toBe(0);
	});
});
