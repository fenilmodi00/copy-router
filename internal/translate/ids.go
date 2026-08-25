package translate

import (
	"crypto/rand"
	"encoding/hex"
	"strings"
)

func generateChatCmplID() string {
	return "chatcmpl-" + randomHex(8)
}

func generateToolCallID() string {
	return "call_" + randomHex(4)
}

func generateAnthropicMsgID() string {
	return "msg_" + randomHex(8)
}

func randomHex(n int) string {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return strings.Repeat("0", n*2)
	}
	return hex.EncodeToString(buf)
}
