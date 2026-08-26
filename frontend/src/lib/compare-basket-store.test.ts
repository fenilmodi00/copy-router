import { beforeEach, describe, expect, it, vi } from "vitest";
import { CAP, STORAGE_KEY, dedupeAndCap, useCompareBasket } from "./compare-basket-store";

function setStored(ids: string[]) {
  window.localStorage.setItem(STORAGE_KEY, JSON.stringify(ids));
}

describe("compare-basket-store", () => {
  beforeEach(() => {
    window.localStorage.clear();
    // Fresh store state per test; the module-level Zustand store persists
    // across tests in the same worker otherwise.
    vi.resetModules();
  });

  it("rejects the 5th add at the cap", async () => {
    const { useCompareBasket } = await import("./compare-basket-store");
    const store = useCompareBasket.getState();
    for (const id of ["a", "b", "c", "d"]) store.add(id);
    const ids = useCompareBasket.getState().ids;
    expect(ids).toEqual(["a", "b", "c", "d"]);
    useCompareBasket.getState().add("e");
    expect(useCompareBasket.getState().ids).toEqual(["a", "b", "c", "d"]);
  });

  it("hydrates from localStorage on mount", async () => {
    setStored(["x", "y"]);
    const { useCompareBasket } = await import("./compare-basket-store");
    // The store reads localStorage at module init (see implementation).
    expect(useCompareBasket.getState().ids).toEqual(["x", "y"]);
  });

  it("caps a >cap localStorage payload to CAP on hydrate", async () => {
    setStored(["a", "b", "c", "d", "e"]);
    const { useCompareBasket, CAP } = await import("./compare-basket-store");
    expect(useCompareBasket.getState().ids.length).toBe(CAP);
  });
});