"use client";

import { ConfigPanel } from "@/components/settings/ConfigPanel";
import { RouterKeysPanel } from "@/components/settings/RouterKeysPanel";
import { SettingsPage, SettingsSection } from "@/components/settings/SettingsPage";
import { UsageSnippetsPanel } from "@/components/settings/UsageSnippetsPanel";
import { Code2, KeyRound, Settings as SettingsIcon } from "lucide-react";

export default function GeneralSettingsPage() {
  return (
    <SettingsPage href="/settings">
      <SettingsSection
        icon={<Code2 className="size-4" />}
        title="How to use"
        description="OpenAI-compatible API. Set model to auto and the router chooses."
      >
        <UsageSnippetsPanel />
      </SettingsSection>

      <SettingsSection
        icon={<KeyRound className="size-4" />}
        title="Router API keys"
        description="Keys used to authenticate requests to this router."
      >
        <RouterKeysPanel />
      </SettingsSection>

      <SettingsSection
        icon={<SettingsIcon className="size-4" />}
        title="Configuration"
        description="Runtime values set via environment variables."
      >
        <ConfigPanel />
      </SettingsSection>
    </SettingsPage>
  );
}
