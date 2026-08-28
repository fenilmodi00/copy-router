import type { AiandModel } from "@/lib/api";
import type { PlaygroundModel } from "@/lib/playground-store";

export function reasoningEffortsForModel(
	model: PlaygroundModel,
	models: AiandModel[],
): string[] {
	if (model != null) {
		const row = models.find((m) => m.id === model);
		if (row != null && row.reasoning_efforts.length > 0) {
			return row.reasoning_efforts;
		}
		return [];
	}
	const seen = new Set<string>();
	const union: string[] = [];
	for (const row of models) {
		for (const effort of row.reasoning_efforts) {
			if (!seen.has(effort)) {
				seen.add(effort);
				union.push(effort);
			}
		}
	}
	return union;
}

export function normalizeReasoningForModel(
	reasoning: string | undefined,
	model: PlaygroundModel,
	models: AiandModel[],
): string | undefined {
	const efforts = reasoningEffortsForModel(model, models);
	if (model == null) {
		if (reasoning == null || reasoning === "") return undefined;
		if (efforts.length > 0 && efforts.includes(reasoning)) return reasoning;
		return undefined;
	}
	if (efforts.length === 0) return undefined;
	if (reasoning != null && reasoning !== "" && efforts.includes(reasoning)) {
		return reasoning;
	}
	return efforts[0];
}

export function clampReasoningEffortForChat(
	reasoning: string | undefined,
	model: PlaygroundModel,
	models: AiandModel[],
): string | undefined {
	if (model == null) return undefined;
	return normalizeReasoningForModel(reasoning, model, models);
}
