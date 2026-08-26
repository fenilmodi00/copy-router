"use client";

import { cn } from "@/lib/cn";

export interface MetricToggleOption {
  value: string;
  label: string;
}

// Minimal segmented control for metric switches (requests vs spend vs
// cost-per-1K on the model detail chart). No external dependency.
export function MetricToggle({
  options,
  value,
  onChange,
  className,
}: {
  options: MetricToggleOption[];
  value: string;
  onChange: (v: string) => void;
  className?: string;
}) {
  return (
    <div
      className={cn(
        "inline-flex items-center gap-0.5 rounded-lg border border-border p-0.5",
        className,
      )}
    >
      {options.map(opt => {
        const active = opt.value === value;
        return (
          <button
            key={opt.value}
            type="button"
            onClick={() => onChange(opt.value)}
            aria-pressed={active}
            className={cn(
              "rounded-md px-2 py-1 text-2xs font-medium",
              active
                ? "bg-foreground/10 text-foreground"
                : "text-muted-foreground hover:text-foreground",
            )}
          >
            {opt.label}
          </button>
        );
      })}
    </div>
  );
}