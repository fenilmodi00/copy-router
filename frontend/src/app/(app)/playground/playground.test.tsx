import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { SWRConfig } from "swr";

const { runPlaygroundChat, lastChatArgs } = vi.hoisted(() => ({
	runPlaygroundChat: vi.fn(),
	lastChatArgs: { value: null as Record<string, unknown> | null },
}));

vi.mock("@/lib/playground-chat", () => ({
	runPlaygroundChat: (...args: unknown[]) => {
		lastChatArgs.value = args[0] as Record<string, unknown>;
		return runPlaygroundChat(...args);
	},
	buildChatBody: (messages: unknown, model: unknown, params: unknown) => ({
		messages,
		model: model ?? "auto",
		stream: true,
		...((params as object) ?? {}),
	}),
}));

vi.mock("@/lib/data-cache", () => ({
	useCatalog: () => ({
		data: [
			{
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
			},
		],
		isLoading: false,
	}),
}));

vi.mock("@/lib/use-login-session-gate", () => ({
	useLoginSession: () => ({ state: "authed", surface: "admin" }),
}));

vi.mock("@/lib/api", async (importOriginal) => {
	const actual = await importOriginal<typeof import("@/lib/api")>();
	return {
		...actual,
		api: {
			...actual.api,
			auth: {
				...actual.api.auth,
				me: vi.fn().mockResolvedValue({ authenticated: true, subject: "Admin User" }),
				accountMe: vi.fn().mockResolvedValue({ authenticated: false }),
			},
		},
	};
});

vi.mock("@/components/Page", () => ({
	Page: ({
		children,
		header,
		subheader,
	}: {
		children: React.ReactNode;
		header?: React.ReactNode;
		subheader?: React.ReactNode;
	}) => (
		<div>
			{header}
			{subheader}
			{children}
		</div>
	),
}));

import PlaygroundPage from "./page";
import { STORAGE_KEY, usePlaygroundStore } from "@/lib/playground-store";

