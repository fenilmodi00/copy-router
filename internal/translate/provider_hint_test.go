package translate_test

import (
	"encoding/json"
	"testing"

	"workweave/router/internal/translate"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPrepareOpenAI_OpenAISource_EmptyToolsNoOverrides(t *testing.T) {
	src := []byte(`{"model":"x","messages":[{"role":"user","content":"hi"}],"tools":[],"max_tokens":256}`)
	env, err := translate.ParseOpenAI(src)
	require.NoError(t, err)

	out, err := env.PrepareOpenAI(nil, translate.EmitOptions{
		TargetModel: "deepseek/deepseek-chat",
	})
	require.NoError(t, err)

	var doc map[string]any
	require.NoError(t, json.Unmarshal(out.Body, &doc))

	_, hasTemp := doc["temperature"]
	assert.False(t, hasTemp, "empty tools must not trigger temperature override")
}
