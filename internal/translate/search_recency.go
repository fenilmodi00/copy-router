package translate

import (
	"strings"

	"github.com/tidwall/gjson"
)

// SearchToolUseRecency reports how many assistant turns have elapsed since the last actual
// web-search/fetch tool invocation (not mere advertisement): 0 = current turn,
// N = N turns ago, -1 = no search use in history.
func (e *RequestEnvelope) SearchToolUseRecency() int {
	switch e.format {
	case FormatAnthropic:
		if isCurrentTurnSearchUse(e.body) {
			return 0
		}
		return searchUseRecency(gjson.GetBytes(e.body, "messages"), anthropicSearchUse)
	case FormatOpenAI:
		items := gjson.GetBytes(e.body, "messages")
		if !items.Exists() {
			items = gjson.GetBytes(e.body, "input")
		}
		return searchUseRecency(items, openAISearchUse)
	default:
		return -1
	}
}

// isCurrentTurnSearchUse reports whether the body is a self-contained
// web-search turn: one user message whose only purpose is to run the search
// (forced tool_choice naming the server tool, or text carrying Claude Code's
// search sub-turn prompt), with the native tool declared. Fork-adapted from
// upstream's websearch.DetectSearchTurn: same check, inlined so the deleted
// internal/websearch package stays deleted.
func isCurrentTurnSearchUse(body []byte) bool {
	tool, ok := findServerTool(body)
	if !ok {
		return false
	}
	messages := gjson.GetBytes(body, "messages").Array()
	if len(messages) != 1 || messages[0].Get("role").String() != "user" {
		return false
	}
	text := strings.TrimSpace(userText(messages[0]))
	if text == "" {
		return false
	}
	// A forced choice only counts when it names the search tool; forcing some
	// other tool is an ordinary turn that happens to also declare web_search.
	forced := gjson.GetBytes(body, "tool_choice.type").String() == "tool" &&
		gjson.GetBytes(body, "tool_choice.name").String() == tool
	if !forced && !strings.HasPrefix(text, claudeCodeSearchPrompt) {
		return false
	}
	return true
}

// findServerTool returns the name of the first native web-search server tool
// declared in an Anthropic Messages body.
func findServerTool(body []byte) (string, bool) {
	name := ""
	found := false
	gjson.GetBytes(body, "tools").ForEach(func(_, tool gjson.Result) bool {
		if !strings.HasPrefix(tool.Get("type").String(), serverToolPrefix) {
			return true
		}
		name = tool.Get("name").String()
		if name == "" {
			name = "web_search"
		}
		found = true
		return false
	})
	return name, found
}

// userText concatenates the text of a message whose content is either a bare
// string or Anthropic's content-block array.
func userText(message gjson.Result) string {
	content := message.Get("content")
	if content.Type == gjson.String {
		return content.String()
	}
	var b strings.Builder
	content.ForEach(func(_, block gjson.Result) bool {
		if block.Get("type").String() == "text" {
			b.WriteString(block.Get("text").String())
			b.WriteString("\n")
		}
		return true
	})
	return b.String()
}

// claudeCodeSearchPrompt is the sub-turn Claude Code sends when its WebSearch
// tool fires: a single user message whose text is this prefix plus the query.
const claudeCodeSearchPrompt = "Perform a web search for the query: "

// searchUseRecency walks the conversation in order, counting assistant turns
// after the most recent item usedFn matches.
func searchUseRecency(items gjson.Result, usedFn func(gjson.Result) bool) int {
	recency := -1
	items.ForEach(func(_, item gjson.Result) bool {
		if usedFn(item) {
			recency = 0
			return true
		}
		if recency >= 0 && item.Get("role").String() == "assistant" {
			recency++
		}
		return true
	})
	return recency
}

func anthropicSearchUse(message gjson.Result) bool {
	used := false
	message.Get("content").ForEach(func(_, block gjson.Result) bool {
		switch block.Get("type").String() {
		case "server_tool_use", "web_search_tool_result", "web_fetch_tool_result":
			used = true
		case "tool_use":
			name := block.Get("name").String()
			used = name == "WebSearch" || name == "WebFetch"
		}
		return !used
	})
	return used
}

func openAISearchUse(item gjson.Result) bool {
	return strings.HasPrefix(item.Get("type").String(), "web_search_call")
}
