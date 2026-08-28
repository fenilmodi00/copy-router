import { describe, expect, it } from "vitest";
import type { AiandModel } from "@/lib/api";
import { buildChatBody } from "@/lib/playground-chat";

const motif3: AiandModel = {
	id: "motif-technologies/motif-3",
	provider: "motif-technologies",
	context_window: 262_144,
	capabilities: ["reasoning", "tool_calling"],
	reasoning_efforts: ["low", "medium", "high"],
	reasoning_effort_default: "medium",
	input_per_1m: "0.50",
	output_per_1m: "2.00",
	cached_input_per_1m: "0.20",
	currency: "usd",
};

const flash: AiandModel = {
	id: "deepseek-ai/deepseek-v4-flash",
	provider: "deepseek-ai",
	context_window: 1_000_000,
	capabilities: [],
	reasoning_efforts: ["none", "max"],
	reasoning_effort_default: "none",
	input_per_1m: "0.1",
	output_per_1m: "0.2",
	cached_input_per_1m: "0.05",
	currency: "usd",
};

describe("buildChatBody reasoning_effort", () => {
	it("clamps unsupported none to low for motif-3", () => {
		const body = buildChatBody(
			[{ role: "user", content: "hi" }],
			"motif-technologies/motif-3",
			{ reasoning: "none" },
			[motif3, flash],
		);
		expect(body.reasoning_effort).toBe("low");
	});

	it("omits reasoning_effort for auto route", () => {
		const body = buildChatBody(
			[{ role: "user", content: "hi" }],
			null,
			{ reasoning: "none" },
			[motif3, flash],
		);
		expect(body).not.toHaveProperty("reasoning_effort");
	});

	it("keeps a supported effort for forced models", () => {
		const body = buildChatBody(
			[{ role: "user", content: "hi" }],
			"motif-technologies/motif-3",
			{ reasoning: "high" },
			[motif3],
		);
		expect(body.reasoning_effort).toBe("high");
	});
});
