import { render, screen } from "@testing-library/react";
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

vi.mock("next/navigation", () => ({
  useRouter: () => ({ push: vi.fn(), replace: vi.fn(), back: vi.fn(), prefetch: vi.fn() }),
}));

const { aiandModels, metrics, useDashboardFilters, ModelSelectorPill } = vi.hoisted(() => {
  const now = new Date().toISOString();
  const useDashboardFilters = () => ({
    filters: {
      fromISO: now,
      toISO: now,
      granularity: "hour",
      range: { id: "last-24h", label: "Last 24 hours" },
    },
    setRangeId: vi.fn(),
    setGranularity: vi.fn(),
  });
  const ModelSelectorPill = (props: {
    models: { id: string; label: string }[];
    selected: string[];
    onToggle: (id: string) => void;
  }) => (
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
  };
  const metrics = {
    modelBreakdown: vi.fn().mockResolvedValue({ buckets: [] }),
  };
  return { aiandModels, metrics, useDashboardFilters, ModelSelectorPill };
});

vi.mock("@/lib/api", () => ({ api: { aiandModels, metrics } }));

vi.mock("@/components/DashboardPageFilters", () => ({ useDashboardFilters }));
vi.mock("@/components/DashboardPageFilters/ModelSelectorPill", () => ({ ModelSelectorPill }));

import ModelsPage from "./page";

function renderPage() {
  return render(
    <SWRConfig value={{ provider: () => new Map(), dedupingInterval: 0 }}>
      <ModelsPage />
    </SWRConfig>,
  );
}

describe("ModelsPage (models route)", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    lastHref.value = "";
  });

  it("renders each catalog model row landing on a basePath-correct /ui/models/<id> link", async () => {
    renderPage();
    const link = await screen.findByRole("link", { name: /deepseek-ai\/deepseek-v4-flash/ });
    expect(link).toHaveAttribute("href", "/ui/models/deepseek-ai~deepseek-v4-flash");
  });

  it("passes a basePath-correct href to next/link for the row anchor", async () => {
    renderPage();
    await screen.findByRole("link", { name: /deepseek-ai\/deepseek-v4-flash/ });
    expect(lastHref.value).toBe("/ui/models/deepseek-ai~deepseek-v4-flash");
  });
});
