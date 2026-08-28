import { describe, expect, it } from "vitest";
import {
  DEFAULT_FORCE_MODEL,
  KEY_PLACEHOLDER,
  usageSnippet,
} from "./usageSnippets";

const ctx = {
  origin: "https://router.example",
  apiKey: KEY_PLACEHOLDER,
};

describe("usageSnippet", () => {
  it("auto curl uses model auto and omits force-model header", () => {
    const snip = usageSnippet("curl", "auto", ctx);
    expect(snip).toMatch(/"model":\s*"auto"/);
    expect(snip).not.toContain("x-weave-force-model");
    expect(snip).toContain(`${ctx.origin}/v1/chat/completions`);
    expect(snip).toContain(`Authorization: Bearer ${KEY_PLACEHOLDER}`);
  });

  it("force curl puts the model in the body and the model id", () => {
    const snip = usageSnippet("curl", "force", ctx);
    expect(snip).toContain(DEFAULT_FORCE_MODEL);
    expect(snip).toContain(`"model": "${DEFAULT_FORCE_MODEL}"`);
    expect(snip).not.toContain("x-weave-force-model");
  });

  it("python uses base_url ending in /v1", () => {
    const snip = usageSnippet("python", "auto", ctx);
    expect(snip).toContain(`base_url="${ctx.origin}/v1"`);
    expect(snip).toContain("from openai import OpenAI");
    expect(snip).toContain('model="auto"');
  });

  it("typescript uses baseURL ending in /v1", () => {
    const snip = usageSnippet("typescript", "auto", ctx);
    expect(snip).toContain(`baseURL: "${ctx.origin}/v1"`);
    expect(snip).toContain('import OpenAI from "openai"');
    expect(snip).toContain('model: "auto"');
  });

  it("force python and typescript use model on the create call", () => {
    const py = usageSnippet("python", "force", {
      ...ctx,
      forceModel: "moonshotai/kimi-k2.7",
    });
    expect(py).toContain('model="moonshotai/kimi-k2.7"');
    expect(py).not.toContain("default_headers");

    const ts = usageSnippet("typescript", "force", ctx);
    expect(ts).toContain(`model: "${DEFAULT_FORCE_MODEL}"`);
    expect(ts).not.toContain("defaultHeaders");
  });

  it("empty origin still produces a string", () => {
    const snip = usageSnippet("curl", "auto", { origin: "", apiKey: KEY_PLACEHOLDER });
    expect(typeof snip).toBe("string");
    expect(snip.length).toBeGreaterThan(0);
    expect(snip).toContain("/v1/chat/completions");
  });
});