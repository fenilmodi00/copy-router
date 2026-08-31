// Command record_provider_fixtures refreshes the translation-conformance
// upstream fixtures from the live ai& upstream. For each case it runs the SAME
// inbound Anthropic body through the router's own Prepare* emit, sends the
// translated request to the real upstream, and writes the raw response to the
// fixture the conformance suite (internal/proxy/conformance_*_test.go) replays
// offline.
//
// It is a separate main package (not a _test.go), so `go test ./...` never runs
// it and CI never touches the network. It is further gated on RECORD=1 and
// AIAND_API_KEY being present.
//
// Usage (from the repo root):
//
//	RECORD=1 AIAND_API_KEY=… go run ./scripts/record_provider_fixtures
//
// After recording, regenerate the goldens and review the diff:
//
//	go test ./internal/proxy/ -run TestConformance -update
//	git diff internal/proxy/testdata/conformance
package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"workweave/router/internal/observability"
	"workweave/router/internal/providers"
	"workweave/router/internal/providers/openaicompat"
	"workweave/router/internal/router"
	"workweave/router/internal/router/catalog"
	"workweave/router/internal/translate"
)

// fixtureRoot is where the conformance suite reads upstream fixtures from,
// relative to the repo root (the recorder's expected working directory).
const fixtureRoot = "internal/proxy/testdata/conformance"

type format int

const (
	formatOpenAIChat format = iota
	formatOpenAIResponses
)

type recordCase struct {
	fixture       string // path under fixtureRoot, e.g. "openai_chat/basic_text.upstream.sse"
	format        format
	model         string // catalog model ID; UpstreamID is used when non-empty
	anthropicBody string
}

// cases mirror the conformance suite's fixtures, restricted to models the
// catalog binds to ai&. Keep them in sync: a new conformance case that wants
// live-recorded input adds an entry here.
var cases = []recordCase{
	{"openai_chat/basic_text.upstream.sse", formatOpenAIChat, "deepseek-ai/deepseek-v4-flash",
		`{"model":"deepseek-ai/deepseek-v4-flash","stream":true,"max_tokens":1024,"messages":[{"role":"user","content":"Say hi."}]}`},
	{"openai_chat/toolcall.upstream.sse", formatOpenAIChat, "deepseek-ai/deepseek-v4-flash",
		`{"model":"deepseek-ai/deepseek-v4-flash","stream":true,"max_tokens":1024,"tools":` + weatherTool + `,"messages":[{"role":"user","content":"Weather in NYC?"}]}`},
	{"responses/toolcall.upstream.sse", formatOpenAIResponses, "moonshotai/kimi-k2.7",
		`{"model":"moonshotai/kimi-k2.7","stream":true,"max_tokens":2048,"thinking":{"type":"enabled","budget_tokens":24576},"tools":` + weatherTool + `,"messages":[{"role":"user","content":"Weather in NYC?"}]}`},
}

const weatherTool = `[{"name":"get_weather","description":"Get the weather","input_schema":{"type":"object","properties":{"city":{"type":"string"}},"required":["city"]}}]`

func main() {
	if os.Getenv("RECORD") != "1" {
		fmt.Println("Refusing to hit live providers without RECORD=1 (see file header for usage)")
		return
	}
	client := &http.Client{Timeout: 180 * time.Second}
	var recorded, skipped, failed int
	for _, c := range cases {
		key := os.Getenv("AIAND_API_KEY")
		if key == "" {
			fmt.Println("SKIP  (AIAND_API_KEY not set)")
			skipped++
			continue
		}
		if err := record(client, c, key); err != nil {
			fmt.Printf("FAIL  %s: %v\n", c.fixture, err)
			failed++
			continue
		}
		fmt.Printf("OK    %s\n", c.fixture)
		recorded++
	}
	fmt.Printf("\nrecorded=%d skipped=%d failed=%d\n", recorded, skipped, failed)
	if failed > 0 {
		os.Exit(1)
	}
}

// upstreamID returns the wire model name for a catalog model: the ai& binding's
// UpstreamID when non-empty, else the catalog ID.
func upstreamID(modelID string) string {
	m, ok := catalog.ByID(modelID)
	if !ok || len(m.Providers) == 0 {
		return modelID
	}
	if u := m.Providers[0].UpstreamID; u != "" {
		return u
	}
	return m.ID
}

func record(client *http.Client, c recordCase, apiKey string) (err error) {
	model := upstreamID(c.model)
	env, err := translate.ParseAnthropic([]byte(c.anthropicBody))
	if err != nil {
		return fmt.Errorf("parse anthropic body: %w", err)
	}
	opts := translate.EmitOptions{
		TargetModel:    model,
		TargetProvider: providers.ProviderAiand,
		Capabilities:   router.Lookup(model),
	}

	prep, err := prepare(env, c.format, opts)
	if err != nil {
		return fmt.Errorf("emit upstream request: %w", err)
	}

	url, hdr := endpoint(c.format, apiKey)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, url, bytes.NewReader(prep.Body))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	for k, v := range hdr {
		req.Header.Set(k, v)
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("upstream call: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("upstream status %d: %s", resp.StatusCode, observability.Preview(string(body), 400))
	}

	path := filepath.Join(fixtureRoot, c.fixture)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, body, 0o644)
}

func prepare(env *translate.RequestEnvelope, f format, opts translate.EmitOptions) (providers.PreparedRequest, error) {
	switch f {
	case formatOpenAIChat:
		return env.PrepareOpenAI(http.Header{}, opts)
	case formatOpenAIResponses:
		return env.PrepareOpenAIResponses(http.Header{}, opts)
	default:
		return providers.PreparedRequest{}, fmt.Errorf("unknown format %d", f)
	}
}

// endpoint returns the live ai& upstream URL and auth headers for a case.
func endpoint(f format, apiKey string) (string, map[string]string) {
	switch f {
	case formatOpenAIChat:
		return openaicompat.AiandBaseURL + "/chat/completions", map[string]string{"Authorization": "Bearer " + apiKey}
	case formatOpenAIResponses:
		return openaicompat.AiandBaseURL + "/responses", map[string]string{"Authorization": "Bearer " + apiKey}
	default:
		return "", nil
	}
}
