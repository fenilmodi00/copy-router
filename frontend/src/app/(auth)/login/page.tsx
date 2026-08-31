"use client";

import { Suspense, useEffect, useState } from "react";
import { useRouter, useSearchParams } from "next/navigation";

import { Input } from "@/components/Input";
import { Button } from "@/components/molecules/Button";
import { Skeleton } from "@/components/atoms/Skeleton";
import { Appearance, Intent } from "@/components/types";
import { api } from "@/lib/api";

export default function LoginPage() {
  return (
    <Suspense
      fallback={
        <div className="w-full max-w-sm rounded-lg border border-border-darker bg-background p-6 shadow-sm">
          <Skeleton className="mb-4 h-8 w-32" />
          <Skeleton className="h-10 w-full" />
        </div>
      }
    >
      <LoginInner />
    </Suspense>
  );
}

/**
 * Reject anything that isn't a same-origin internal path (e.g. `javascript:`,
 * `//attacker.com`, `https://...`). After basePath stripping in api.ts the
 * value should look like `/dashboard` or `/settings?tab=keys`.
 */
function safeNext(raw: string | null): string {
  if (raw == null) return "/dashboard";
  if (!raw.startsWith("/")) return "/dashboard";
  if (raw.startsWith("//") || raw.startsWith("/\\")) return "/dashboard";
  return raw;
}

function LoginInner() {
  const router = useRouter();
  const params = useSearchParams();
  const next = safeNext(params.get("next"));

  useEffect(() => {
    let cancelled = false;
    api
      .auth
      .accountMe()
      .then((res) => {
        if (cancelled) return;
        if (res.authenticated) {
          router.replace(next);
        }
      })
      .catch(() => {
        // Probe failures (session expired) fall through to the form below.
      });
    return () => {
      cancelled = true;
    };
  }, [router, next]);

  return <AiandKeyForm next={next} />;
}

function AiandKeyForm({ next }: { next: string }) {
  const router = useRouter();
  const [apiKey, setApiKey] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    if (!apiKey) return;
    setSubmitting(true);
    setError(null);
    try {
      await api.auth.loginWithKey(apiKey);
      router.replace(next);
    } catch (err: unknown) {
      const message = err instanceof Error ? err.message : "Sign-in failed.";
      if (message.includes("401") || message.includes("403")) {
        setError("That key wasn't accepted. Check the value and try again.");
      } else if (message.includes("404")) {
        setError("Self-service sign-in isn't available on this deployment.");
      } else {
        setError(message);
      }
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <div className="w-full max-w-sm rounded-lg border border-border-darker bg-background p-6 shadow-sm">
      <div className="mb-6 flex items-center gap-3">
        {/* eslint-disable-next-line @next/next/no-img-element */}
        <img src="/ui/aiand.svg" alt="aiand-auto" width={29} height={32} className="h-8 w-auto" />
        <div>
          <h1 className="font-display text-base font-semibold text-foreground">aiand-auto</h1>
          <p className="text-2xs text-muted-foreground">Sign in with your aiand key</p>
        </div>
      </div>

      <form onSubmit={handleSubmit} className="space-y-3" autoComplete="off">
        <Input
          label="aiand API key"
          type="password"
          name="aiand-api-key"
          autoComplete="off"
          data-1p-ignore
          data-lpignore="true"
          data-form-type="other"
          autoFocus
          value={apiKey}
          onChange={(e) => setApiKey(e.target.value)}
          placeholder="sk-..."
          required
        />
        {error && (
          <p className="rounded-md border border-danger/30 bg-danger/5 px-3 py-2 text-2xs text-danger">
            {error}
          </p>
        )}
        <Button
          type="submit"
          appearance={Appearance.Filled}
          intent={Intent.Primary}
          className="w-full"
          disabled={submitting || apiKey === ""}
        >
          {submitting ? "Signing in…" : "Sign in"}
        </Button>
      </form>

      <p className="mt-4 text-2xs text-muted-foreground">
        Use the aiand API key from your account. Your session is stored in a cookie set by the router.
      </p>
    </div>
  );
}
