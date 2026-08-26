import { render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { TooltipProvider } from "@/components/molecules/Tooltip";
import ModelDetailView from "./view";

// Route-level test against the real implementation: `page.tsx` is a thin
// static-export wrapper; `view.tsx` resolves the id from `props.params`
// (client nav) falling back to the pathname when `props.params` is absent and
// `useParams` still carries the "__none__" placeholder generateStaticParams
// baked into the export (deep link). Catalog ids contain "/", so the static
// export links with "~" and the page normalizes both forms.

vi.mock("next/navigation", () => ({
  useParams: () => ({ id: "__none__" }),
  usePathname: () => "/models/deepseek-ai~deepseek-v4-flash",
  useRouter: () => ({ push: vi.fn(), replace: vi.fn(), back: vi.fn(), prefetch: vi.fn() }),
}));

vi.mock("@/lib/compare-basket-store", () => ({
  useCompareBasket: () => ({
    ids: [],
    hydrated: true,
    add: vi.fn(),
    remove: vi.fn(),
    clear: vi.fn(),
    setHydrated: vi.fn(),
  }),
}));

const { aiandModels, model } = vi.hoisted(() => {
  const list = vi.fn();
  const model = {
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
  };
  list.mockResolvedValue({ data: [model] });
  return { aiandModels: { list }, model };
});

vi.mock("@/lib/api", () => ({
  api: {
    aiandModels,
    metrics: {
      modelBreakdown: vi.fn().mockResolvedValue({ buckets: [] }),
      timeseries: vi.fn().mockResolvedValue({ buckets: [] }),
    },
  },
}));

describe("ModelDetailView (models/[id] route)", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    aiandModels.list.mockResolvedValue({ data: [model] });
  });

  it("resolves the id from props.params and renders the model name", async () => {
    render(
      <TooltipProvider>
        <ModelDetailView params={{ id: "deepseek-ai~deepseek-v4-flash" }} />
      </TooltipProvider>,
    );
    // The header is `{provider} / {id}` (`deepseek-ai / deepseek-v4-flash`)
    // split across nested Text nodes ("/"), so match the heading's normalized
    // text with a regex rather than an exact single-node text matcher.
    expect(await screen.findByRole("heading", { name: /deepseek-ai.*deepseek-v4-flash/ })).toBeInTheDocument();
  });

  it("falls back to the pathname when params carry the __none__ placeholder", async () => {
    render(
      <TooltipProvider>
        <ModelDetailView />
      </TooltipProvider>,
    );
    expect(await screen.findByRole("heading", { name: /deepseek-ai.*deepseek-v4-flash/ })).toBeInTheDocument();
  });

  it("renders an empty state when the catalog has no matching id", async () => {
    aiandModels.list.mockResolvedValue({ data: [] });
    render(<ModelDetailView />);
    expect(
      await screen.findByText(
        'Model "deepseek-ai/deepseek-v4-flash" is not in the live ai& catalog.',
      ),
    ).toBeInTheDocument();
  });
});