import { fireEvent, render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { SWRConfig } from "swr";

const { lastHref } = vi.hoisted(() => ({ lastHref: { value: "" } }));

vi.mock("next/link", () => ({
  default: ({ href, ...rest }: React.AnchorHTMLAttributes<HTMLAnchorElement>) => {
    const full =
      typeof href === "string" && href.startsWith("/") && !href.startsWith("/ui")
        ? `/ui${href}`
        : href;
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

const { api } = vi.hoisted(() => {
  const api = {
    onboarding: {
      get: vi.fn().mockResolvedValue({ first_request_served_at: "2026-08-20T00:00:00Z" }),
    },
    auth: {
      accountMe: vi.fn(),
      me: vi.fn(),
      logout: vi.fn(),
      accountLogout: vi.fn(),
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
          {
            bucket: "2026-08-27T00:00:00Z",
            requested_cost_usd: 2.0,
            actual_cost_usd: 1.0,
            total_tokens: 40_000,
            request_count: 8,
          },
          {
            bucket: "2026-08-27T01:00:00Z",
            requested_cost_usd: 0.35,
            actual_cost_usd: 0.17,
            total_tokens: 7_000,
            request_count: 2,
          },
        ],
      }),
      modelBreakdown: vi.fn().mockResolvedValue({
        buckets: [
          {
            bucket: "2026-08-27T00:00:00Z",
            decision_model: "deepseek-ai/deepseek-v4-flash",
            request_count: 1,
            total_tokens: 40_000,
            actual_cost_usd: 1.0,
          },
        ],
      }),
      details: vi.fn(),
    },
    aiandModels: {
      list: vi.fn().mockResolvedValue({
        data: [
          {
            id: "deepseek-ai/deepseek-v4-flash",
            provider: "deepseek-ai",
            context_window: 1_000_000,
            capabilities: ["reasoning"],
            reasoning_efforts: ["none"],
            reasoning_effort_default: "none",
            input_per_1m: "0.15",
            output_per_1m: "0.25",
            cached_input_per_1m: "0.08",
            currency: "usd",
          },
        ],
      }),
    },
  };
  return { api };
});

vi.mock("@/lib/api", () => ({ api }));

const { loginSession } = vi.hoisted(() => ({
  loginSession: { state: "authed" as const, surface: null as "account" | "admin" | null },
}));

vi.mock("@/lib/use-login-session-gate", () => ({
  useLoginSession: () => loginSession,
  useLoginSessionGate: () => loginSession,
  LoginSessionProvider: ({ children }: { children: React.ReactNode }) => children,
}));

vi.mock("@/components/DashboardPageFilters", () => {
  const DashboardPageFilters = () => <div>filters</div>;
  const useDashboardFilters = () => ({
    filters: {
      fromISO: "2026-08-26T15:30:00Z",
      toISO: "2026-08-27T15:30:00Z",
      granularity: "hour",
      range: { id: "last-24h", label: "Last 24 hours" },
    },
    setRangeId: vi.fn(),
    setGranularity: vi.fn(),
  });
  return { DashboardPageFilters, useDashboardFilters };
});

vi.mock("@/components/RouterOnboarding", () => ({
  RouterOnboarding: () => <div>onboarding</div>,
}));

import DashboardPage from "./page";

function sparklineValues(): number[][] {
  const spans = Array.from(document.querySelectorAll<HTMLElement>('[data-testid="sparkline"]'));
  return spans
    .sort((a, b) => Number(a.dataset.index) - Number(b.dataset.index))
    .map(el => JSON.parse(el.dataset.values ?? "[]") as number[]);
}

function series(): { tokens: number[]; requests: number[]; costs: number[] } {
  const [tokens, requests, costs] = sparklineValues();
  return { tokens: tokens ?? [], requests: requests ?? [], costs: costs ?? [] };
}

function isFlat(a: number[]): boolean {
  return a.length < 2 || new Set(a).size <= 1;
}

function renderDashboard() {
  // Isolate SWR cache per test so mocks don't leak across cases.
  return render(
    <SWRConfig value={{ provider: () => new Map(), dedupingInterval: 0 }}>
      <DashboardPage />
    </SWRConfig>,
  );
}

describe("DashboardPage bugfix regression", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    lastHref.value = "";
    sparklineIndex = 0;
    loginSession.surface = null;
    api.onboarding.get.mockResolvedValue({ first_request_served_at: "2026-08-20T00:00:00Z" });
  });

  it("skips harness onboarding for selfserve account sessions with no first request", async () => {
    loginSession.surface = "account";
    api.onboarding.get.mockResolvedValue({ first_request_served_at: null });

    renderDashboard();

    expect(await screen.findByText("Overview")).toBeInTheDocument();
    expect(screen.queryByText("onboarding")).not.toBeInTheDocument();
  });

  it("still shows harness onboarding for selfhosted when no request has been served", async () => {
    loginSession.surface = "admin";
    api.onboarding.get.mockResolvedValue({ first_request_served_at: null });

    renderDashboard();

    expect(await screen.findByText("onboarding")).toBeInTheDocument();
    expect(screen.queryByText("Overview")).not.toBeInTheDocument();
  });

  it("shows em dash cache hit rate when no prompt or semantic cache usage", async () => {
    renderDashboard();
    await screen.findByText("Overview");
    expect(screen.getByText("—%")).toBeInTheDocument();
    expect(screen.getByText("no cached usage yet")).toBeInTheDocument();
  });

  it("feeds the Tokens KPI the per-bucket token series, not the cost curve — non-flat at tiny spend", async () => {
    renderDashboard();
    await screen.findByRole("link", { name: "deepseek-ai/deepseek-v4-flash" });

    const { tokens } = series();
    expect(tokens.length).toBeGreaterThanOrEqual(2);
    expect(isFlat(tokens)).toBe(false);
  });

  it("feeds the Requests KPI request_count and the Actual-cost KPI actual cost", async () => {
    renderDashboard();
    await screen.findByText("Overview");
    await screen.findByRole("link", { name: "deepseek-ai/deepseek-v4-flash" });

    const { requests, costs } = series();
    expect(requests.length).toBeGreaterThan(2);
    expect(Math.max(...requests)).toBe(8);
    expect(requests.reduce((a, b) => a + b, 0)).toBe(10);
    expect(Math.max(...costs)).toBe(1);
    expect(costs.reduce((a, b) => a + b, 0)).toBeCloseTo(1.17, 5);
  });

  it("renders the spend-table model link with the /ui basePath", async () => {
    renderDashboard();
    await screen.findByText("Overview");
    const link = await screen.findByRole("link", { name: "deepseek-ai/deepseek-v4-flash" });
    expect(link).toHaveAttribute("href", "/ui/models/deepseek-ai~deepseek-v4-flash");
  });

  it("pushes the Popularity leaderboard selection via basePath-aware router.push", async () => {
    renderDashboard();
    await screen.findByRole("link", { name: "deepseek-ai/deepseek-v4-flash" });
    const row = screen.getByRole("button", { name: /deepseek-ai\/deepseek-v4-flash.*tok/ });
    fireEvent.click(row);
    expect(push).toHaveBeenCalled();
    const call = push.mock.calls[push.mock.calls.length - 1];
    const url = typeof call === "string" ? call : call[0];
    expect(url).toMatch(/^\/models\/deepseek-ai~deepseek-v4-flash$/);
  });

  it("does not fetch the 1000-row details endpoint for popularity", async () => {
    renderDashboard();
    await screen.findByRole("link", { name: "deepseek-ai/deepseek-v4-flash" });
    expect(api.metrics.details).not.toHaveBeenCalled();
    expect(api.metrics.modelBreakdown).toHaveBeenCalled();
  });
});
