import { render, screen, within } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { SWRConfig } from "swr";

// Route-level ?ids= deep-link test on the real compare page: the page owns
// the URL hydration (basket.add per URL id, setHydrated(true)) in a mount
// effect, so this test drives the REAL store + compare page and asserts the
// rendered verdicts against hand-computed costs.
//
// Hand-computed sample costs (15K in + 35K out, CACHE_HIT_RATE = 0.7):
//   A in 0.15/out 0.25/cached 0.08 -> plain (2250+8750)/1e6 = 0.011,
//     cached (675+840+8750)/1e6 = 0.010265
//   B in 0.30/out 0.50/cached 0.20 -> plain 0.022, cached 0.02095
//   C in 0.40/out 0.60/cached 0.30 -> plain 0.027, cached 0.02595
// Green-tint: the no-cache column tints the cheapest 3 (here all 3); the
// cached column tints only the minimum (A).

vi.mock("next/link", () => ({
  default: (props: React.AnchorHTMLAttributes<HTMLAnchorElement>) => <a {...props} />,
}));

const { models } = vi.hoisted(() => ({
  models: [
    {
      id: "model-a",
      provider: "acme",
      context_window: 1_000_000,
      capabilities: ["reasoning"],
      reasoning_efforts: ["none"],
      reasoning_effort_default: "none",
      input_per_1m: "0.15",
      output_per_1m: "0.25",
      cached_input_per_1m: "0.08",
      currency: "usd",
    },
    {
      id: "model-b",
      provider: "acme",
      context_window: 131_072,
      capabilities: ["chat"],
      reasoning_efforts: ["none"],
      reasoning_effort_default: "none",
      input_per_1m: "0.30",
      output_per_1m: "0.50",
      cached_input_per_1m: "0.20",
      currency: "usd",
    },
    {
      id: "model-c",
      provider: "acme",
      context_window: 262_144,
      capabilities: ["chat"],
      reasoning_efforts: ["none"],
      reasoning_effort_default: "none",
      input_per_1m: "0.40",
      output_per_1m: "0.60",
      cached_input_per_1m: "0.30",
      currency: "usd",
    },
  ],
}));

vi.mock("@/lib/api", () => ({
  api: {
    aiandModels: { list: vi.fn().mockResolvedValue({ data: models }) },
    metrics: { modelBreakdown: vi.fn().mockResolvedValue({ buckets: [] }) },
  },
}));

describe("ComparePage ?ids= deep link", () => {
  beforeEach(() => {
    window.localStorage.clear();
    vi.resetModules();
    window.history.replaceState({}, "", "/models/compare");
  });

  it("hydrates the basket from a shared ?ids= URL and prices the sample", async () => {
    window.history.replaceState({}, "", "/models/compare?ids=model-a,model-b,model-c");
    const { default: ComparePage } = await import("./page");
    render(
      <SWRConfig value={{ provider: () => new Map(), dedupingInterval: 0 }}>
        <ComparePage />
      </SWRConfig>,
    );

    expect(await screen.findByText("model-a")).toBeInTheDocument();
    expect(screen.getByText("model-b")).toBeInTheDocument();
    expect(screen.getByText("model-c")).toBeInTheDocument();

    const noCacheRow = screen.getByText("Sample cost (15K in + 35K out)").closest("tr")!;
    expect(within(noCacheRow).getAllByText("$0.01")).toHaveLength(1);
    expect(within(noCacheRow).getAllByText("$0.02")).toHaveLength(1);
    expect(within(noCacheRow).getAllByText("$0.03")).toHaveLength(1);

    const cachedRow = screen.getByText("Sample cost @ 70% cache hit").closest("tr")!;
    const cachedCells = within(cachedRow).getAllByText(/\$0\.0[1-3]/);
    const tinted = cachedCells.filter(
      el => el.closest("span")?.className.includes("text-success"),
    );
    // Only the cheapest cached verdict (model-a) is green-tinted.
    expect(tinted).toHaveLength(1);
    expect(tinted[0]).toHaveTextContent("$0.01");
  });

  it("renders the empty state when no ids are in the URL", async () => {
    const { default: ComparePage } = await import("./page");
    render(
      <SWRConfig value={{ provider: () => new Map(), dedupingInterval: 0 }}>
        <ComparePage />
      </SWRConfig>,
    );
    expect(await screen.findByText(/No models selected/)).toBeInTheDocument();
  });
});