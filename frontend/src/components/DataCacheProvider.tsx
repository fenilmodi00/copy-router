"use client";

import { SWRConfig } from "swr";
import { persistedCacheProvider } from "@/lib/swr-persist";

/**
 * App-wide SWR defaults: stale-while-revalidate, in-flight dedupe, and
 * keepPreviousData so remounted dashboard pages reuse prior payloads.
 * The cache persists to localStorage (TTL-capped) so a refresh shows
 * stale data instantly, then revalidates in the background.
 */
export function DataCacheProvider({ children }: { children: React.ReactNode }) {
  return (
    <SWRConfig
      value={{
        provider: persistedCacheProvider,
        revalidateOnFocus: true,
        keepPreviousData: true,
        dedupingInterval: 2_000,
      }}
    >
      {children}
    </SWRConfig>
  );
}
