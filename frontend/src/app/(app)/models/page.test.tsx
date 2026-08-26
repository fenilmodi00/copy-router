import { render, screen, within } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

// Catalog-explorer route test: every model row must link to the detail page
// with the /ui basePath, because the dashboard is served under /ui. This is
// the regression test for the dropped-prefix defect: the row anchor was a
// bare <a href="/models/..."> which Next's basePath handling does NOT prefix,
// 404ing on the static export. The page under test is a client component
// (useState/useEffect), so we mock its hooks + the API.

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

vi.mock("next/navigation", () => ({
  useRouter: () => ({ push: vi.fn(), replace: vi.fn(), back: vi.fn(), prefetch: vi.fn() }),
}));

const { aiandModels, metrics, modelBreakdown, useDashboardFilters, ModelSelectorPill } = vi.hoisted(() => {
  const now = new Date().toISOString();
  const useDashboardFilters = () => ({
    filters: { fromISO: now, toISO: now, granularity: "day", range: { id: "last-month", label: "Last month" } },
    setRangeId: vi.fn(),
    setGranularity: vi.fn(),
  });
  const ModelSelectorPill = (props: { models: { id: string; label: string }[]; selected: string[]; onToggle: (id: string) => void }) => (
    <div>
      {props.models.map(m => (
        <button key={m.id} type="button" onClick={() => props.onToggle(m.id)}>
          {m.label}
        </button>
      ))}
    </div>
  );
  const aiandModels = {
    list: vi.fn().mockResolvedValue({
      data: [{ id: "deepseek-ai/deepseek-v4-flash", provider: "deepseek-ai", context_window: 1_000_000, capabilities: ["reasoning"], reasoning_efforts: ["none"], reasoning_effort_default: "none", input_per_1m: "0.15", output_per_1m: "0.25", cached_input_per_1m: "0.08", currency: "usd" }],
    }),
  };
  const modelBreakdown = vi.fn().mockResolvedValue({ buckets: [] });
  const metrics = { modelBreakdown };
  return { aiandModels, metrics, modelBreakdown, useDashboardFilters, ModelSelectorPill };
});

vi.mock("@/lib/api", () => ({ api: { aiandModels, metrics } }));

vi.mock("@/components/DashboardPageFilters", () => ({ useDashboardFilters }));
vi.mock("@/components/DashboardPageFilters/ModelSelectorPill", () => ({ ModelSelectorPill }));

import ModelsPage from "./page";

describe("ModelsPage (models route)", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    lastHref.value = "";
  });

  it("renders each catalog model row landing on a basePath-correct /ui/models/<id> link", async () => {
    render(<ModelsPage />);
    const link = await screen.findByRole("link", { name: /deepseek-ai\/deepseek-v4-flash/ });
    expect(link).toHaveAttribute("href", "/ui/models/deepseek-ai~deepseek-v4-flash");
  });

  it("passes a basePath-correct href to next/link for the row anchor", async () => {
    render(<ModelsPage />);
    await screen.findByRole("link", { name: /deepseek-ai\/deepseek-v4-flash/ });
    expect(lastHref.value).toBe("/ui/models/deepseek-ai~deepseek-v4-flash");
  });
});