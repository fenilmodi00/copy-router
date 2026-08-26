import { describe, expect, it } from "vitest";
import {
  CACHE_HIT_RATE,
  SAMPLE_COMPLETION_OUT,
  SAMPLE_PROMPT_IN,
  cachedVerdict,
  plainVerdict,
} from "./compare-verdict";

// The two shapes the spec prices: 15K prompt + 35K completion, and that same
// shape assuming 70% of prompt tokens hit the cache. Both are pure functions —
// this module hosts the formula so the compare page and tests share it.
describe("compare verdict math", () => {
  const m = {
    input_per_1m: "0.15",
    output_per_1m: "0.25",
    cached_input_per_1m: "0.08",
  };

  it("prices a 15K prompt + 35K completion sample", () => {
    expect(SAMPLE_PROMPT_IN).toBe(15_000);
    expect(SAMPLE_COMPLETION_OUT).toBe(35_000);
    expect(CACHE_HIT_RATE).toBe(0.7);

    // (15_000 × 0.15 + 35_000 × 0.25) / 1e6
    const expected = (15_000 * 0.15 + 35_000 * 0.25) / 1e6;
    const actual = plainVerdict(m.input_per_1m, m.output_per_1m);
    expect(actual).toBeCloseTo(expected, 6);
  });

  it("reduces prompt cost at 70% cache hit", () => {
    // prompt tokens charged at cached_input_per_1m * hit + input * (1 - hit)
    const chargedPrompt = 15_000 * CACHE_HIT_RATE * 0.08 + 15_000 * (1 - CACHE_HIT_RATE) * 0.15;
    const completion = 35_000 * 0.25;
    const expected = (chargedPrompt + completion) / 1e6;
    const actual = cachedVerdict(m.input_per_1m, m.output_per_1m, m.cached_input_per_1m);
    expect(actual).toBeCloseTo(expected, 6);
  });

  it("cached version is strictly cheaper than non-cached", () => {
    const plain = plainVerdict(m.input_per_1m, m.output_per_1m);
    const cached = cachedVerdict(m.input_per_1m, m.output_per_1m, m.cached_input_per_1m);
    expect(cached).toBeLessThan(plain);
  });
});