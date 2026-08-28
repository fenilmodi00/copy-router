"use client";

import { cn } from "@/lib/cn";

export function Toggle({
	checked,
	onChange,
	label,
	className,
}: {
	checked: boolean;
	onChange: (checked: boolean) => void;
	label: string;
	className?: string;
}) {
	return (
		<button
			type="button"
			role="switch"
			aria-checked={checked}
			aria-label={label}
			onClick={() => onChange(!checked)}
			className={cn(
				"relative inline-flex h-5 w-9 shrink-0 rounded-full border border-border transition-colors",
				checked ? "bg-foreground" : "bg-muted",
				className,
			)}
		>
			<span
				className={cn(
					"pointer-events-none absolute top-0.5 size-4 rounded-full bg-background shadow-sm transition-transform",
					checked ? "translate-x-[18px]" : "translate-x-0.5",
				)}
			/>
		</button>
	);
}
