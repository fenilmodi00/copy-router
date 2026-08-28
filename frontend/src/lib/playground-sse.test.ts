import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { parsePlaygroundErrorBody, streamChat } from "./playground-sse";
import type { PlaygroundError } from "./playground-store";

function mockStreamResponse(chunks: string[], status = 200) {
	const encoded = chunks.map((c) => new TextEncoder().encode(c));
	let index = 0;
	return {
		ok: status >= 200 && status < 300,
		status,
		body: {
			getReader: () => ({
				read: () => {
					if (index >= encoded.length) {
						return Promise.resolve({ done: true, value: undefined });
					}
					const value = encoded[index++];
					return Promise.resolve({ done: false, value });
				},
			}),
		},
		text: async () => chunks.join(""),
	};
}

describe("playground-sse", () => {
	const makeError = (params: Partial<PlaygroundError> = {}): PlaygroundError => ({
		type: "http_error",
		message: "",
		param: null,
		code: "",
		...params,
	});

	beforeEach(() => {
		vi.useFakeTimers();
	});

	afterEach(() => {
		vi.useRealTimers();
		vi.unstubAllGlobals();
	});

	it("parses OpenAI chat.completion.chunk delta content", async () => {
		const onDelta = vi.fn();
		const onDone = vi.fn();
		const onError = vi.fn();

		const chunk =
			'data: {"id":"cmpl-1","choices":[{"index":0,"delta":{"content":"Hello"}}]}\n\n' +
			"data: [DONE]\n\n";
		vi.stubGlobal("fetch", vi.fn(() => Promise.resolve(mockStreamResponse([chunk]))));

		await streamChat({
			url: "http://example.com/chat",
			body: { prompt: "hi" },
			onDelta,
			onDecision: vi.fn(),
			onError,
			onDone,
		});

		expect(onDelta).toHaveBeenCalledWith("Hello");
		expect(onDone).toHaveBeenCalled();
	});

	it("skips routing marker chunks in OpenAI stream", async () => {
		const onDelta = vi.fn();
		const chunk =
			'data: {"choices":[{"delta":{"content":"✦ **Weave Router** → gpt-4o"}}]}\n\n' +
			'data: {"choices":[{"delta":{"content":"Hi"}}]}\n\n' +
			"data: [DONE]\n\n";
		vi.stubGlobal("fetch", vi.fn(() => Promise.resolve(mockStreamResponse([chunk]))));

		await streamChat({
			url: "http://example.com/chat",
			body: { prompt: "hi" },
			onDelta,
			onDecision: vi.fn(),
			onError: vi.fn(),
			onDone: vi.fn(),
		});

		expect(onDelta).toHaveBeenCalledTimes(1);
		expect(onDelta).toHaveBeenCalledWith("Hi");
	});

	it("accumulates deltas from chunked fake stream", async () => {
		const deltas: string[] = [];
		const onDelta = vi.fn((d) => deltas.push(d));
		const onDone = vi.fn();
		const onError = vi.fn();

		vi.stubGlobal(
			"fetch",
			vi.fn(() =>
				Promise.resolve(
					mockStreamResponse(["data: hello\n\ndata: ", "world\n\n", "data: [DONE]\n\n"]),
				),
			),
		);

		await streamChat({
			url: "http://example.com/chat",
			body: { prompt: "hi" },
			onDelta,
			onDecision: vi.fn(),
			onError,
			onDone,
		});

		expect(deltas).toEqual(["hello", "world"]);
		expect(onDone).toHaveBeenCalled();
		expect(onError).not.toHaveBeenCalled();
	});

	it("honors [DONE] marker and resolves", async () => {
		const onDone = vi.fn();
		const onError = vi.fn();
		const onDelta = vi.fn();

		vi.stubGlobal(
			"fetch",
			vi.fn(() =>
				Promise.resolve(mockStreamResponse(["data: partial data\n\ndata: [DONE]\n\n"])),
			),
		);

		await streamChat({
			url: "http://example.com/chat",
			body: { prompt: "hi" },
			onDelta,
			onDecision: vi.fn(),
			onError,
			onDone,
		});

		expect(onDelta).toHaveBeenCalledWith("partial data");
		expect(onDone).toHaveBeenCalled();
	});

	it("parses multi-line data payloads (blank lines and event: lines tolerated)", async () => {
		const onDelta = vi.fn();
		const onDone = vi.fn();
		const onError = vi.fn();

		const text =
			"event: ping\n\ndata: line one\ndata: line two\n\ndata: line three\n\n";
		vi.stubGlobal("fetch", vi.fn(() => Promise.resolve(mockStreamResponse([text]))));

		await streamChat({
			url: "http://example.com/chat",
			body: { prompt: "hi" },
			onDelta,
			onDecision: vi.fn(),
			onError,
			onDone,
		});

		expect(onDelta).toHaveBeenCalledWith("line oneline two");
		expect(onDelta).toHaveBeenCalledWith("line three");
	});

	it("aborts mid-stream and retains partial text", async () => {
		const onDone = vi.fn();
		const onError = vi.fn();
		const onDelta = vi.fn();
		const abortController = new AbortController();

		let readCount = 0;
		vi.stubGlobal(
			"fetch",
			vi.fn(() =>
				Promise.resolve({
					ok: true,
					status: 200,
					body: {
						getReader: () => ({
							read: () => {
								readCount += 1;
								if (readCount === 1) {
									return Promise.resolve({
										done: false,
										value: new TextEncoder().encode("data: partial chunk\n\n"),
									});
								}
								return new Promise(() => {});
							},
						}),
					},
				}),
			),
		);

		const promise = streamChat({
			url: "http://example.com/chat",
			body: { prompt: "hi" },
			signal: abortController.signal,
			onDelta,
			onDecision: vi.fn(),
			onError,
			onDone,
		});

		await Promise.resolve();
		abortController.abort();
		await promise;

		expect(onDelta).toHaveBeenCalledWith("partial chunk");
		expect(onDone).toHaveBeenCalled();
		expect(onError).not.toHaveBeenCalled();
	});

	it("surfaces HTTP error envelope as PlaygroundError", async () => {
		const onError = vi.fn();
		const onDone = vi.fn();
		const onDelta = vi.fn();

		const errorBody = JSON.stringify({
			error: {
				type: "envelope_error",
				message: "upstream failed",
				param: "model",
				code: "MODEL_UNAVAILABLE",
			},
		});

		vi.stubGlobal(
			"fetch",
			vi.fn(() =>
				Promise.resolve({
					ok: false,
					status: 502,
					text: async () => errorBody,
				}),
			),
		);

		await streamChat({
			url: "http://example.com/chat",
			body: { prompt: "hi" },
			onDelta,
			onDecision: vi.fn(),
			onError,
			onDone,
		});

		expect(onError).toHaveBeenCalledTimes(1);
		expect(onError).toHaveBeenCalledWith(
			makeError({
				type: "envelope_error",
				message: "upstream failed",
				param: "model",
				code: "MODEL_UNAVAILABLE",
			}),
		);
		expect(onDone).not.toHaveBeenCalled();
	});

	it("surfaces mashed buffered upstream JSON plus trailing SSE as one envelope", () => {
		const mashed = [
			'{"error":{"message":"Model motif-3 does not support reasoning_effort none","type":"invalid_request_error","param":"reasoning_effort","code":"invalid_value"}}',
			'data: {"code":"upstream_interrupted","message":"upstream returned status 400 (buffered)","param":null,"type":"api_error"}',
		].join("\n");

		expect(parsePlaygroundErrorBody(400, mashed)).toEqual({
			type: "invalid_request_error",
			message: "Model motif-3 does not support reasoning_effort none",
			param: "reasoning_effort",
			code: "invalid_value",
		});
	});

	it("parses routing_metadata SSE events into onDecision", async () => {
		const onDecision = vi.fn();
		const chunk =
			"event: routing_metadata\n" +
			'data: {"model":"deepseek-ai/deepseek-v4-flash","provider":"aiand","reason":"cluster","requested_cost_usd":0.01,"actual_cost_usd":0.008,"id":"req-1","cache_savings_usd":0}\n\n' +
			'data: {"choices":[{"delta":{"content":"Hi"}}]}\n\n' +
			"data: [DONE]\n\n";
		vi.stubGlobal("fetch", vi.fn(() => Promise.resolve(mockStreamResponse([chunk]))));

		await streamChat({
			url: "http://example.com/chat",
			body: { prompt: "hi" },
			onDelta: vi.fn(),
			onDecision,
			onError: vi.fn(),
			onDone: vi.fn(),
		});

		expect(onDecision).toHaveBeenCalledWith(
			expect.objectContaining({
				model: "deepseek-ai/deepseek-v4-flash",
				provider: "aiand",
			}),
		);
	});
});
