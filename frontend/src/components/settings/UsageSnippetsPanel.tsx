"use client";

import { CopyBlock } from "@/components/CopyBlock";
import { Text } from "@/components/atoms/Text";
import { Button } from "@/components/molecules/Button";
import { Appearance } from "@/components/types";
import { routerOrigin } from "@/lib/installCommands";
import {
  KEY_PLACEHOLDER,
  SNIPPET_LANGUAGES,
  type SnippetLanguage,
  type SnippetMode,
  snippetLanguage,
  usageSnippet,
} from "@/lib/usageSnippets";
import { useState } from "react";

const MODES: { id: SnippetMode; label: string }[] = [
  { id: "auto", label: "Auto routing" },
  { id: "force", label: "Pin a model" },
];

export function UsageSnippetsPanel() {
  const [mode, setMode] = useState<SnippetMode>("auto");
  const [lang, setLang] = useState<SnippetLanguage>("curl");
  const origin = routerOrigin();
  const selected = snippetLanguage(lang);

  return (
    <div className="flex flex-col gap-3">
      <div role="tablist" aria-label="Routing mode" className="flex flex-wrap gap-2">
        {MODES.map(m => (
          <Button
            key={m.id}
            type="button"
            role="tab"
            aria-selected={mode === m.id}
            size="sm"
            appearance={mode === m.id ? Appearance.Filled : Appearance.Outlined}
            className={
              mode === m.id ? "!border-brand !bg-brand !text-white hover:!bg-brand/90" : undefined
            }
            onClick={() => setMode(m.id)}
          >
            {m.label}
          </Button>
        ))}
      </div>

      <div role="tablist" aria-label="Snippet language" className="flex flex-wrap gap-2">
        {SNIPPET_LANGUAGES.map(s => (
          <Button
            key={s.id}
            type="button"
            role="tab"
            aria-selected={lang === s.id}
            size="sm"
            appearance={lang === s.id ? Appearance.Filled : Appearance.Outlined}
            className={
              lang === s.id ? "!border-brand !bg-brand !text-white hover:!bg-brand/90" : undefined
            }
            onClick={() => setLang(s.id)}
          >
            {s.label}
          </Button>
        ))}
      </div>

      <Text className="text-2xs text-muted-foreground">{selected.detail}</Text>

      {origin === "" ? (
        <Text className="text-2xs text-muted-foreground">Preparing…</Text>
      ) : (
        <CopyBlock
          value={usageSnippet(lang, mode, { origin, apiKey: KEY_PLACEHOLDER })}
          title="Copy usage snippet"
        />
      )}

      <Text className="text-2xs text-muted-foreground">
        {mode === "auto"
          ? "The router picks the model for each request."
          : "The catalog id in the model field forces that exact model."}
      </Text>
    </div>
  );
}
