"use client";

import React, { useState } from "react";

import { FilterPill } from "./FilterPill";
import { cn } from "@/lib/cn";
import { Check } from "lucide-react";

export interface ModelDescriptor {
  id: string;
  label: string;
}

// Filter-pill multi-select over catalog models. Mirrors the Date/Granularity
// pills' shape (FilterPill + in-place list) but toggles multiple values.
export function ModelSelectorPill({
  models,
  selected,
  onToggle,
  className,
}: {
  models: ModelDescriptor[];
  selected: string[];
  onToggle: (id: string) => void;
  className?: string;
}) {
  const [open, setOpen] = useState(false);
  return (
    // `relative` wrapper makes the absolute dropdown position against its own
    // pill instead of escaping to the nearest positioned page ancestor.
    <div className={cn("relative", className)}>
      <FilterPill>
        <span className="font-medium">Model</span>
        <span className="text-muted-foreground">is</span>
        <FilterPill.Button className="-mr-2 pr-2" onClick={() => setOpen(o => !o)}>
          {selected.length === 0 ? "all" : `${selected.length} selected`}
        </FilterPill.Button>
      </FilterPill>
      {open && (
        <div className="absolute left-0 top-full z-20 mt-1 w-64 rounded-lg border border-border bg-card p-1 shadow-lg">
          {models.map(m => {
            const active = selected.includes(m.id);
            return (
              <button
                key={m.id}
                type="button"
                onClick={() => onToggle(m.id)}
                className="flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-left text-xs hover:bg-foreground/5"
              >
                <span className="flex size-4 items-center justify-center">
                  {active && <Check className="size-3.5" />}
                </span>
                <span className="truncate">{m.label}</span>
              </button>
            );
          })}
        </div>
      )}
    </div>
  );
}