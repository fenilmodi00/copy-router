import { act, render, screen } from "@testing-library/react";
import { SWRConfig } from "swr";
import { describe, expect, it, vi } from "vitest";

// One cache provider shared across two mounts — the production wiring
// (DataCacheProvider at the root). Proves the cache-hit contract the
// timestamped keys used to defeat.
const { api } = vi.hoisted(() => {
  const summary = vi.fn().mockResolvedValue({
    request_count: 42,
    total_tokens: 1_000,
    total_requested_cost_usd: 2,
    total_actual_cost_usd: 1,
  });
  return {
    api: {
      metrics: { summary },
      onboarding: {
        get: vi.fn().mockResolvedValue({ first_request_served_at: "2026-08-20T00:00:00Z" }),
      },
    },
  };
});

vi.mock("@/lib/api", () => ({ api }));

// Pin the clock: two mounts of "now" land inside the same 5-minute quantum
// bucket, exactly like a dashboard → models → dashboard navigation.
vi.setSystemTime(new Date("2026-08-27T15:30:00.000Z"));

import { useMetricsSummary } from "@/lib/data-cache";

function Probe({ rangeId }: { rangeId: string }) {
  const { data, isLoading } = useMetricsSummary(
    rangeId,
    "2026-08-26T15:30:00.000Z",
    "2026-08-27T15:30:00.000Z",
  );
  return (
    <div>
      <div data-testid="loading">{isLoading ? "yes" : "no"}</div>
      <div data-testid="requests">{data?.request_count ?? "—"}</div>
    </div>
  );
}

describe("stale-while-revalidate across remounts", () => {
  it("serves the second mount from cache without a refetch, then revalidates", async () => {
    const shared = new Map();

    // First mount: fetches.
    const first = render(
      <SWRConfig value={{ provider: () => shared, dedupingInterval: 0 }}>
        <Probe rangeId="last-24h" />
      </SWRConfig>,
    );
    expect(await screen.findByText("42")).toBeInTheDocument();
    expect(api.metrics.summary).toHaveBeenCalledTimes(1);
    first.unmount();

    // Fresh response for the background revalidation.
    api.metrics.summary.mockResolvedValue({
      request_count: 50,
      total_tokens: 1_200,
      total_requested_cost_usd: 2,
      total_actual_cost_usd: 1,
    });

    // Second mount within the same quantum bucket: cached 42 paints
    // synchronously — no skeleton flash — and exactly one revalidation runs.
    render(
      <SWRConfig value={{ provider: () => shared, dedupingInterval: 0 }}>
        <Probe rangeId="last-24h" />
      </SWRConfig>,
    );

    // Cached data is available on first paint (isLoading false, 42 shown).
    expect(screen.getByTestId("loading").textContent).toBe("no");
    expect(screen.getByTestId("requests").textContent).toBe("42");

    // Background revalidation updates the value in place.
    await act(async () => {
      await vi.waitFor(() => {
        expect(api.metrics.summary).toHaveBeenCalledTimes(2);
      });
    });
    await vi.waitFor(() => {
      expect(screen.getByTestId("requests").textContent).toBe("50");
    });
  });
});