describe("Playground page", () => {
	beforeEach(() => {
		window.localStorage.clear();
		window.localStorage.setItem(
			STORAGE_KEY,
			JSON.stringify({ messages: [], model: null, sessionID: "sess_test" }),
		);
		vi.clearAllMocks();
		usePlaygroundStore.setState({
			messages: [],
			status: "idle",
			model: null,
			params: {
				temperature: 1,
				max_tokens: 4096,
				reasoning: "max",
				stream: true,
			},
			sessionID: "sess_test",
			abortController: null,
		});
		runPlaygroundChat.mockImplementation(async (opts) => {
			opts.onDelta("Hello");
			opts.onDelta(" world");
			opts.onDecision({
				model: "deepseek-ai/deepseek-v4-flash",
				provider: "deepseek-ai",
				reason: "best fit",
				requestedCostUsd: 0.12,
				actualCostUsd: 0.08,
				id: "req_1",
				cacheSavingsUsd: 0.03,
			});
			opts.onDone();
		});
	});

	function renderPage() {
		return render(
			<SWRConfig value={{ provider: () => new Map() }}>
				<PlaygroundPage />
			</SWRConfig>,
		);
	}

	it("renders the empty state", () => {
		renderPage();
		expect(screen.getByText("Send a message to run the router.")).toBeInTheDocument();
	});

	it("renders the control panel sliders and stream toggle", () => {
		renderPage();
		expect(screen.getByText("Control Panel")).toBeInTheDocument();
		expect(screen.getByLabelText("Temperature")).toBeInTheDocument();
		expect(screen.getByLabelText("Max Tokens")).toBeInTheDocument();
		expect(screen.getByLabelText("Reasoning effort")).toBeInTheDocument();
		expect(screen.getByRole("switch", { name: "Stream" })).toHaveAttribute(
			"aria-checked",
			"true",
		);
	});

	it("model selector default reads Auto route", () => {
		renderPage();
		expect(screen.getByLabelText("Model selector")).toHaveValue("");
	});

	it("submitting a message calls runPlaygroundChat with model auto by default", async () => {
		renderPage();
		fireEvent.change(screen.getByLabelText("Message composer"), {
			target: { value: "hello" },
		});
		fireEvent.click(screen.getByRole("button", { name: "Send" }));

		await waitFor(() => expect(runPlaygroundChat).toHaveBeenCalled());
		expect(lastChatArgs.value?.model).toBeNull();
		expect(lastChatArgs.value?.sessionID).toBe("sess_test");
	});

	it("submitting uses the forced model when selected", async () => {
		renderPage();
		fireEvent.change(screen.getByLabelText("Model selector"), {
			target: { value: "deepseek-ai/deepseek-v4-flash" },
		});
		fireEvent.change(screen.getByLabelText("Message composer"), {
			target: { value: "hello" },
		});
		fireEvent.click(screen.getByRole("button", { name: "Send" }));

		await waitFor(() => expect(runPlaygroundChat).toHaveBeenCalled());
		expect(lastChatArgs.value?.model).toBe("deepseek-ai/deepseek-v4-flash");
	});

	it("assistant delta paints incrementally", async () => {
		renderPage();
		fireEvent.change(screen.getByLabelText("Message composer"), {
			target: { value: "hello" },
		});
		fireEvent.click(screen.getByRole("button", { name: "Send" }));

		await waitFor(() =>
			expect(screen.getByText("Hello world")).toBeInTheDocument(),
		);
	});

	it("decision inspector shows model and both cost dollars", async () => {
		renderPage();
		fireEvent.change(screen.getByLabelText("Message composer"), {
			target: { value: "hello" },
		});
		fireEvent.click(screen.getByRole("button", { name: "Send" }));

		await waitFor(() =>
			expect(screen.getAllByText(/deepseek-ai\/deepseek-v4-flash/).length).toBeGreaterThan(
				1,
			),
		);
		expect(screen.getByText(/Requested: \$0\.12/)).toBeInTheDocument();
		expect(screen.getByText(/Actual: \$0\.08/)).toBeInTheDocument();
		expect(screen.getByText(/Cache savings: \$0\.03/)).toBeInTheDocument();
	});

	it("view code toggle reveals the request preview", () => {
		renderPage();
		fireEvent.click(screen.getByRole("switch", { name: "View Code" }));
		expect(
			screen.getByText(/Equivalent request body for the current playground settings/i),
		).toBeInTheDocument();
	});

	it("error panel surfaces the classified envelope and Retry re-sends", async () => {
		runPlaygroundChat.mockImplementation(async (opts) => {
			opts.onError({
				type: "envelope_error",
				message: "upstream failed",
				param: "model",
				code: "502",
			});
		});

		renderPage();
		fireEvent.change(screen.getByLabelText("Message composer"), {
			target: { value: "hello" },
		});
		fireEvent.click(screen.getByRole("button", { name: "Send" }));

		await waitFor(() =>
			expect(screen.getByText("upstream failed")).toBeInTheDocument(),
		);

		runPlaygroundChat.mockClear();
		fireEvent.click(screen.getByRole("button", { name: "Retry" }));

		await waitFor(() => expect(runPlaygroundChat).toHaveBeenCalledTimes(1));
	});

	it("shows served model when routing_metadata arrives before deltas", async () => {
		runPlaygroundChat.mockImplementation(async (opts) => {
			opts.onDecision({
				model: "deepseek-ai/deepseek-v4-flash",
				provider: "aiand",
				reason: "cluster pick",
				requestedCostUsd: 0.01,
				actualCostUsd: 0.008,
				id: "req_early",
				cacheSavingsUsd: 0,
			});
			opts.onDelta("Routed reply");
			opts.onDone();
		});

		renderPage();
		fireEvent.change(screen.getByLabelText("Message composer"), {
			target: { value: "hello" },
		});
		fireEvent.click(screen.getByRole("button", { name: "Send" }));

		await waitFor(() =>
			expect(screen.getByText("Routed reply")).toBeInTheDocument(),
		);
		expect(screen.getAllByText(/deepseek-ai\/deepseek-v4-flash/).length).toBeGreaterThan(0);
		expect(screen.getAllByText("aiand").length).toBeGreaterThan(0);
	});

	it("Stop mid-stream preserves partial text", async () => {
		runPlaygroundChat.mockImplementation(async (opts) => {
			opts.onDelta("partial");
			await new Promise(() => {});
		});

		renderPage();
		fireEvent.change(screen.getByLabelText("Message composer"), {
			target: { value: "hello" },
		});
		fireEvent.click(screen.getByRole("button", { name: "Send" }));

		await waitFor(() => expect(screen.getByText("partial")).toBeInTheDocument());
		fireEvent.click(screen.getByRole("button", { name: "Stop" }));
		expect(screen.getByText("partial")).toBeInTheDocument();
	});

	it("shows the file upload notice in the composer", () => {
		renderPage();
		expect(
			screen.getByText(/This model does not support file uploads/i),
		).toBeInTheDocument();
	});
});
