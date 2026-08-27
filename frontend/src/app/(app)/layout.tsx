"use client";

import { SettingsSidebar } from "@/components/SettingsSidebar";
import { Sidebar } from "@/components/Sidebar";
import { SidebarLayout } from "@/components/SidebarLayout";
import { Skeleton } from "@/components/atoms/Skeleton";
import { useLoginSessionGate } from "@/lib/use-login-session-gate";
import { usePathname } from "next/navigation";

export default function AppLayout({ children }: { children: React.ReactNode }) {
  const pathname = usePathname();
  const state = useLoginSessionGate();

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

  const sidebar = pathname.startsWith("/settings") ? <SettingsSidebar /> : <Sidebar />;

  return <SidebarLayout sidebar={sidebar}>{children}</SidebarLayout>;
}
