import { describe, expect, it } from "vitest";
import type { AiandModel } from "@/lib/api";
import {
	clampReasoningEffortForChat,
	normalizeReasoningForModel,
} from "@/lib/playground-reasoning";

const motif3: AiandModel = {
	id: "motif-technologies/motif-3",
	provider: "motif-technologies",
	context_window: 262_144,
	capabilities: ["reasoning"],
	reasoning_efforts: ["low", "medium", "high"],
	reasoning_effort_default: "medium",
	input_per_1m: "0.50",
	output_per_1m: "2.00",
	cached_input_per_1m: "0.20",
	currency: "usd",
};

describe("playground-reasoning", () => {
	it("clamps none to low for motif-3", () => {
		expect(clampReasoningEffortForChat("none", "motif-technologies/motif-3", [motif3])).toBe(
			"low",
		);
	});

	it("omits effort for auto route", () => {
		expect(clampReasoningEffortForChat("none", null, [motif3])).toBeUndefined();
	});

	it("resets invalid stored effort when model changes", () => {
		expect(normalizeReasoningForModel("none", "motif-technologies/motif-3", [motif3])).toBe(
			"low",
		);
	});
});
