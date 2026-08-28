"use client";

import { create } from "zustand";

export const STORAGE_KEY = "weave-router.playground";

export type PlaygroundDecision = {
	model: string;
	provider: string;
	reason: string;
	requestedCostUsd: number;
	actualCostUsd: number;
	id: string;
	cacheSavingsUsd: number;
};

export type PlaygroundError = {
	type: string;
	message: string;
	param: unknown;
	code: string;
};

export type PlaygroundMessage = {
	id: string;
	role: "user" | "assistant" | "system";
	content: string;
	decision: PlaygroundDecision | null;
	error: PlaygroundError | null;
	createdAt: number;
};

export type PlaygroundStatus = "idle" | "streaming" | "error";
export type PlaygroundModel = string | null;
export type PlaygroundParams = {
	temperature?: number;
	max_tokens?: number;
	reasoning?: string;
	stream?: boolean;
};

export const DEFAULT_PLAYGROUND_PARAMS: PlaygroundParams = {
	temperature: 1,
	max_tokens: 4096,
	stream: true,
};

type PersistedState = {
	messages: PlaygroundMessage[];
	model: PlaygroundModel;
	sessionID: string;
};

function newId(): string {
	return Math.random().toString(36).slice(2, 11);
}

export function newSessionID(): string {
	return Math.random().toString(36).slice(2, 14);
}

export function normalizeDecision(
	raw: Partial<PlaygroundDecision> & {
		requested_cost_usd?: number;
		actual_cost_usd?: number;
		cache_savings_usd?: number;
	},
): PlaygroundDecision {
	return {
		model: raw.model ?? "",
		provider: raw.provider ?? "",
		reason: raw.reason ?? "",
		requestedCostUsd: raw.requestedCostUsd ?? raw.requested_cost_usd ?? 0,
		actualCostUsd: raw.actualCostUsd ?? raw.actual_cost_usd ?? 0,
		id: raw.id ?? "",
		cacheSavingsUsd: raw.cacheSavingsUsd ?? raw.cache_savings_usd ?? 0,
	};
}

function normalizeMessage(msg: PlaygroundMessage): PlaygroundMessage {
	return {
		...msg,
		decision: msg.decision ? normalizeDecision(msg.decision) : null,
	};
}

function readInitial(): PersistedState {
	if (typeof window === "undefined") {
		return { messages: [], model: null, sessionID: "" };
	}
	try {
		const raw = window.localStorage.getItem(STORAGE_KEY);
		if (raw == null) return { messages: [], model: null, sessionID: "" };
		const parsed = JSON.parse(raw) as PersistedState;
		return {
			messages: (parsed.messages ?? []).map(normalizeMessage),
			model: parsed.model ?? null,
			sessionID: parsed.sessionID ?? "",
		};
	} catch {
		return { messages: [], model: null, sessionID: "" };
	}
}

function persist(state: Pick<PersistedState, "messages" | "model" | "sessionID">) {
	if (typeof window === "undefined") return;
	try {
		window.localStorage.setItem(
			STORAGE_KEY,
			JSON.stringify({
				messages: state.messages,
				model: state.model,
				sessionID: state.sessionID,
			}),
		);
	} catch {
		// Storage unavailable: keep in-memory state.
	}
}

function lastUserIndex(messages: PlaygroundMessage[]): number {
	for (let i = messages.length - 1; i >= 0; i--) {
		if (messages[i].role === "user") return i;
	}
	return -1;
}

function lastAssistantIndex(messages: PlaygroundMessage[]): number {
	for (let i = messages.length - 1; i >= 0; i--) {
		if (messages[i].role === "assistant") return i;
	}
	return -1;
}

const initial = readInitial();

export interface PlaygroundStore {
	messages: PlaygroundMessage[];
	status: PlaygroundStatus;
	model: PlaygroundModel;
	params: PlaygroundParams;
	sessionID: string;
	abortController: AbortController | null;
	pendingDecision: PlaygroundDecision | null;
	appendUser: (text: string) => void;
	appendAssistantDelta: (delta: string) => void;
	appendSystemOrError: (msg: PlaygroundMessage) => void;
	setDecision: (messageId: string, decision: PlaygroundDecision) => void;
	attachStreamingDecision: (decision: PlaygroundDecision) => void;
	retryLastUser: () => void;
	abort: () => void;
	clear: () => void;
	hydrate: () => void;
	setModel: (model: PlaygroundModel) => void;
	setParams: (params: PlaygroundParams) => void;
	setSessionID: (sessionID: string) => void;
	beginStreaming: () => AbortController;
	endStreaming: (status?: PlaygroundStatus) => void;
}

