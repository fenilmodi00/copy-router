package proxy

import (
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// NormalizeOpenAIPlaygroundModel rewrites model:null or an empty model field to
// "auto" so dashboard clients can omit an explicit model without forcing.
func NormalizeOpenAIPlaygroundModel(body []byte) []byte {
	model := gjson.GetBytes(body, "model")
	if model.Type != gjson.Null && !(model.Type == gjson.String && model.String() == "") {
		return body
	}
	patched, err := sjson.SetBytes(body, "model", "auto")
	if err != nil {
		return body
	}
	return patched
}

// InjectOpenAIPlaygroundSession stamps a stable playground session id into an
// OpenAI Chat Completions body so DeriveSessionKey pins consistently across
// dashboard refreshes for the same browser store.
func InjectOpenAIPlaygroundSession(body []byte, sessionID string) []byte {
	if sessionID == "" {
		return body
	}
	patched, err := sjson.SetBytes(body, "metadata.user_id", sessionID)
	if err != nil {
		return body
	}
	return patched
}
