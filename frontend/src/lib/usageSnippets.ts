export type SnippetLanguage = "curl" | "python" | "typescript";

export type SnippetMode = "auto" | "force";

export interface SnippetContext {
  /** Empty string during SSR. */
  origin: string;
  apiKey: string;
  forceModel?: string;
}

export interface SnippetSpec {
  id: SnippetLanguage;
  label: string;
  detail: string;
}

export const KEY_PLACEHOLDER = "<YOUR_ROUTER_KEY>";

export const DEFAULT_FORCE_MODEL = "moonshotai/kimi-k2.7";

export const SNIPPET_LANGUAGES: SnippetSpec[] = [
  {
    id: "curl",
    label: "curl",
    detail: "Raw HTTP against the OpenAI Chat Completions endpoint.",
  },
  {
    id: "python",
    label: "Python",
    detail: "OpenAI Python SDK with base_url pointed at this router.",
  },
  {
    id: "typescript",
    label: "TypeScript",
    detail: "OpenAI TypeScript SDK with baseURL pointed at this router.",
  },
];

export function snippetLanguage(id: SnippetLanguage): SnippetSpec {
  const found = SNIPPET_LANGUAGES.find(s => s.id === id);
  if (found == null) throw new Error(`unknown snippet language: ${id}`);
  return found;
}

/** Empty origin is allowed; callers gate the UI rather than inventing a host. */
export function usageSnippet(lang: SnippetLanguage, mode: SnippetMode, ctx: SnippetContext): string {
  const apiKey = ctx.apiKey || KEY_PLACEHOLDER;
  const forceModel = ctx.forceModel ?? DEFAULT_FORCE_MODEL;
  const origin = ctx.origin;
  const baseURL = `${origin}/v1`;
  const completionsURL = `${origin}/v1/chat/completions`;

  switch (lang) {
    case "curl":
      return curlSnippet(completionsURL, apiKey, mode, forceModel);
    case "python":
      return pythonSnippet(baseURL, apiKey, mode, forceModel);
    case "typescript":
      return typescriptSnippet(baseURL, apiKey, mode, forceModel);
  }
}

function curlSnippet(url: string, apiKey: string, mode: SnippetMode, forceModel: string): string {
  const model = mode === "force" ? forceModel : "auto";
  return `curl ${url} \\
  -H "Authorization: Bearer ${apiKey}" \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "${model}",
    "messages": [
      {"role": "user", "content": "Hello"}
    ]
  }'`;
}

function pythonSnippet(baseURL: string, apiKey: string, mode: SnippetMode, forceModel: string): string {
  const model = mode === "force" ? forceModel : "auto";
  return `from openai import OpenAI

client = OpenAI(
    base_url="${baseURL}",
    api_key="${apiKey}",
)

completion = client.chat.completions.create(
    model="${model}",
    messages=[
        {"role": "user", "content": "Hello"},
    ],
)
print(completion.choices[0].message.content)`;
}

function typescriptSnippet(
  baseURL: string,
  apiKey: string,
  mode: SnippetMode,
  forceModel: string,
): string {
  const model = mode === "force" ? forceModel : "auto";
  return `import OpenAI from "openai";

const client = new OpenAI({
  baseURL: "${baseURL}",
  apiKey: "${apiKey}",
});

const completion = await client.chat.completions.create({
  model: "${model}",
  messages: [{ role: "user", content: "Hello" }],
});
console.log(completion.choices[0].message.content);`;
}
