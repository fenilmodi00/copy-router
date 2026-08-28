import {
	normalizeDecision,
	type PlaygroundDecision,
	type PlaygroundError,
} from "./playground-store";

export interface SSEHandlers {
	onDelta: (delta: string) => void;
	onDecision: (decision: PlaygroundDecision) => void;
	onError: (error: PlaygroundError) => void;
	onDone: () => void;
}

export interface SSEOptions extends SSEHandlers {
	url: string;
	body: unknown;
	signal?: AbortSignal;
}

export interface SSEResponseOptions extends SSEHandlers {
	response: Response;
	signal?: AbortSignal;
}

const ROUTING_MARKER_PREFIX = "✦ **Weave Router**";

type ErrorPayload = Partial<PlaygroundError> & {
	error?: Partial<PlaygroundError>;
};

function normalizePlaygroundError(
	err: Partial<PlaygroundError>,
	status: number,
): PlaygroundError {
	return {
		type: err.type ?? "http_error",
		message: err.message ?? "Request failed",
		param: err.param ?? null,
		code: err.code != null ? String(err.code) : String(status),
	};
}

function parseJsonErrorPayload(text: string): PlaygroundError | null {
	try {
		const parsed = JSON.parse(text) as ErrorPayload;
		if (parsed.error) {
			return normalizePlaygroundError(parsed.error, 0);
		}
		if (parsed.type || parsed.message || parsed.code) {
			return normalizePlaygroundError(parsed, 0);
		}
	} catch {
		// Not JSON.
	}
	return null;
}

function parseSSEDataLine(line: string): PlaygroundError | null {
	const trimmed = line.trim();
	if (!trimmed.startsWith("data:")) return null;
	const payload = trimmed.slice(5).trimStart();
	if (payload === "[DONE]") return null;
	return parseJsonErrorPayload(payload);
}

export function parsePlaygroundErrorBody(status: number, text: string): PlaygroundError {
	const lines = text.split("\n");
	let trailingSSE: PlaygroundError | null = null;
	for (let i = lines.length - 1; i >= 0; i--) {
		const candidate = parseSSEDataLine(lines[i] ?? "");
		if (candidate != null) {
			trailingSSE = candidate;
			break;
		}
	}

	const primaryText =
		trailingSSE != null
			? lines
					.slice(0, lines.findLastIndex((line) => parseSSEDataLine(line) != null))
					.join("\n")
					.trim()
			: text.trim();

	const primary = parseJsonErrorPayload(primaryText);
	if (primary != null) {
		if (
			trailingSSE != null &&
			trailingSSE.code === "upstream_interrupted" &&
			primary.type === "invalid_request_error" &&
			primary.message
		) {
			return normalizePlaygroundError(primary, status);
		}
		return normalizePlaygroundError(primary, status);
	}
	if (trailingSSE != null) {
		return normalizePlaygroundError(trailingSSE, status);
	}
	return {
		type: "http_error",
		message: text,
		param: null,
		code: String(status),
	};
}

function parseErrorBody(status: number, text: string): PlaygroundError {
	return parsePlaygroundErrorBody(status, text);
}

function isRoutingMarkerContent(content: string): boolean {
	return content.startsWith(ROUTING_MARKER_PREFIX);
}

function extractStreamDelta(payload: string): string | null {
	if (!payload.startsWith("{")) {
		return payload;
	}
	try {
		const parsed = JSON.parse(payload) as {
			error?: PlaygroundError;
			model?: string;
			provider?: string;
			reason?: string;
			choices?: Array<{ delta?: { content?: string } }>;
		};
		if (parsed.error) {
			return null;
		}
		if (typeof parsed.model === "string" && typeof parsed.provider === "string") {
			return null;
		}
		const content = parsed.choices?.[0]?.delta?.content;
		if (typeof content !== "string" || content.length === 0) {
			return null;
		}
		if (isRoutingMarkerContent(content)) {
			return null;
		}
		return content;
	} catch {
		return payload;
	}
}

