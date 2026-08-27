"use client";

import { ErrorBanner } from "./shared";
import { Card } from "@/components/molecules/Card";
import { useConfig } from "@/lib/data-cache";
import type { RouterConfig } from "@/lib/api";

interface ConfigRow {
  label: string;
  value: string;
  description: string;
}

function buildRows(cfg: RouterConfig): ConfigRow[] {
  return [
    {
      label: "Cluster version",
      value: cfg.cluster_version || "—",
      description: "Active routing artifact bundle served by default",
    },
    {
      label: "Embed only user message",
      value: cfg.embed_only_user_message ? "Enabled" : "Disabled",
      description:
        "Whether the router embeds user-role text only (no system, assistant, or tool_result content) for cluster routing",
    },
    {
      label: "Sticky decision TTL",
      value: cfg.sticky_decision_ttl_ms || "—",
      description: "How long a sticky routing decision is cached per conversation",
    },
    {
      label: "Semantic cache",
      value: cfg.semantic_cache_enabled ? "Enabled" : "Disabled",
      description: "Whether semantic response caching is active",
    },
    {
      label: "OpenTelemetry",
      value: cfg.otel_enabled ? "Enabled" : "Disabled",
      description: "Whether OTEL tracing and metrics are exported",
    },
    {
      label: "Dev mode",
      value: cfg.dev_mode ? "On" : "Off",
      description: "Relaxed auth and verbose logging — never enable in production",
    },
  ];
}

export function ConfigPanel() {
  const { data: config, error: rawError, isLoading } = useConfig();
  const error =
    rawError instanceof Error
      ? rawError.message
      : rawError
        ? "Failed to load config."
        : null;

  return (
    <>
      {error && <ErrorBanner>{error}</ErrorBanner>}

      <Card className="p-0">
        <Card.Header className="border-b border-border px-5 py-3">
          <Card.Title variant="h4">Runtime values</Card.Title>
        </Card.Header>
        <Card.Content>
          {config == null ? (
            isLoading || error == null ? (
              <div className="space-y-3 px-5 py-6">
                <Card.Loading className="border-0 shadow-none" />
              </div>
            ) : (
              <div className="px-5 py-8 text-center text-2xs text-muted-foreground">
                Failed to load
              </div>
            )
          ) : (
            <ul className="divide-y divide-border">
              {buildRows(config).map(row => (
                <li
                  key={row.label}
                  className="flex items-start justify-between gap-6 px-5 py-4"
                >
                  <div className="flex-1">
                    <p className="text-xs font-medium text-foreground">{row.label}</p>
                    <p className="mt-0.5 text-2xs text-muted-foreground">{row.description}</p>
                  </div>
                  <span className="shrink-0 font-mono text-xs text-foreground">{row.value}</span>
                </li>
              ))}
            </ul>
          )}
        </Card.Content>
      </Card>
    </>
  );
}
