package admin_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"weave-os/router/internal/api/admin"
	"weave-os/router/internal/providers"
	"weave-os/router/internal/router/cluster"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeDeployedModels struct {
	entries []cluster.DeployedEntry
}

func (f fakeDeployedModels) DefaultDeployedModels() []cluster.DeployedEntry { return f.entries }

func TestCatalogModelsHandler_SortsByProviderThenModel(t *testing.T) {
	gin.SetMode(gin.TestMode)

	src := fakeDeployedModels{entries: []cluster.DeployedEntry{
		{Model: "qwen/qwen3.8-27b", Provider: providers.ProviderAIAND},
		{Model: "zai-org/glm-5.3", Provider: providers.ProviderAIAND},
		{Model: "deepseek-ai/deepseek-v4-flash", Provider: providers.ProviderAIAND},
		{Model: "moonshotai/kimi-k3", Provider: providers.ProviderAIAND},
	}}

	engine := gin.New()
	engine.GET("/v1/router/models", admin.CatalogModelsHandler(src, nil))

	req := httptest.NewRequest(http.MethodGet, "/v1/router/models", nil)
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var got admin.CatalogModelsResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))

	require.Len(t, got.Models, 4)
	assert.Equal(t, providers.ProviderAIAND, got.Models[0].Provider)
	assert.Equal(t, "deepseek-ai/deepseek-v4-flash", got.Models[0].Model)
	assert.Equal(t, "moonshotai/kimi-k3", got.Models[1].Model)
	assert.Equal(t, "qwen/qwen3.8-27b", got.Models[2].Model)
	assert.Equal(t, "zai-org/glm-5.3", got.Models[3].Model)
}

func TestCatalogModelsHandler_EmptyListReturnsEmptyArray(t *testing.T) {
	gin.SetMode(gin.TestMode)

	engine := gin.New()
	engine.GET("/v1/router/models", admin.CatalogModelsHandler(fakeDeployedModels{}, nil))

	req := httptest.NewRequest(http.MethodGet, "/v1/router/models", nil)
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	// Empty slice must round-trip as [], not null — the Weave control plane
	// distinguishes "no models" from "missing field".
	assert.JSONEq(t, `{"models":[]}`, rec.Body.String())
}

type fakeHMMRoster struct {
	entries []cluster.DeployedEntry
	err     error
	calls   int
}

func (f *fakeHMMRoster) HMMDeployedModels(context.Context) ([]cluster.DeployedEntry, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	return f.entries, nil
}

func TestCatalogModelsHandler_HMMStrategyReturnsRosterNotCluster(t *testing.T) {
	gin.SetMode(gin.TestMode)

	clusterSrc := fakeDeployedModels{entries: []cluster.DeployedEntry{
		{Model: "zai-org/glm-5.3", Provider: providers.ProviderAIAND},
	}}
	hmmSrc := &fakeHMMRoster{entries: []cluster.DeployedEntry{
		{Model: "moonshotai/kimi-k3", Provider: providers.ProviderAIAND},
		{Model: "deepseek-ai/deepseek-v4-pro", Provider: providers.ProviderAIAND},
	}}

	engine := gin.New()
	engine.GET("/v1/router/models", admin.CatalogModelsHandler(clusterSrc, hmmSrc))

	req := httptest.NewRequest(http.MethodGet, "/v1/router/models?strategy=hmm", nil)
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, 1, hmmSrc.calls)

	var got admin.CatalogModelsResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	require.Len(t, got.Models, 2)
	// Sorted model-then-model within the single aiand provider; crucially the
	// cluster's zai-org/glm-5.3 does NOT appear.
	assert.Equal(t, "deepseek-ai/deepseek-v4-pro", got.Models[0].Model)
	assert.Equal(t, "moonshotai/kimi-k3", got.Models[1].Model)
}

func TestCatalogModelsHandler_HMMStrategyFallsBackToClusterWhenNoSource(t *testing.T) {
	gin.SetMode(gin.TestMode)

	clusterSrc := fakeDeployedModels{entries: []cluster.DeployedEntry{
		{Model: "zai-org/glm-5.3", Provider: providers.ProviderAIAND},
	}}

	engine := gin.New()
	engine.GET("/v1/router/models", admin.CatalogModelsHandler(clusterSrc, nil))

	req := httptest.NewRequest(http.MethodGet, "/v1/router/models?strategy=hmm", nil)
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var got admin.CatalogModelsResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	require.Len(t, got.Models, 1)
	assert.Equal(t, "zai-org/glm-5.3", got.Models[0].Model)
}

func TestCatalogModelsHandler_HMMRosterErrorReturns503(t *testing.T) {
	gin.SetMode(gin.TestMode)

	hmmSrc := &fakeHMMRoster{err: errors.New("sidecar unavailable")}

	engine := gin.New()
	engine.GET("/v1/router/models", admin.CatalogModelsHandler(fakeDeployedModels{}, hmmSrc))

	req := httptest.NewRequest(http.MethodGet, "/v1/router/models?strategy=hmm_embedding", nil)
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)

	require.Equal(t, http.StatusServiceUnavailable, rec.Code)
}

func TestCatalogModelsHandler_ScopeCatalogReturnsFullCatalog(t *testing.T) {
	gin.SetMode(gin.TestMode)

	clusterSrc := fakeDeployedModels{entries: []cluster.DeployedEntry{
		{Model: "zai-org/glm-5.3", Provider: providers.ProviderAIAND},
	}}
	hmmSrc := &fakeHMMRoster{entries: []cluster.DeployedEntry{
		{Model: "moonshotai/kimi-k3", Provider: providers.ProviderAIAND},
	}}

	engine := gin.New()
	engine.GET("/v1/router/models", admin.CatalogModelsHandler(clusterSrc, hmmSrc))

	req := httptest.NewRequest(http.MethodGet, "/v1/router/models?scope=catalog&strategy=hmm", nil)
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, 0, hmmSrc.calls, "scope=catalog must not consult the HMM roster")

	var got admin.CatalogModelsResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	require.Greater(t, len(got.Models), 1, "full catalog is larger than the 1-entry cluster fixture")

	seen := map[string]struct{}{}
	for _, m := range got.Models {
		require.NotEmpty(t, m.Model)
		require.NotEmpty(t, m.Provider)
		_, dup := seen[m.Model]
		require.False(t, dup, "duplicate catalog id %s", m.Model)
		seen[m.Model] = struct{}{}
	}
	_, hasRoster := seen["zai-org/glm-5.3"]
	require.True(t, hasRoster, "catalog must include a roster Models row")
}
