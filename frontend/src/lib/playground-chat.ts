import type { AiandModel } from "@/lib/api";
import { api } from "@/lib/api";
import { clampReasoningEffortForChat } from "@/lib/playground-reasoning";
import { consumeSSEResponse, parsePlaygroundErrorBody } from "@/lib/playground-sse";
import type { PlaygroundModel, PlaygroundParams } from "@/lib/playground-store";

export function buildChatBody(
	messages: Array<{ role: "user" | "assistant"; content: string }>,
	model: PlaygroundModel,
	params: PlaygroundParams,
	models: AiandModel[] = [],
) {
	const reasoning = clampReasoningEffortForChat(params.reasoning, model, models);
	return {
		model: model ?? "auto",
		messages,
		stream: params.stream ?? true,
		...(params.temperature != null ? { temperature: params.temperature } : {}),
		...(params.max_tokens != null ? { max_tokens: params.max_tokens } : {}),
		...(reasoning != null ? { reasoning_effort: reasoning } : {}),
	};
}

async function consumeJsonResponse(
	response: Response,
	opts: Pick<
		Parameters<typeof runPlaygroundChat>[0],
		"onDelta" | "onError" | "onDone"
	>,
) {
	if (!response.ok) {
		const text = await response.text();
		opts.onError(parsePlaygroundErrorBody(response.status, text));
		return;
	}
	const json = (await response.json()) as {
		choices?: Array<{ message?: { content?: string } }>;
	};
	const content = json.choices?.[0]?.message?.content ?? "";
	if (content) opts.onDelta(content);
	opts.onDone();
}

export async function runPlaygroundChat(opts: {
	messages: Array<{ role: "user" | "assistant"; content: string }>;
	model: PlaygroundModel;
	params: PlaygroundParams;
	models?: AiandModel[];
	sessionID?: string;
	signal: AbortSignal;
	onDelta: (delta: string) => void;
	onDecision: Parameters<typeof consumeSSEResponse>[0]["onDecision"];
	onError: Parameters<typeof consumeSSEResponse>[0]["onError"];
	onDone: () => void;
}) {
	const body = buildChatBody(opts.messages, opts.model, opts.params, opts.models ?? []);
	const response = await api.playground.chat(body, {
		signal: opts.signal,
		forceModel: opts.model,
		sessionID: opts.sessionID,
	});
	if (body.stream === false) {
		await consumeJsonResponse(response, {
			onDelta: opts.onDelta,
			onError: opts.onError,
			onDone: opts.onDone,
		});
		return;
	}
	await consumeSSEResponse({
		response,
		signal: opts.signal,
		onDelta: opts.onDelta,
		onDecision: opts.onDecision,
		onError: opts.onError,
		onDone: opts.onDone,
	});
}
