"use client";

import { Sidebar } from "@/components/Sidebar";
import { SidebarLayout } from "@/components/SidebarLayout";
import { Skeleton } from "@/components/atoms/Skeleton";
import {
  LoginSessionProvider,
  useLoginSession,
} from "@/lib/use-login-session-gate";

function AppShell({ children }: { children: React.ReactNode }) {
  const { state } = useLoginSession();

  if (state !== "authed") {
    return (
      <SidebarLayout sidebar={<div className="md:w-[244px]" />}>
        <div className="flex flex-col gap-4 p-6">
          <Skeleton className="h-8 w-48" />
          <Skeleton className="h-32 w-full" />
          <Skeleton className="h-32 w-full" />
        </div>
      </SidebarLayout>
    );
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
