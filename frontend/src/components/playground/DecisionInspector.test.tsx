import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { DecisionInspector } from "./DecisionInspector";

describe("DecisionInspector", () => {
	it("shows a human label instead of the raw cluster reason", () => {
		render(
			<DecisionInspector
				decision={{
					model: "deepseek-ai/deepseek-v4-flash",
					provider: "aiand",
					reason:
						"cluster:v0.76 top_p=[9,10,11,15] model=deepseek-ai/deepseek-v4-flash provider=aiand",
					requestedCostUsd: 0.01,
					actualCostUsd: 0.008,
					id: "req-1",
					cacheSavingsUsd: 0,
				}}
			/>,
		);

		expect(screen.getByText("Auto-routed")).toBeInTheDocument();
		expect(screen.queryByText(/top_p=/)).not.toBeInTheDocument();
	});
});
