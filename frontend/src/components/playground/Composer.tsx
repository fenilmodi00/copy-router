"use client";

import { Button } from "@/components/molecules/Button";
import { Intent } from "@/components/types";
import type { PlaygroundStatus } from "@/lib/playground-store";
import { ArrowUp, Paperclip } from "lucide-react";
import { useState } from "react";

export function PlaygroundComposer({
	status,
	onSubmit,
	onStop,
}: {
	status: PlaygroundStatus;
	onSubmit: (text: string) => void;
	onStop: () => void;
}) {
	const [text, setText] = useState("");
	const streaming = status === "streaming";

	function submit() {
		const trimmed = text.trim();
		if (!trimmed || streaming) return;
		onSubmit(trimmed);
		setText("");
	}

	return (
		<div className="border-t border-border bg-background p-4">
			<div className="relative rounded-xl border border-border bg-background shadow-sm">
				<textarea
					aria-label="Message composer"
					className="min-h-[120px] w-full resize-none rounded-xl bg-transparent px-4 pb-12 pt-4 text-sm outline-none"
					value={text}
					disabled={streaming}
					onChange={(e) => setText(e.target.value)}
					onKeyDown={(e) => {
						if (e.key === "Enter" && !e.shiftKey) {
							e.preventDefault();
							submit();
						}
					}}
					placeholder="Type a message..."
				/>
				<div className="absolute inset-x-0 bottom-0 flex items-center justify-between px-4 pb-3">
					<div className="flex items-center gap-1.5 text-2xs text-muted-foreground">
						<Paperclip className="size-3.5" />
						<span>This model does not support file uploads.</span>
					</div>
					{streaming ? (
						<Button type="button" size="sm" intent={Intent.Danger} onClick={onStop}>
							Stop
						</Button>
					) : (
						<Button
							type="button"
							size="icon"
							intent={Intent.Primary}
							disabled={!text.trim()}
							aria-label="Send"
							className="rounded-full"
							onClick={submit}
						>
							<ArrowUp className="size-4" />
						</Button>
					)}
				</div>
			</div>
		</div>
	);
}

/** @deprecated Use PlaygroundComposer */
export const Composer = PlaygroundComposer;
