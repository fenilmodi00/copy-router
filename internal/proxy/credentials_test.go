package proxy_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"workweave/router/internal/auth"
	"workweave/router/internal/providers"
	"workweave/router/internal/proxy"
)

func TestBuildCredentialsMap_NilOnEmptySlice(t *testing.T) {
	assert.Nil(t, proxy.BuildCredentialsMap(nil))
}

func TestBuildCredentialsMap_IndexesByProvider(t *testing.T) {
	keys := []*auth.ExternalAPIKey{
		{Provider: providers.ProviderAiand, Plaintext: []byte("sk-live")},
	}
	m := proxy.BuildCredentialsMap(keys)
	require.NotNil(t, m)
	got, ok := m[providers.ProviderAiand]
	require.True(t, ok)
	assert.Equal(t, []byte("sk-live"), got.APIKey)
	assert.Equal(t, "byok", got.Source)
}

func TestBuildCredentialsMap_DropsEmptyPlaintext(t *testing.T) {
	keys := []*auth.ExternalAPIKey{
		{Provider: providers.ProviderAiand, Plaintext: []byte("")},
	}
	assert.Nil(t, proxy.BuildCredentialsMap(keys))
}

func TestBuildCredentialsMap_NilWhenAllEmpty(t *testing.T) {
	keys := []*auth.ExternalAPIKey{
		{Provider: providers.ProviderAiand, Plaintext: nil},
		{Provider: "other", Plaintext: []byte("")},
	}
	assert.Nil(t, proxy.BuildCredentialsMap(keys))
}

func TestExtractClientCredentials_AiandBearer(t *testing.T) {
	headers := http.Header{"Authorization": []string{"Bearer sk-aiand-client-key"}}
	creds := proxy.ExtractClientCredentials(providers.ProviderAiand, headers)
	require.NotNil(t, creds)
	assert.Equal(t, []byte("sk-aiand-client-key"), creds.APIKey)
	assert.Equal(t, "client", creds.Source)
}

// The leak guard: router-issued rk_ bearers arrive on the same headers via
// WithAuth, and must never be forwarded upstream as provider credentials.
func TestExtractClientCredentials_RejectsRouterBearer(t *testing.T) {
	headers := http.Header{"Authorization": []string{"Bearer rk_live_routerkey_123456789"}}
	assert.Nil(t, proxy.ExtractClientCredentials(providers.ProviderAiand, headers))
}

func TestExtractClientCredentials_RejectsAnthropicShapedBearer(t *testing.T) {
	// One Bearer header must not be misidentified as creds for a provider it
	// doesn't belong to.
	headers := http.Header{"Authorization": []string{"Bearer sk-ant-api-real-key"}}
	assert.Nil(t, proxy.ExtractClientCredentials(providers.ProviderAiand, headers))
}

func TestExtractClientCredentials_MissingHeader(t *testing.T) {
	assert.Nil(t, proxy.ExtractClientCredentials(providers.ProviderAiand, http.Header{}))
}

func TestExtractClientCredentials_TrimsWhitespaceFromForwardedKey(t *testing.T) {
	headers := http.Header{"Authorization": []string{"Bearer   sk-aiand-padded   "}}
	creds := proxy.ExtractClientCredentials(providers.ProviderAiand, headers)
	require.NotNil(t, creds)
	assert.Equal(t, []byte("sk-aiand-padded"), creds.APIKey)
}
