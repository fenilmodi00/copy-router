// Sample-cost verdict math shared by the compare page and its tests. The two
// shapes the spec prices: a 15K-prompt + 35K-completion sample, and that same
// sample assuming CACHE_HIT_RATE of prompt tokens hit the prompt cache.
export const SAMPLE_PROMPT_IN = 15_000;
export const SAMPLE_COMPLETION_OUT = 35_000;
export const CACHE_HIT_RATE = 0.7;

export function plainVerdict(inputPer1M: string, outputPer1M: string): number {
  const input = Number(inputPer1M);
  const output = Number(outputPer1M);
  return (SAMPLE_PROMPT_IN * input + SAMPLE_COMPLETION_OUT * output) / 1_000_000;
}

export function cachedVerdict(
  inputPer1M: string,
  outputPer1M: string,
  cachedInputPer1M: string,
): number {
  const input = Number(inputPer1M);
  const output = Number(outputPer1M);
  const cached = Number(cachedInputPer1M);
  const prompt =
    SAMPLE_PROMPT_IN * (1 - CACHE_HIT_RATE) * input +
    SAMPLE_PROMPT_IN * CACHE_HIT_RATE * cached;
  return (prompt + SAMPLE_COMPLETION_OUT * output) / 1_000_000;
}