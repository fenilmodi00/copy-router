"use client";

import { Text } from "@/components/atoms/Text";
import type { AiandModel } from "@/lib/api";
import {
	normalizeReasoningForModel,
	reasoningEffortsForModel,
} from "@/lib/playground-reasoning";
import type { PlaygroundModel, PlaygroundParams } from "@/lib/playground-store";
import { DEFAULT_PLAYGROUND_PARAMS } from "@/lib/playground-store";
import { Settings2 } from "lucide-react";
import { useEffect } from "react";
import { Toggle } from "./Toggle";

const ROUTER_DEFAULT_VALUE = "";

export function ControlPanel({
	params,
	onChange,
	model,
	models,
}: {
	params: PlaygroundParams;
	onChange: (params: PlaygroundParams) => void;
	model: PlaygroundModel;
	models: AiandModel[];
}) {
	const temperature = params.temperature ?? DEFAULT_PLAYGROUND_PARAMS.temperature!;
	const maxTokens = params.max_tokens ?? DEFAULT_PLAYGROUND_PARAMS.max_tokens!;
	const stream = params.stream ?? DEFAULT_PLAYGROUND_PARAMS.stream!;
	const efforts = reasoningEffortsForModel(model, models);
	const normalizedReasoning = normalizeReasoningForModel(params.reasoning, model, models);
	const reasoningValue =
		normalizedReasoning ?? (model == null ? ROUTER_DEFAULT_VALUE : efforts[0] ?? ROUTER_DEFAULT_VALUE);

	useEffect(() => {
		const next = normalizeReasoningForModel(params.reasoning, model, models);
		if (params.reasoning !== next) {
			onChange({ ...params, reasoning: next });
		}
	}, [model, models, onChange, params]);

	return (
		<aside className="flex w-72 shrink-0 flex-col border-l border-border bg-background">
			<div className="flex items-center gap-2 border-b border-border px-4 py-3">
				<Settings2 className="size-4 text-muted-foreground" />
				<Text variant="h4" as="p" className="text-sm">
					Control Panel
				</Text>
			</div>
			<div className="flex flex-col gap-5 p-4">
				<SliderRow
					label="Temperature"
					value={temperature}
					min={0}
					max={2}
					step={0.1}
					display={Number.isInteger(temperature) ? String(temperature) : temperature.toFixed(1)}
					onChange={(v) => onChange({ ...params, temperature: v })}
				/>
				<SliderRow
					label="Max Tokens"
					value={maxTokens}
					min={256}
					max={8192}
					step={256}
					display={String(maxTokens)}
					onChange={(v) => onChange({ ...params, max_tokens: v })}
				/>
				{efforts.length > 0 ? (
					<label className="flex flex-col gap-2">
						<span className="text-xs text-muted-foreground">Reasoning</span>
						<select
							aria-label="Reasoning effort"
							className="h-9 rounded-md border border-border bg-background px-2 text-sm text-foreground"
							value={reasoningValue}
							onChange={(e) => {
								const next = e.target.value;
								onChange({
									...params,
									reasoning: next === ROUTER_DEFAULT_VALUE ? undefined : next,
								});
							}}
						>
							{model == null ? (
								<option value={ROUTER_DEFAULT_VALUE}>Router default</option>
							) : null}
							{efforts.map((effort) => (
								<option key={effort} value={effort}>
									{effort}
								</option>
							))}
						</select>
					</label>
				) : null}
				<div className="flex items-center justify-between gap-3">
					<Text className="text-xs text-muted-foreground">Stream</Text>
					<Toggle
						checked={stream}
						onChange={(next) => onChange({ ...params, stream: next })}
						label="Stream"
					/>
				</div>
			</div>
		</aside>
	);
}

function SliderRow({
	label,
	value,
	min,
	max,
	step,
	display,
	onChange,
}: {
	label: string;
	value: number;
	min: number;
	max: number;
	step: number;
	display: string;
	onChange: (value: number) => void;
}) {
	return (
		<label className="flex flex-col gap-2">
			<div className="flex items-center justify-between gap-2">
				<span className="text-xs text-muted-foreground">{label}</span>
				<span className="text-xs tabular-nums text-foreground">{display}</span>
			</div>
			<input
				type="range"
				min={min}
				max={max}
				step={step}
				value={value}
				aria-label={label}
				className="w-full accent-foreground"
				onChange={(e) => onChange(Number(e.target.value))}
			/>
		</label>
	);
}
