"use client";

import { Text } from "@/components/atoms/Text";
import { Button } from "@/components/molecules/Button";
import { Appearance } from "@/components/types";
import type { AiandModel } from "@/lib/api";
import type { PlaygroundModel } from "@/lib/playground-store";
import { Check, Copy, Info } from "lucide-react";
import { useState } from "react";
import { ModelSelector } from "./ModelSelector";
import { Toggle } from "./Toggle";

export function PlaygroundToolbar({
	models,
	model,
	onModelChange,
	showDetails,
	onShowDetailsChange,
	viewCode,
	onViewCodeChange,
	onCopy,
}: {
	models: AiandModel[];
	model: PlaygroundModel;
	onModelChange: (model: PlaygroundModel) => void;
	showDetails: boolean;
	onShowDetailsChange: (show: boolean) => void;
	viewCode: boolean;
	onViewCodeChange: (show: boolean) => void;
	onCopy: () => void;
}) {
	const [copied, setCopied] = useState(false);

	function handleCopy() {
		onCopy();
		setCopied(true);
		window.setTimeout(() => setCopied(false), 2000);
	}

	return (
		<div className="flex w-full min-w-0 items-center gap-3 px-6 py-2">
			<div className="flex min-w-0 flex-1 items-center gap-2">
				<ModelSelector models={models} value={model} onChange={onModelChange} compact />
				<Button
					type="button"
					size="sm"
					appearance={Appearance.Outlined}
					onClick={handleCopy}
					aria-label="Copy model"
				>
					{copied ? <Check className="size-3.5" /> : <Copy className="size-3.5" />}
					Copy
				</Button>
				<Button
					type="button"
					size="sm"
					appearance={showDetails ? Appearance.Filled : Appearance.Outlined}
					onClick={() => onShowDetailsChange(!showDetails)}
					aria-pressed={showDetails}
					aria-label="Details"
				>
					<Info className="size-3.5" />
					Details
				</Button>
			</div>
			<div className="flex shrink-0 items-center gap-2">
				<Text className="text-xs text-muted-foreground">View Code</Text>
				<Toggle
					checked={viewCode}
					onChange={onViewCodeChange}
					label="View Code"
				/>
			</div>
		</div>
	);
}