export const usePlaygroundStore = create<PlaygroundStore>((set, get) => ({
	messages: initial.messages,
	status: "idle",
	model: initial.model,
	params: { ...DEFAULT_PLAYGROUND_PARAMS },
	sessionID: initial.sessionID,
	abortController: null,
	pendingDecision: null,
	appendUser: (text) =>
		set((state) => {
			const messages: PlaygroundMessage[] = [
				...state.messages,
				{
					id: newId(),
					role: "user",
					content: text,
					decision: null,
					error: null,
					createdAt: Date.now(),
				},
			];
			persist({ messages, model: state.model, sessionID: state.sessionID });
			return { messages };
		}),
	appendAssistantDelta: (delta) =>
		set((state) => {
			const messages = [...state.messages];
			const last = messages[messages.length - 1];
			const prev = messages[messages.length - 2];
			const canAccumulate =
				last?.role === "assistant" && prev?.role !== "system";
			const pendingDecision = state.pendingDecision;
			if (canAccumulate) {
				messages[messages.length - 1] = {
					...last,
					content: last.content + delta,
					decision: last.decision ?? pendingDecision,
				};
			} else {
				messages.push({
					id: newId(),
					role: "assistant",
					content: delta,
					decision: pendingDecision,
					error: null,
					createdAt: Date.now(),
				});
			}
			persist({ messages, model: state.model, sessionID: state.sessionID });
			return { messages, status: "streaming", pendingDecision: null };
		}),
	appendSystemOrError: (msg) =>
		set((state) => {
			const messages = [...state.messages];
			const userIdx = lastUserIndex(messages);
			const insertAt = userIdx >= 0 ? userIdx + 1 : messages.length;
			messages.splice(insertAt, 0, msg);
			persist({ messages, model: state.model, sessionID: state.sessionID });
			return { messages };
		}),
	setDecision: (messageId, decision) =>
		set((state) => {
			const normalized = normalizeDecision(decision);
			const messages = state.messages.map((msg) =>
				msg.id === messageId ? { ...msg, decision: normalized } : msg,
			);
			persist({ messages, model: state.model, sessionID: state.sessionID });
			return { messages, pendingDecision: null };
		}),
	attachStreamingDecision: (decision) =>
		set((state) => {
			const normalized = normalizeDecision(decision);
			const assistantIdx = lastAssistantIndex(state.messages);
			if (assistantIdx >= 0) {
				const messages = state.messages.map((msg, idx) =>
					idx === assistantIdx ? { ...msg, decision: normalized } : msg,
				);
				persist({ messages, model: state.model, sessionID: state.sessionID });
				return { messages, pendingDecision: null };
			}
			return { pendingDecision: normalized };
		}),
	retryLastUser: () => {
		const state = get();
		const lastUser = [...state.messages].reverse().find((m) => m.role === "user");
		if (!lastUser) return;
		get().appendUser(lastUser.content);
	},
	abort: () => {
		get().abortController?.abort();
		set({ status: "idle", abortController: null });
	},
	clear: () => {
		persist({ messages: [], model: get().model, sessionID: get().sessionID });
		set({ messages: [], status: "idle", abortController: null });
	},
	hydrate: () => {
		const next = readInitial();
		set((state) => ({
			...state,
			messages: next.messages,
			model: next.model,
			sessionID: next.sessionID,
		}));
	},
	setModel: (model) =>
		set((state) => {
			persist({ messages: state.messages, model, sessionID: state.sessionID });
			return { model };
		}),
	setParams: (params) => set({ params }),
	setSessionID: (sessionID) =>
		set((state) => {
			persist({ messages: state.messages, model: state.model, sessionID });
			return { sessionID };
		}),
	beginStreaming: () => {
		const abortController = new AbortController();
		set({ status: "streaming", abortController, pendingDecision: null });
		return abortController;
	},
	endStreaming: (status = "idle") =>
		set({ status, abortController: null }),
}));

export const clearStoredPlayground = () => {
	try {
		window.localStorage.removeItem(STORAGE_KEY);
	} catch {
		// Non-fatal.
	}
};