function processEventBlock(
	block: string,
	onDelta: (delta: string) => void,
	onDecision: (decision: PlaygroundDecision) => void,
	onError: (error: PlaygroundError) => void,
): boolean {
	const lines = block.split("\n");
	let eventName = "";
	const dataParts: string[] = [];

	for (const line of lines) {
		if (line.startsWith("event:")) {
			eventName = line.slice(6).trim();
			continue;
		}
		if (line.startsWith("data:")) {
			dataParts.push(line.slice(5).trimStart());
		}
	}

	if (dataParts.length === 0) return false;
	const payload = dataParts.join("");
	if (payload === "[DONE]") return true;

	if (eventName === "routing_metadata" || payload.startsWith("{")) {
		try {
			const parsed = JSON.parse(payload) as Record<string, unknown>;
			if (typeof parsed.model === "string" && typeof parsed.provider === "string") {
				onDecision(normalizeDecision(parsed as Parameters<typeof normalizeDecision>[0]));
				return false;
			}
			if (parsed.error) {
				onError(
					normalizePlaygroundError(parsed.error as Partial<PlaygroundError>, 0),
				);
				return true;
			}
			if (
				typeof parsed.type === "string" &&
				typeof parsed.message === "string" &&
				(parsed.code === "upstream_interrupted" || parsed.type === "api_error")
			) {
				onError(
					normalizePlaygroundError(parsed as Partial<PlaygroundError>, 0),
				);
				return true;
			}
		} catch {
			// Fall through to delta extraction.
		}
	}

	const delta = extractStreamDelta(payload);
	if (delta != null && delta.length > 0) {
		onDelta(delta);
	}
	return false;
}

export async function consumeSSEResponse({
	response,
	signal,
	onDelta,
	onDecision,
	onError,
	onDone,
}: SSEResponseOptions): Promise<void> {
	if (!response.ok) {
		const text = await response.text();
		onError(parseErrorBody(response.status, text));
		return;
	}

	const reader = response.body?.getReader();
	if (!reader) {
		onDone();
		return;
	}

	const decoder = new TextDecoder();
	let buffer = "";

	const flush = () => {
		let boundary = buffer.indexOf("\n\n");
		while (boundary !== -1) {
			const block = buffer.slice(0, boundary);
			buffer = buffer.slice(boundary + 2);
			if (block.trim() && processEventBlock(block, onDelta, onDecision, onError)) {
				onDone();
				return true;
			}
			boundary = buffer.indexOf("\n\n");
		}
		return false;
	};

	try {
		while (true) {
			if (signal?.aborted) break;
			const { done, value } = await reader.read();
			if (done) break;
			buffer += decoder.decode(value, { stream: true });
			if (flush()) return;
		}
		buffer += decoder.decode();
		if (buffer.includes("[DONE]")) {
			const before = buffer.split("[DONE]")[0];
			if (before.trim()) processEventBlock(before, onDelta, onDecision, onError);
			onDone();
			return;
		}
		if (buffer.trim() && processEventBlock(buffer, onDelta, onDecision, onError)) {
			onDone();
			return;
		}
	} catch (err) {
		if (signal?.aborted) {
			onDone();
			return;
		}
		throw err;
	}

	if (signal?.aborted) {
		onDone();
		return;
	}
	onDone();
}

export async function streamChat({
	url,
	body,
	signal,
	onDelta,
	onDecision,
	onError,
	onDone,
}: SSEOptions): Promise<void> {
	const response = await fetch(url, {
		method: "POST",
		headers: { "Content-Type": "application/json" },
		body: JSON.stringify(body),
		signal,
		credentials: "include",
	});

	return consumeSSEResponse({
		response,
		signal,
		onDelta,
		onDecision,
		onError,
		onDone,
	});
}
