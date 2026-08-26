import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { CacheHitGauge } from "./CacheHitGauge";

// The `—%` contract (spec user story 19): a fresh install with zero cached
// reads renders a dash, never a fake 0% — and never a percentage when the
// denominator is undefined or zero.
describe("CacheHitGauge", () => {
  it("renders —% when total input tokens is zero", () => {
    render(<CacheHitGauge cacheReadTokens={0} totalInputTokens={0} />);
    expect(screen.getByText("—%")).toBeInTheDocument();
  });

  it("renders —% when denominator is undefined", () => {
    render(<CacheHitGauge cacheReadTokens={5} totalInputTokens={NaN} />);
    expect(screen.getByText("—%")).toBeInTheDocument();
  });

  it("renders a percent when there is valid usage", () => {
    render(<CacheHitGauge cacheReadTokens={50} totalInputTokens={100} />);
    expect(screen.getByText("50%")).toBeInTheDocument();
  });
});