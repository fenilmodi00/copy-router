import { describe, expect, it } from "vitest";
import { formatContext, formatNumber, formatUSD, toNumber } from "./format";

describe("formatUSD", () => {
  it("renders zero as $0.00", () => {
    expect(formatUSD(0)).toBe("$0.00");
  });
  it("renders NaN as an em dash", () => {
    expect(formatUSD(NaN)).toBe("—");
  });
  it("keeps 4 decimals below $0.001", () => {
    expect(formatUSD(0.0004)).toBe("$0.0004");
  });
  it("rounds to cents above $0.001", () => {
    expect(formatUSD(0.1234)).toBe("$0.12");
    expect(formatUSD(12.345)).toBe("$12.35");
  });
});

describe("formatNumber", () => {
  it("handles the 1K boundary", () => {
    expect(formatNumber(999)).toBe("999");
    expect(formatNumber(1500)).toBe("1.5K");
  });
  it("handles the 1M boundary", () => {
    expect(formatNumber(1_000_000)).toBe("1.0M");
    expect(formatNumber(2_500_000)).toBe("2.5M");
  });
  it("handles the 1B boundary", () => {
    expect(formatNumber(1_000_000_000)).toBe("1000.0M");
  });
});

describe("formatContext", () => {
  it("keeps small windows readable", () => {
    expect(formatContext(131_072)).toBe("131.1K");
  });
  it("compresses large windows", () => {
    expect(formatContext(1_048_576)).toBe("1.0M");
  });
  it("renders zero as an em dash", () => {
    expect(formatContext(0)).toBe("—");
  });
});

describe("formatContext large windows", () => {
  it("compresses 1M+", () => {
    expect(formatContext(2_000_000)).toBe("2.0M");
  });
});

describe("toNumber tiny strings", () => {
  it("drops irrelevant trailing precision without losing the magnitude", () => {
    expect(toNumber("0.0001")).toBe(0.0001);
  });
});

describe("toNumber", () => {
  it("parses string prices", () => {
    expect(toNumber("0.15")).toBe(0.15);
  });
  it("maps garbage to zero", () => {
    expect(toNumber("n/a")).toBe(0);
  });
});