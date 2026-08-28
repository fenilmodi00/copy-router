import { describe, expect, it } from "vitest";
import { formatPlaygroundReason } from "./playground-reason";

describe("formatPlaygroundReason", () => {
	it("hides cluster scorer dumps", () => {
		expect(
			formatPlaygroundReason(
				"cluster:v0.76 top_p=[9,10,11,15] model=deepseek-ai/deepseek-v4-flash provider=aiand",
			),
		).toBe("Auto-routed");
	});

	it("passes through short human reasons", () => {
		expect(formatPlaygroundReason("Forced model")).toBe("Forced model");
	});

	it("labels empty reasons with the served model", () => {
		expect(formatPlaygroundReason("", "deepseek-ai/deepseek-v4-flash")).toBe(
			"Routed to deepseek-ai/deepseek-v4-flash",
		);
	});
});
