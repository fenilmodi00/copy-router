"use client";

import { create } from "zustand";

export const CAP = 4;
export const STORAGE_KEY = "weave-router.compare-basket";

// Cap-silently: keep order, drop anything past the cap. Non-destructive for
// callers that pass a pre-hydrated URL payload (compare page clamps to 4).
export function dedupeAndCap(ids: string[], cap: number): string[] {
  const seen = new Set<string>();
  const out: string[] = [];
  for (const id of ids) {
    if (out.length >= cap) break;
    if (seen.has(id)) continue;
    seen.add(id);
    out.push(id);
  }
  return out;
}

function readInitial(): string[] {
  if (typeof window === "undefined") return [];
  try {
    const raw = window.localStorage.getItem(STORAGE_KEY);
    if (raw == null) return [];
    return dedupeAndCap(JSON.parse(raw) as string[], CAP);
  } catch {
    // Corrupt storage is treated as empty rather than crashing at module init.
    return [];
  }
}

// Module-level store; hydration from localStorage happens at import time. The
// `hydrated` flag lets the compare page gate a nonce render on a store that
// has read its persisted payload.
export const useCompareBasket = create<{
  ids: string[];
  hydrated: boolean;
  add: (id: string) => void;
  remove: (id: string) => void;
  clear: () => void;
  setHydrated: (v: boolean) => void;
}>()(set => ({
  ids: readInitial(),
  hydrated: false,
  add: id =>
    set(state => {
      if (state.ids.includes(id) || state.ids.length >= CAP) return state;
      const next = [...state.ids, id];
      try {
        window.localStorage.setItem(STORAGE_KEY, JSON.stringify(next));
      } catch {
        // Storage unavailable (private mode): keep the in-memory state.
      }
      return { ids: next };
    }),
  remove: id =>
    set(state => {
      const next = state.ids.filter(x => x !== id);
      try {
        window.localStorage.setItem(STORAGE_KEY, JSON.stringify(next));
      } catch {
        // Non-fatal.
      }
      return { ids: next };
    }),
  clear: () =>
    set(() => {
      try {
        window.localStorage.removeItem(STORAGE_KEY);
      } catch {
        // Non-fatal.
      }
      return { ids: [] };
    }),
  setHydrated: v => set(() => ({ hydrated: v })),
}));