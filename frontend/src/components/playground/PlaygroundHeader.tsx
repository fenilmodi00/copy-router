"use client";

import { Text } from "@/components/atoms/Text";
import { PageHeader } from "@/components/PageHeader";
import { api } from "@/lib/api";
import { cn } from "@/lib/cn";
import { newSessionID, usePlaygroundStore } from "@/lib/playground-store";
import { useLoginSession } from "@/lib/use-login-session-gate";
import { ChevronDown } from "lucide-react";
import useSWR from "swr";

function initialsFromName(name: string): string {
	const parts = name.trim().split(/\s+/).filter(Boolean);
	if (parts.length === 0) return "?";
	if (parts.length === 1) return parts[0].slice(0, 2).toUpperCase();
	return `${parts[0][0] ?? ""}${parts[1][0] ?? ""}`.toUpperCase();
}

function truncateSession(id: string): string {
	if (id.length <= 12) return id;
	return `${id.slice(0, 10)}…`;
}

export function PlaygroundHeader() {
	const { surface } = useLoginSession();
	const sessionID = usePlaygroundStore((s) => s.sessionID);
	const setSessionID = usePlaygroundStore((s) => s.setSessionID);
	const clear = usePlaygroundStore((s) => s.clear);

	const accountQ = useSWR(
		surface === "account" ? ["account-me"] : null,
		() => api.auth.accountMe(),
	);

	const displayName = accountQ.data?.display_name ?? "User";
	const avatarInitials = initialsFromName(displayName);

	function startNewSession() {
		setSessionID(newSessionID());
		clear();
	}

	return (
		<PageHeader
			left={
				<Text variant="h4" as="h2">
					Playground
				</Text>
			}
			right={
				<div className="flex items-center gap-3">
					<div className="relative">
						<select
							aria-label="Playground session"
							className={cn(
								"h-8 appearance-none rounded-lg border border-border bg-muted pl-3 pr-8 text-xs text-foreground",
							)}
							value={sessionID}
							onChange={(e) => {
								if (e.target.value === "__new__") {
									startNewSession();
									return;
								}
								setSessionID(e.target.value);
							}}
						>
							{sessionID ? (
								<option value={sessionID}>{truncateSession(sessionID)}</option>
							) : (
								<option value="">New session</option>
							)}
							<option value="__new__">Start new session</option>
						</select>
						<ChevronDown className="pointer-events-none absolute right-2 top-1/2 size-3.5 -translate-y-1/2 text-muted-foreground" />
					</div>

					<div
						className="flex size-8 items-center justify-center rounded-full bg-muted text-2xs font-medium text-foreground"
						title={displayName}
						aria-label={`Signed in as ${displayName}`}
					>
						{avatarInitials}
					</div>
				</div>
			}
		/>
	);
}
