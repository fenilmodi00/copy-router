"use client";

import { SWRConfig } from "swr";

/**
 * App-wide SWR defaults: stale-while-revalidate, in-flight dedupe, and
 * keepPreviousData so remounted dashboard pages reuse prior payloads.
 * Uses SWR's default global Map provider so cache survives remounts.
 */
export function DataCacheProvider({ children }: { children: React.ReactNode }) {
  return (
    <SWRConfig
      value={{
        revalidateOnFocus: true,
        keepPreviousData: true,
        dedupingInterval: 2_000,
      }}
    >
      {children}
    </SWRConfig>
  );
}
