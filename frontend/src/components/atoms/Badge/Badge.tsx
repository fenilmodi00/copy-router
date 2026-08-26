import { cn } from "@/lib/cn";
import { capabilityColor, TIER_COLORS } from "@/lib/capability-colors";
import React from "react";

export type BadgeVariant = "default" | "capability" | "tier";

export interface BadgeProps extends React.HTMLAttributes<HTMLSpanElement> {
  variant?: BadgeVariant;
  tone?: string;
}

const defaultStyles =
  "inline-flex items-center gap-1 rounded-md border px-1.5 py-0.5 text-2xs font-medium";

export function Badge({
  variant = "default",
  tone = "text-muted-foreground",
  className,
  children,
  ...props
}: BadgeProps) {
  const accent =
    variant === "capability"
      ? "border-primary/20 bg-primary/5"
      : variant === "tier"
        ? "border-warning/20 bg-warning/5"
        : "border-border bg-muted";
  return (
    <span className={cn(defaultStyles, accent, tone, className)} {...props}>
      {children}
    </span>
  );
}

export function CapabilityBadge({ name }: { name: string }) {
  return (
    <Badge variant="capability" tone={capabilityColor(name)}>
      {name}
    </Badge>
  );
}

export function TierBadge({ tier }: { tier: string }) {
  return (
    <Badge variant="tier" tone={TIER_COLORS[tier] ?? "text-muted-foreground"}>
      {tier}
    </Badge>
  );
}

Badge.Capability = CapabilityBadge;
Badge.Tier = TierBadge;