package proxy_test

import (
	"context"
	"net/http"
	"testing"

	"workweave/router/internal/providers"
	"workweave/router/internal/proxy"
	"workweave/router/internal/router"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// listerClient is a providers.Client that also lists models, recording the
// credentials it saw so tests can assert they were placed in context.
type listerClient struct {
	models    []string
	seenCreds *proxy.Credentials
}

func (c *listerClient) Proxy(context.Context, router.Decision, providers.PreparedRequest, http.ResponseWriter, *http.Request) error {
	return nil
}
func (c *listerClient) Passthrough(context.Context, providers.PreparedRequest, http.ResponseWriter, *http.Request) error {
	return nil
}
func (c *listerClient) ListModels(ctx context.Context) ([]string, error) {
	c.seenCreds = proxy.CredentialsFromContext(ctx)
	return c.models, nil
}

// nonListerClient has no model-listing surface.
type nonListerClient struct{}

func (nonListerClient) Proxy(context.Context, router.Decision, providers.PreparedRequest, http.ResponseWriter, *http.Request) error {
	return nil
}
func (nonListerClient) Passthrough(context.Context, providers.PreparedRequest, http.ResponseWriter, *http.Request) error {
	return nil
}

func upstreamModelsService(providerMap map[string]providers.Client) *proxy.Service {
	return proxy.NewService(nil, providerMap, nil, false, nil, nil, false, providers.ProviderAiand, "deepseek-ai/deepseek-v4-flash", nil)
}

func TestListUpstreamModels_PassesCredentialsToLister(t *testing.T) {
	lister := &listerClient{models: []string{"deepseek-ai/deepseek-v4-flash", "moonshotai/kimi-k3"}}
	svc := upstreamModelsService(map[string]providers.Client{providers.ProviderAiand: lister})

	creds := &proxy.Credentials{APIKey: []byte("byok"), BaseURL: "https://api.aiand.com/v1"}
	models, err := svc.ListUpstreamModels(context.Background(), providers.ProviderAiand, creds)
	require.NoError(t, err)
	assert.Equal(t, []string{"deepseek-ai/deepseek-v4-flash", "moonshotai/kimi-k3"}, models)
	require.NotNil(t, lister.seenCreds, "the BYOK credentials must reach the adapter via context")
	assert.Equal(t, "byok", string(lister.seenCreds.APIKey))
}

func TestListUpstreamModels_UnsupportedProvider(t *testing.T) {
	svc := upstreamModelsService(map[string]providers.Client{providers.ProviderAiand: nonListerClient{}})

	_, err := svc.ListUpstreamModels(context.Background(), providers.ProviderAiand, nil)
	assert.ErrorIs(t, err, proxy.ErrModelListingUnsupported)
}

func TestListUpstreamModels_UnknownProvider(t *testing.T) {
	svc := upstreamModelsService(nil)

	_, err := svc.ListUpstreamModels(context.Background(), providers.ProviderAiand, nil)
	assert.ErrorIs(t, err, proxy.ErrProviderNotConfigured)
}
