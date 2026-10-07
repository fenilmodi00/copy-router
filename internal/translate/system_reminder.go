package translate

// geminiSystemReminder returns a system-message addendum for Gemini 3.x. Empty
// for non-Gemini models or older Gemini families.
//
// Gemini 3.x in agentic-coding traces (SWE-bench Verified) reads source files
// extensively, then ends turns describing the proposed edit in markdown
// instead of calling Edit/Write. The behavior persists even when the request
// carries Edit/Write tool schemas. This addendum nudges it to apply edits via
// the tools when the user asked for a change.
//
// The nudge is deliberately CONDITIONAL. An earlier unconditional phrasing
// ("always call Edit/Write; a turn that only explores counts as giving up")
// framed every explore-only turn as failure, which tipped advisory/read-only
// requests ("what should I fix?") into unsolicited edits, commits, and PRs.
// The conditional form preserves the under-calling fix on genuine edit turns
// while leaving analysis/review requests in prose — mirroring the industry
// norm of mode-scoped guidance (Cline plan/act, Aider ask/code) over a blunt
// global imperative, which research shows over-pushes and is an unreliable
// governor of model action.
func geminiSystemReminder(model string) string {
	if isGemini3xModel(model) {
		return geminiToolUseReminder
	}
	return ""
}

const geminiToolUseReminder = "When the user has asked you to apply a change, prefer calling the Edit or Write tool over describing the edit in prose or markdown; once you have read enough to know what to change, the next step is the Edit/Write call, not a summary. If the user only asked for analysis, a review, or what could be fixed, answer in prose — do not edit, commit, or push unless they asked you to."
