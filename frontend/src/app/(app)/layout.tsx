"use client";

import { Sidebar } from "@/components/Sidebar";
import { SidebarLayout } from "@/components/SidebarLayout";
import {
  LoginSessionProvider,
  useLoginSession,
} from "@/lib/use-login-session-gate";
import { useBackgroundWarm } from "@/lib/data-cache";

function AppShell({ children }: { children: React.ReactNode }) {
  // Warm likely-next-route caches (Models/Settings data) once, after paint.
  useBackgroundWarm();

  const { state } = useLoginSession();

  // Render the page immediately while the login probe is in flight — the
  // auth round-trip no longer serializes ahead of page data, and cached
  // data paints right away. An anonymous verdict redirects to /login (the
  // probe is one cheap call; the brief window shows skeleton pages whose
  // fetches would 401 anyway). Warm likely-next-route caches once painted.
  if (state === "anonymous") {
    return null;
  }

  return <SidebarLayout sidebar={<Sidebar />}>{children}</SidebarLayout>;
}

export default function AppLayout({ children }: { children: React.ReactNode }) {
  return (
    <LoginSessionProvider>
      <AppShell>{children}</AppShell>
    </LoginSessionProvider>
  );
}
