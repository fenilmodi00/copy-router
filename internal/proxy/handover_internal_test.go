package proxy

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"

	"workweave/router/internal/providers"
	"workweave/router/internal/router"
	"workweave/router/internal/translate"
)

// fakeHandoverProvider is the minimal providers.Client surface needed by
// the summarizer test: it can return a canned non-streaming body, sleep
// past a deadline, or surface a non-2xx status. It also captures the last
// PreparedRequest so tests can assert OpenAI Chat Completions shape.
type fakeHandoverProvider struct {
	respBody    string
	respStatus  int
	sleep       time.Duration
	upstreamErr error

	lastDecision router.Decision
	lastPrep     providers.PreparedRequest
	lastPath     string
}

func (f *fakeHandoverProvider) Proxy(ctx context.Context, d router.Decision, prep providers.PreparedRequest, w http.ResponseWriter, r *http.Request) error {
	f.lastDecision = d
	f.lastPrep = prep
	if r != nil {
		f.lastPath = r.URL.Path
	}
	if f.sleep > 0 {
		select {
		case <-time.After(f.sleep):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	if f.upstreamErr != nil {
		return f.upstreamErr
	}
	if f.respStatus == 0 {
		w.WriteHeader(http.StatusOK)
	} else {
		w.WriteHeader(f.respStatus)
	}
	_, _ = io.WriteString(w, f.respBody)
	return nil
}

func (f *fakeHandoverProvider) Passthrough(_ context.Context, _ providers.PreparedRequest, _ http.ResponseWriter, _ *http.Request) error {
	return nil
}

// sampleConversation is the test fixture used across the cases. Both
// system and a couple of message turns so buildSummaryRequestBody has
// real content to flatten.
const sampleConversation = `{
  "model": "moonshotai/kimi-k3",
  "system": "You are a helpful assistant.",
  "messages": [
    {"role": "user", "content": "Step 1?"},
    {"role": "assistant", "content": "Done."},
    {"role": "user", "content": "Step 2?"}
  ]
}`

// canonicalOpenAIResponse is what a real non-streaming OpenAI
// /v1/chat/completions response looks like. The summarizer extracts
// choices[0].message.content and prompt_tokens/completion_tokens.
const canonicalOpenAIResponse = `{
  "id": "chatcmpl_test_001",
  "object": "chat.completion",
  "model": "deepseek-ai/deepseek-v4-flash",
  "choices": [
    {
      "index": 0,
      "message": {"role": "assistant", "content": "Refactor in progress: step 1 done, step 2 pending."},
      "finish_reason": "stop"
    }
  ],
  "usage": {"prompt_tokens": 42, "completion_tokens": 17}
}`

func TestProviderSummarizer_SuccessReturnsAssistantText(t *testing.T) {
	t.Parallel()

	env, err := translate.ParseAnthropic([]byte(sampleConversation))
	require.NoError(t, err)

	fake := &fakeHandoverProvider{
		respBody:   canonicalOpenAIResponse,
		respStatus: http.StatusOK,
	}
	s := NewProviderSummarizer(fake, providers.ProviderAiand, "", 200*time.Millisecond)

	got, usage, err := s.Summarize(context.Background(), env)
	require.NoError(t, err)
	assert.Equal(t, "Refactor in progress: step 1 done, step 2 pending.", got)
	assert.Equal(t, providers.ProviderAiand, s.Provider())
	assert.Equal(t, providers.ProviderAiand, usage.Provider)
	assert.Equal(t, 42, usage.InputTokens)
	assert.Equal(t, 17, usage.OutputTokens)
	assert.Equal(t, DefaultHandoverModel, usage.Model)

	// OpenAI Chat Completions body shape — not Anthropic Messages.
	require.True(t, gjson.ValidBytes(fake.lastPrep.Body), "request body must be JSON")
	assert.Equal(t, "/v1/chat/completions", fake.lastPath)
	assert.Equal(t, providers.ProviderAiand, fake.lastDecision.Provider)
	assert.False(t, gjson.GetBytes(fake.lastPrep.Body, "stream").Bool())
	assert.Equal(t, int64(DefaultHandoverMaxTokens), gjson.GetBytes(fake.lastPrep.Body, "max_tokens").Int())
	assert.False(t, gjson.GetBytes(fake.lastPrep.Body, "tools").Exists(), "summary call must not carry tools")
	msgs := gjson.GetBytes(fake.lastPrep.Body, "messages")
	require.True(t, msgs.IsArray())
	require.GreaterOrEqual(t, len(msgs.Array()), 1)
	last := msgs.Array()[len(msgs.Array())-1]
	assert.Equal(t, "user", last.Get("role").String())
	assert.Equal(t, gjson.String, last.Get("content").Type, "OpenAI instruction content must be a string, not Anthropic content[]")
	assert.False(t, last.Get("content").IsArray(), "must not emit Anthropic-style content array")
	assert.Contains(t, last.Get("content").String(), "Summarize the conversation")
}

func TestProviderSummarizer_TimeoutReturnsError(t *testing.T) {
	t.Parallel()

	env, err := translate.ParseAnthropic([]byte(sampleConversation))
	require.NoError(t, err)

	fake := &fakeHandoverProvider{
		respBody: canonicalOpenAIResponse,
		// Sleep longer than the summarizer's timeout.
		sleep: 200 * time.Millisecond,
	}
	s := NewProviderSummarizer(fake, "", "", 25*time.Millisecond)

	got, _, err := s.Summarize(context.Background(), env)
	require.Error(t, err)
	assert.Empty(t, got)
	// Either the ctx.Err() bubble or the fake's own ctx-aware return both
	// surface DeadlineExceeded.
	assert.True(t, errors.Is(err, context.DeadlineExceeded), "expected DeadlineExceeded, got %v", err)
}

func TestProviderSummarizer_Non2xxReturnsError(t *testing.T) {
	t.Parallel()

	env, err := translate.ParseAnthropic([]byte(sampleConversation))
	require.NoError(t, err)

	fake := &fakeHandoverProvider{
		respBody:   `{"error":"oops"}`,
		respStatus: http.StatusInternalServerError,
	}
	s := NewProviderSummarizer(fake, "", "", 200*time.Millisecond)

	got, _, err := s.Summarize(context.Background(), env)
	require.Error(t, err)
	assert.Empty(t, got)
	assert.True(t, strings.Contains(err.Error(), "500"), "error must mention upstream status 500; got %v", err)
}

func TestProviderSummarizer_EmptyContentReturnsErrEmptySummary(t *testing.T) {
	t.Parallel()

	env, err := translate.ParseAnthropic([]byte(sampleConversation))
	require.NoError(t, err)

	// Successful 200 but no assistant text (empty choices / null content).
	fake := &fakeHandoverProvider{
		respBody:   `{"id":"chatcmpl_empty","choices":[{"message":{"role":"assistant","content":""},"finish_reason":"stop"}]}`,
		respStatus: http.StatusOK,
	}
	s := NewProviderSummarizer(fake, "", "", 200*time.Millisecond)

	got, _, err := s.Summarize(context.Background(), env)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrEmptySummary)
	assert.Empty(t, got)
}

func TestProviderSummarizer_NilEnvelopeReturnsError(t *testing.T) {
	t.Parallel()

	fake := &fakeHandoverProvider{}
	s := NewProviderSummarizer(fake, "", "", 200*time.Millisecond)

	_, _, err := s.Summarize(context.Background(), nil)
	require.Error(t, err)
}

func TestProviderSummarizer_DefaultProviderIsAiand(t *testing.T) {
	t.Parallel()
	s := NewProviderSummarizer(&fakeHandoverProvider{}, "", "", 0)
	assert.Equal(t, providers.ProviderAiand, s.Provider())
	// ProviderAnthropic constant must remain available for BYOK/gateway —
	// this summarizer simply must not hardcode it as the handover label.
	assert.NotEqual(t, providers.ProviderAnthropic, s.Provider())
	assert.Equal(t, "anthropic", providers.ProviderAnthropic)
}
