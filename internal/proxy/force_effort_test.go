package proxy

import "testing"

// forcedReasoningEffort encodes the escalate-on-failure policy: gpt-5.x low by
// default / high on a failed prior turn; gemini-3.x pinned low; everything else
// untouched (""). Policy is name-prefix based, not catalog-ID based.
func TestForcedReasoningEffort(t *testing.T) {
	cases := []struct {
		model    string
		escalate bool
		want     string
	}{
		{"gpt-5.4", false, "low"},
		{"gpt-5.4-mini", true, "high"},
		{"gpt-5.5", false, "low"},
		{"gpt-5.6-sol", true, "high"},
		{"gemini-3-pro-preview", false, "low"},
		{"gemini-3.5-flash", true, "low"}, // effort-immune: escalation ignored
		{"grok-4.6", false, "low"},        // unconditional: bare pin must not fall to xAI's non-disableable high default
		{"grok-4.5", true, "low"},         // floor ignores escalation
		{"grok-4", false, "low"},
		{"claude-opus-5", false, ""}, // adaptive path untouched
		{"claude-sonnet-5", true, ""},
		{"deepseek-ai/deepseek-v4-pro", true, ""},
		{"gemini-2.5-flash", true, ""}, // only gemini-3* is pinned
	}
	for _, tc := range cases {
		got := forcedReasoningEffort(tc.model, tc.escalate)
		if got != tc.want {
			t.Errorf("forcedReasoningEffort(%q, %v) = %q, want %q", tc.model, tc.escalate, got, tc.want)
		}
	}
}
