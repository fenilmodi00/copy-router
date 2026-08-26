"use client";

import { ProviderKeysPanel } from "@/components/settings/ProviderKeysPanel";
import { SettingsPage, SettingsSection } from "@/components/settings/SettingsPage";
import { Plug } from "lucide-react";

export default function ProviderKeysSettingsPage() {
  return (
    <SettingsPage href="/settings/providers">
      <SettingsSection
        icon={<Plug className="size-4" />}
        title="Provider API keys"
        description="Bring your own aiand API key (BYOK), or point an OpenAI-compatible gateway at this router."
      >
        <ProviderKeysPanel />
      </SettingsSection>
    </SettingsPage>
  );
}
