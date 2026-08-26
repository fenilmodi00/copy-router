import { fireEvent, render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

// ... dashboard regression test description ...

import { Text } from "@/components/atoms/Text";
import { TextProps } from "@/components/atoms/Text";

const { lastHref } = vi.hoisted(() => ({ lastHref: { value: "" } }));

// BasePath-aware next/link stand-in: Next's real <Link> prefixes the /ui
// basePath onto internal hrefs; the mocked anchor keeps that behavior so the
// basePath assertions exercise the real seam (raw <a href> does NOT prefix).
vi.mock("next/link", () => ({
  default: ({ href, ...rest }: React.AnchorHTMLAttributes<HTMLAnchorElement>) => {
    const full = typeof href === "string" && href.startsWith("/") && !href.startsWith("/ui") ? `/ui${href}` : href;
    lastHref.value = full ?? "";
    return <a href={full} {...rest} />;
  },
}));

let sparklineIndex = 0;

vi.mock("@/components/molecules/Sparkline", () => ({
  Sparkline: ({ data }: { data: number[] }) => {
    const idx = sparklineIndex++;
    return (
      <span data-testid="sparkline" data-index={idx} data-values={JSON.stringify(data)}>
        sparkline
      </span>
    );
  },
}));

const { push } = vi.hoisted(() => ({ push: vi.fn() }));

vi.mock("next/navigation", () => ({
  useRouter: () => ({ push, replace: vi.fn(), back: vi.fn(), prefetch: vi.fn() }),
}));

const { api, onSkip, onComplete } = vi.hoisted(() => {
  const api = {
    onboarding: {
      get: vi.fn().mockResolvedValue({ first_request_served_at: "2026-08-20T00:00:00Z" }),
    },
    metrics: {
      summary: vi.fn().mockResolvedValue({
        request_count: 10,
        total_tokens: 47_000,
        total_requested_cost_usd: 2.35,
        total_actual_cost_usd: 1.17,
        total_savings_usd: 1.18,
        cache_write_tokens: 0,
        cache_read_tokens: 0,
      }),
      timeseries: vi.fn().mockResolvedValue({
        buckets: [
          { bucket: "2026-08-27T00:00:00Z", requested_cost_usd: 2.0, actual_cost_usd: 1.0, total_tokens: 40_000, request_count: 8 },
          { bucket: "2026-08-27T01:00:00Z", requested_cost_usd: 0.35, actual_cost_usd: 0.17, total_tokens: 7_000, request_count: 2 },
        ],
      }),
      modelBreakdown: vi.fn().mockResolvedValue({ buckets: [] }),
      details: vi.fn().mockResolvedValue({
        rows: [
          {
            timestamp: "2026-08-27T00:00:00Z",
            request_id: "req-1",
            requested_model: "deepseek-ai/deepseek-v4-flash",
            decision_model: "deepseek-ai/deepseek-v4-flash",
            decision_provider: "deepseek-ai",
            decision_reason: "scored",
            sticky_hit: false,
            input_tokens: 40_000,
            output_tokens: 0,
            cache_creation_tokens: null,
            cache_read_tokens: null,
            requested_cost_usd: 2.0,
            actual_cost_usd: 1.0,
            total_latency_ms: 120,
            upstream_status_code: 200,
            router_user_id: "user-1",
            client_app: "cli",
            turn_type: "main_loop",
            user_email: "",
          },
        ],
      }),
    },
    aiandModels: {
      list: vi.fn().mockResolvedValue({
        data: [{ id: "deepseek-ai/deepseek-v4-flash", provider: "deepseek-ai", context_window: 1_000_000, capabilities: ["reasoning"], reasoning_efforts: ["none"], reasoning_effort_default: "none", input_per_1m: "0.15", output_per_1m: "0.25", cached_input_per_1m: "0.08", currency: "usd" }],
      }),
    },
  };
  const onSkip = vi.fn();
  const onComplete = vi.fn();
  return { api, onSkip, onComplete };
});

vi.mock("@/lib/api", () => ({ api }));

// Route-level test: mock the full dashboard filter bar (Popover/Command are
// heavy) and the RouterOnboarding panel; everything else renders for real.
vi.mock("@/components/DashboardPageFilters", () => {
  const DashboardPageFilters = () => <div>filters</div>;
  const useDashboardFilters = () => ({
    filters: {
      fromISO: "2026-08-01T00:00:00Z",
      toISO: "2026-08-27T00:00:00Z",
      granularity: "day",
      range: { id: "last-month", label: "Last month" },
    },
    setRangeId: vi.fn(),
    setGranularity: vi.fn(),
  });
  return { DashboardPageFilters, useDashboardFilters };
});

vi.mock("@/components/RouterOnboarding", () => ({
  default: () => <div>onboarding</div>,
}));

import DashboardPage from "./page";

// Reads the JSON'd sparkline arrays back off the mocked Sparkline spans — the
// card passes its series straight through, so this asserts exactly what each
// KPI card is fed.
function sparklineValues(): number[][] {
  const spans = Array.from(document.querySelectorAll<HTMLElement>('[data-testid="sparkline"]'));
  return spans
    .sort((a, b) => Number(a.dataset.index) - Number(b.dataset.index))
    .map(el => JSON.parse(el.dataset.values ?? "[]") as number[]);
}

// token/request/cost series are the first three KPI cards, in render order.
function series(): { tokens: number[]; requests: number[]; costs: number[] } {
  const [tokens, requests, costs] = sparklineValues();
  return { tokens: tokens ?? [], requests: requests ?? [], costs: costs ?? [] };
}

function isFlat(a: number[]): boolean {
  return a.length < 2 || new Set(a).size <= 1;
}

describe("DashboardPage bugfix regression", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    lastHref.value = "";
    sparklineIndex = 0;
  });

  it("feeds the Tokens KPI the per-bucket token series, not the cost curve — non-flat at tiny spend", async () => {
    render(<DashboardPage />);
    // The sparkline series land in the same async settle as the spend table
    // rows (metrics effect), so wait on a post-effect DOM signal first.
    await screen.findByRole("link", { name: "deepseek-ai/deepseek-v4-flash" });

    const { tokens } = series();
    // Tokens must come from total_tokens: the fixture's buckets hold 40K vs
    // 7K tokens but near-zero dollars, so a cost-fed Tokens card would be a
    // flat two-point line. A tokens-fed card must not be.
    expect(tokens.length).toBeGreaterThanOrEqual(2);
    expect(isFlat(tokens)).toBe(false);
  });

  it("feeds the Requests KPI request_count and the Actual-cost KPI actual cost", async () => {
    render(<DashboardPage />);
    await screen.findByText("Overview");
    await screen.findByRole("link", { name: "deepseek-ai/deepseek-v4-flash" });

    const { requests, costs } = series();
    // Requests must come from request_count ([8, 2]); the old code plotted
    // requested cost on this card (a near-flat [2.0, 0.35] at this spend).
    expect(requests).toEqual([8, 2]);
    expect(costs).toEqual([1, 0.17]);
  });

  it("renders the spend-table model link with the /ui basePath", async () => {
    render(<DashboardPage />);
    await screen.findByText("Overview");
    const link = await screen.findByRole("link", { name: "deepseek-ai/deepseek-v4-flash" });
    expect(link).toHaveAttribute("href", "/ui/models/deepseek-ai~deepseek-v4-flash");
  });

  it("pushes the Popularity leaderboard selection via basePath-aware router.push", async () => {
    render(<DashboardPage />);
    await screen.findByRole("link", { name: "deepseek-ai/deepseek-v4-flash" });
    // The leaderboard renders one row button per model (name + token count);
    // clicking it must route through basePath-aware router.push.
    const row = screen.getByRole("button", { name: /deepseek-ai\/deepseek-v4-flash.*tok/ });
    fireEvent.click(row);
    expect(push).toHaveBeenCalled();
    // route() is next 15's (url, options) signature; url is always the string.
    const call = push.mock.calls[push.mock.calls.length - 1];
    const url = typeof call === "string" ? call : call[0];
    expect(url).toMatch(/^\/models\/deepseek-ai~deepseek-v4-flash$/);
  });
});