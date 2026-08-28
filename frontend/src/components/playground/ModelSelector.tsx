"use client";

import type { AiandModel } from "@/lib/api";
import type { PlaygroundModel } from "@/lib/playground-store";
import { ChevronDown } from "lucide-react";

export function ModelSelector({
	models,
	value,
	onChange,
	compact = false,
}: {
	models: AiandModel[];
	value: PlaygroundModel;
	onChange: (model: PlaygroundModel) => void;
	compact?: boolean;
}) {
	const select = (
		<select
			aria-label="Model selector"
			className={
				compact
					? "h-8 max-w-[240px] appearance-none truncate rounded-lg border border-border bg-background pl-3 pr-8 text-xs text-foreground"
					: "h-9 w-full rounded-md border border-border bg-background px-2 text-sm text-foreground"
			}
			value={value ?? ""}
			onChange={(e) => onChange(e.target.value === "" ? null : e.target.value)}
		>
			<option value="">Auto route</option>
			{models.map((m) => (
				<option key={m.id} value={m.id}>
					{m.id}
				</option>
			))}
		</select>
	);

	if (compact) {
		return (
			<div className="relative min-w-0">
				{select}
				<ChevronDown className="pointer-events-none absolute right-2 top-1/2 size-3.5 -translate-y-1/2 text-muted-foreground" />
			</div>
		);
	}

	return (
		<label className="flex flex-col gap-1 text-xs text-muted-foreground">
			Model
			{select}
		</label>
	);
}
