package translate

import (
	"strconv"
	"strings"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// nativeServerToolPrefixes lists the Anthropic server tools the provider
// executes server-side, not the model. Broader than serverToolPrefix: the
// search-recency walk only covers the tool the router knows how to emulate,
// which is a strictly smaller set than the tools a non-Anthropic upstream
// cannot run.
var nativeServerToolPrefixes = []string{serverToolPrefix, "web_fetch_"}

// serverToolPrefix marks Anthropic's dated native web-search server tools
// (their "type" embeds the signature date, e.g. web_search_20250305).
const serverToolPrefix = "web_search_"

// isNativeServerTool reports whether toolType names an Anthropic native
// server tool (web_search_*/web_fetch_*).
func isNativeServerTool(toolType string) bool {
	for _, prefix := range nativeServerToolPrefixes {
		if strings.HasPrefix(toolType, prefix) {
			return true
		}
	}
	return false
}

// stripServerTools removes Anthropic native server tools (web_search_*,
// web_fetch_*) from an Anthropic Messages body and reports how many it
// removed. Anthropic executes these itself; on non-Anthropic emit
// writeOpenAIToolsFromAnthropic would otherwise convert them into phantom
// OpenAI function tools with no input_schema — tools the upstream model can
// "call" but the client never registered a handler for (the same phantom-tool
// failure claudecode_tool_filter.go exists to prevent). A dangling
// tool_choice:{type:"tool",name:<stripped>} that would force the upstream to
// pick a removed tool is dropped too. The Anthropic->Anthropic passthrough is
// untouched: that upstream is the one that can execute them.
func stripServerTools(body []byte) ([]byte, int) {
	removed := 0
	strippedNames := make(map[string]struct{})
	// Reverse order: deleting by index shifts every later element.
	tools := gjson.GetBytes(body, "tools").Array()
	for i := len(tools) - 1; i >= 0; i-- {
		tool := tools[i]
		if !isNativeServerTool(tool.Get("type").String()) {
			continue
		}
		name := tool.Get("name").String()
		out, err := sjson.DeleteBytes(body, "tools."+strconv.Itoa(i))
		if err != nil {
			continue
		}
		body = out
		removed++
		if name != "" {
			strippedNames[name] = struct{}{}
		}
	}
	if removed == 0 {
		return body, 0
	}
	// Any forced choice now dangling on a stripped tool would 400 upstream;
	// a "tool" choice naming a surviving tool stays.
	if gjson.GetBytes(body, "tool_choice.type").String() == "tool" {
		choiceName := gjson.GetBytes(body, "tool_choice.name").String()
		if _, ok := strippedNames[choiceName]; ok {
			if out, err := sjson.DeleteBytes(body, "tool_choice"); err == nil {
				body = out
			}
		}
	}
	return body, removed
}
