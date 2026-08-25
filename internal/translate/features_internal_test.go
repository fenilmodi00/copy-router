package translate

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestBase64SignatureBytes sums the byte length of every base64
// thought-signature payload and ignores everything else.
func TestBase64SignatureBytes(t *testing.T) {
	assert.Equal(t, 0, base64SignatureBytes([]byte(`{"messages":[]}`)), "no signature field")
	assert.Equal(t, 6, base64SignatureBytes([]byte(`{"signature":"ABCabc"}`)), "single payload counts its base64 bytes only")
	assert.Equal(t, 10, base64SignatureBytes([]byte(`{"signature":"AAAA"},{"signature":"BBBBBB"}`)), "two payloads sum (4 + 6)")
	// A marker with no closing quote is not counted (truncated / malformed).
	assert.Equal(t, 0, base64SignatureBytes([]byte(`{"signature":"AAAA`)), "unterminated payload is skipped")
}

// TestContextOverflowTokenEstimate_FullBody divides the full body bytes
// (signatures included — the count a signature-keeping target receives) by the
// dense-content ratio.
// TestContextOverflowTokenEstimate_FullBody divides the full body bytes
// (signatures included — the count a signature-keeping target receives) by the
// dense-content ratio.
func TestContextOverflowTokenEstimate_FullBody(t *testing.T) {
	body := []byte(strings.Repeat("x", 400))
	e := &RequestEnvelope{body: body, format: FormatAnthropic}
	assert.Equal(t, 100, e.ContextOverflowTokenEstimate(), "400 bytes / 4 = 100 tokens")
}

// TestSignatureTokenSavings returns the token savings a signature-stripping
// target gets from dropping the base64 payloads — but only for Anthropic-format
// input; other formats carry no Anthropic thought-signatures to strip.
// TestSignatureTokenSavings returns the token savings a signature-stripping
// target gets from dropping the base64 payloads — but only for Anthropic-format
// input; other formats carry no Anthropic thought-signatures to strip.
func TestSignatureTokenSavings(t *testing.T) {
	sig := strings.Repeat("A", 800)
	body := []byte(`{"content":"` + strings.Repeat("x", 400) + `","signature":"` + sig + `"}`)

	anthropic := &RequestEnvelope{body: body, format: FormatAnthropic}
	assert.Equal(t, 200, anthropic.SignatureTokenSavings(), "800 signature bytes / 4 = 200 tokens saved")

	// Same bytes arriving as an OpenAI body: the "signature" field is caller
	// data, not an Anthropic block, so nothing is stripped and nothing is saved.
	openai := &RequestEnvelope{body: body, format: FormatOpenAI}
	assert.Equal(t, 0, openai.SignatureTokenSavings(), "non-Anthropic format saves nothing")
}

// TestContextOverflowTokenEstimate_TicketRegression is the regression for the
// 262K-overflow ticket: a signature-light, content-dense ~1.05MB body is a real
// ~263K-token prompt. The old ÷6 estimate (~175K) let it pass the pre-filter
// onto a 256K OSS model, which then hard-400'd on context overflow. The
// strip-aware ÷4 estimate must land above that window so the model is excluded.
// TestContextOverflowTokenEstimate_TicketRegression is the regression for the
// 262K-overflow ticket: a signature-light, content-dense ~1.05MB body is a real
// ~263K-token prompt. The old ÷6 estimate (~175K) let it pass the pre-filter
// onto a 256K OSS model, which then hard-400'd on context overflow. The
// strip-aware ÷4 estimate must land above that window so the model is excluded.
func TestContextOverflowTokenEstimate_TicketRegression(t *testing.T) {
	const kimiWindow = 262_143
	body := []byte(strings.Repeat("x", 1_050_000))
	e := &RequestEnvelope{body: body, format: FormatAnthropic}

	assert.Greater(t, e.ContextOverflowTokenEstimate(), kimiWindow, "dense ~263K-token body must estimate above a 256K window")
	assert.Less(t, e.FullTokenEstimate(), kimiWindow, "the old ÷6 estimate undercounted below the window — the bug this fixes")
}

// TestBase64ImageStats sums inline base64 image payloads per inbound format,
// covering top-level and tool_result-nested Anthropic images, OpenAI data URLs
// (http URLs skipped), and Gemini inlineData (camelCase + snake_case).
// TestContextOverflowTokenEstimate_ImagesRepriced is the regression for
// phantom token inflation on multi-page PDF reads: base64 transport size
// must not be counted at the content byte ratio.
func TestContextOverflowTokenEstimate_ImagesRepriced(t *testing.T) {
	const pages = 20
	const pageBytes = 250_000 // ~250KB base64 per rendered page
	page := strings.Repeat("A", pageBytes)
	blocks := make([]string, pages)
	for i := range blocks {
		blocks[i] = `{"type":"image","source":{"type":"base64","media_type":"image/jpeg","data":"` + page + `"}}`
	}
	body := []byte(`{"messages":[{"role":"user","content":[` + strings.Join(blocks, ",") +
		`,{"type":"text","text":"summarize this paper"}]}]}`)
	e := &RequestEnvelope{body: body, format: FormatAnthropic}

	imgBytes, imgCount := e.base64ImageStats()
	assert.Equal(t, pages, imgCount, "counts one image per page")
	assert.Equal(t, pages*pageBytes, imgBytes, "sums each page's base64 bytes")

	// The naive len/4 estimate is a phantom >1M-token overflow (the bug).
	assert.Greater(t, len(body)/contentBytesPerToken, 1_000_000, "raw body ÷4 falsely overflows")
	// Repriced, the same body is well below any compaction trigger.
	assert.Less(t, e.ContextOverflowTokenEstimate(), 100_000, "repriced estimate stays far below the window")
}
