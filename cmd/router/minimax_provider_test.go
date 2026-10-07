package main

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"weave-os/router/internal/providers"
)

func TestUpstreamIDsForProvider_OpenAI(t *testing.T) {
	ids := upstreamIDsForProvider(providers.ProviderOpenAI)

	assert.Equal(t, "gpt-5.6-luna", ids["gpt-5.6-luna-pro"])
	assert.Equal(t, "gpt-5.6-sol", ids["gpt-5.6-sol-pro"])
}
