import { beforeEach, describe, expect, it, vi } from "vitest";
import { api, mapPlaygroundRouteResponse } from "./api";

describe("api.playground", () => {
	beforeEach(() => {
		vi.unstubAllGlobals();
	});

	it("route parses the playground response keys", async () => {
		const payload = {
			model: "deepseek-ai/deepseek-v4-flash",
			provider: "deepseek-ai",
			reason: "best fit",
			requested_cost_usd: 0.12,
			actual_cost_usd: 0.08,
			id: "req_123",
			cache_savings_usd: 0.02,
		};

		vi.stubGlobal(
			"fetch",
			vi.fn(() =>
				Promise.resolve({
					ok: true,
					status: 200,
					json: async () => payload,
				}),
			),
		);

		const res = await api.playground.route({ messages: [] });
		expect(res).toEqual(payload);
		expect(mapPlaygroundRouteResponse(res).cacheSavingsUsd).toBe(0.02);
	});

	it("route tolerates missing cache_savings_usd", async () => {
		const payload = {
			model: "m",
			provider: "p",
			reason: "r",
			requested_cost_usd: 0,
			actual_cost_usd: 0,
			id: "id1",
		};

		vi.stubGlobal(
			"fetch",
			vi.fn(() =>
				Promise.resolve({
					ok: true,
					status: 200,
					json: async () => payload,
				}),
			),
		);

		const res = await api.playground.route({ messages: [] });
		expect(mapPlaygroundRouteResponse(res).cacheSavingsUsd).toBe(0);
	});

	it("chat wires AbortSignal through fetch", async () => {
		const controller = new AbortController();
		const fetchMock = vi.fn(() =>
			Promise.resolve({
				ok: true,
				status: 200,
				body: null,
			}),
		);
		vi.stubGlobal("fetch", fetchMock);

		await api.playground.chat({ messages: [] }, { signal: controller.signal });

		expect(fetchMock).toHaveBeenCalledWith(
			"/v1/playground/chat",
			expect.objectContaining({ signal: controller.signal }),
		);
	});

	it("chat sends x-weave-force-model when forceModel is set", async () => {
		const fetchMock = vi.fn(() =>
			Promise.resolve({
				ok: true,
				status: 200,
				body: null,
			}),
		);
		vi.stubGlobal("fetch", fetchMock);

		await api.playground.chat(
			{ messages: [] },
			{ forceModel: "deepseek-ai/deepseek-v4-flash" },
		);

		expect(fetchMock).toHaveBeenCalledWith(
			"/v1/playground/chat",
			expect.objectContaining({
				headers: expect.objectContaining({
					"x-weave-force-model": "deepseek-ai/deepseek-v4-flash",
				}),
			}),
		);
	});

	it("chat sends X-Playground-Session when sessionID is set", async () => {
		const fetchMock = vi.fn(() =>
			Promise.resolve({
				ok: true,
				status: 200,
				body: null,
			}),
		);
		vi.stubGlobal("fetch", fetchMock);

		await api.playground.chat(
			{ messages: [] },
			{ sessionID: "sess_abc123" },
		);

		expect(fetchMock).toHaveBeenCalledWith(
			"/v1/playground/chat",
			expect.objectContaining({
				headers: expect.objectContaining({
					"X-Playground-Session": "sess_abc123",
				}),
			}),
		);
	});
});
