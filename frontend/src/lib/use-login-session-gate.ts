"use client";

import { useEffect, useState } from "react";
import { usePathname, useRouter } from "next/navigation";

import { api } from "@/lib/api";

export type LoginSessionState = "checking" | "authed" | "anonymous";

/**
 * Probes the active login surface once at shell mount.
 *
 * Selfserve mounts /account/v1/me and does NOT mount /admin/v1/auth/me.
 * Selfhosted is the reverse. Trying admin-only here 404s in selfserve and
 * the catch-path bounced to /login while login's accountMe saw an authed
 * cookie and bounced back — a dashboard↔login loop. Probe account first;
 * fall back to admin me when the account surface is absent.
 *
 * Path changes only swap sidebar/children — 401 bounce for expired
 * sessions stays in api.request.
 */
export function useLoginSessionGate(): LoginSessionState {
  const router = useRouter();
  const pathname = usePathname();
  const [state, setState] = useState<LoginSessionState>("checking");

  useEffect(() => {
    let cancelled = false;

    async function probe(): Promise<boolean> {
      try {
        const res = await api.auth.accountMe();
        return res.authenticated;
      } catch {
        // Account surface not mounted (selfhosted / managed).
      }
      try {
        const res = await api.auth.me();
        return res.authenticated;
      } catch {
        return false;
      }
    }

    probe().then(authed => {
      if (cancelled) return;
      if (authed) {
        setState("authed");
        return;
      }
      setState("anonymous");
      const next = encodeURIComponent(pathname || "/dashboard");
      router.replace(`/login?next=${next}`);
    });

    return () => {
      cancelled = true;
    };
    // Intentionally once: pathname/router changes must not re-probe.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  return state;
}
