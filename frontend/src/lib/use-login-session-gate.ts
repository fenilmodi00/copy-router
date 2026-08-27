"use client";

import {
  createContext,
  createElement,
  useContext,
  useEffect,
  useState,
  type ReactNode,
} from "react";
import { usePathname, useRouter } from "next/navigation";

import { api } from "@/lib/api";

export type LoginSessionState = "checking" | "authed" | "anonymous";

/** Which login cookie surface authenticated this shell. */
export type AuthSurface = "account" | "admin";

export type LoginSession = {
  state: LoginSessionState;
  surface: AuthSurface | null;
};

const defaultSession: LoginSession = { state: "checking", surface: null };

const LoginSessionContext = createContext<LoginSession>(defaultSession);

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
export function useLoginSessionGate(): LoginSession {
  const router = useRouter();
  const pathname = usePathname();
  const [session, setSession] = useState<LoginSession>(defaultSession);

  useEffect(() => {
    let cancelled = false;

    async function probe(): Promise<{
      authed: boolean;
      surface: AuthSurface | null;
    }> {
      try {
        const res = await api.auth.accountMe();
        if (res.authenticated) {
          return { authed: true, surface: "account" };
        }
        // Account surface mounted but anonymous — do not fall through to
        // admin me (selfserve never mounts it).
        return { authed: false, surface: "account" };
      } catch {
        // Account surface not mounted (selfhosted / managed).
      }
      try {
        const res = await api.auth.me();
        return {
          authed: res.authenticated,
          surface: res.authenticated ? "admin" : null,
        };
      } catch {
        return { authed: false, surface: null };
      }
    }

    probe().then(({ authed, surface }) => {
      if (cancelled) return;
      if (authed) {
        setSession({ state: "authed", surface });
        return;
      }
      setSession({ state: "anonymous", surface });
      const next = encodeURIComponent(pathname || "/dashboard");
      router.replace(`/login?next=${next}`);
    });

    return () => {
      cancelled = true;
    };
    // Intentionally once: pathname/router changes must not re-probe.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  return session;
}

/** Provides the probed LoginSession to dashboard shell descendants. */
export function LoginSessionProvider({ children }: { children: ReactNode }) {
  const session = useLoginSessionGate();
  return createElement(LoginSessionContext.Provider, { value: session }, children);
}

/** Reads the shell login session (surface drives onboarding skip + logout). */
export function useLoginSession(): LoginSession {
  return useContext(LoginSessionContext);
}
