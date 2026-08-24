"use client";

import { CopyBlock } from "@/components/CopyBlock";
import { Text } from "@/components/atoms/Text";
import { Button } from "@/components/molecules/Button";
import { Appearance } from "@/components/types";
import {
  HARNESSES,
  type HarnessID,
  harness,
  installCommand,
  routerOrigin,
} from "@/lib/installCommands";
import { useState } from "react";

interface InstallCommandPickerProps {
  /** Raw bearer token to inline into the command. */
  token: string;
}

/**
 * Harness tabs on top, then the one command to run. The harness choice comes
 * first because it's the only decision the user actually has to make — a
 * freshly minted token is useless until it's paired with the right installer
 * invocation, and burying that behind an endpoint block and an uninstall
 * section is what made the old reveal unreadable. Personal (user) scope only.
 */
export function InstallCommandPicker({ token }: InstallCommandPickerProps) {
  const [harnessID, setHarnessID] = useState<HarnessID>("claude");
  const origin = routerOrigin();
  const selected = harness(harnessID);

  return (
    <div className="flex flex-col gap-3">
      <div role="tablist" aria-label="Harness" className="flex flex-wrap gap-2">
        {HARNESSES.map(h => (
          <Button
            key={h.id}
            type="button"
            role="tab"
            aria-selected={harnessID === h.id}
            size="sm"
            appearance={harnessID === h.id ? Appearance.Filled : Appearance.Outlined}
            className={
              harnessID === h.id
                ? "!border-brand !bg-brand !text-white hover:!bg-brand/90"
                : undefined
            }
            onClick={() => setHarnessID(h.id)}
          >
            {h.label}
          </Button>
        ))}
      </div>

      <Text className="text-2xs text-muted-foreground">{selected.detail}</Text>

      {origin === "" ? (
        <Text className="text-2xs text-muted-foreground">Preparing install command…</Text>
      ) : (
        <CopyBlock
          value={installCommand(harnessID, "user", token, origin)}
          title="Copy install command"
        />
      )}
    </div>
  );
}
