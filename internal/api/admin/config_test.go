package admin_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"workweave/router/internal/api/admin"
	"workweave/router/internal/auth"
	"workweave/router/internal/providers"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// TestConfigHandler_EnvProviderKeys_ReportsAiandKey covers the single-provider
// contract: the config response lists "aiand" only when AIAND_API_KEY is set.
func TestConfigHandler_EnvProviderKeys_ReportsAiandKey(t *testing.T) {
	t.Setenv("AIAND_API_KEY", "dummy-key")

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.GET("/v1/config", func(c *gin.Context) {
		c.Set("router_installation", &auth.Installation{ID: "inst-1"})
	}, admin.ConfigHandler)

	req := httptest.NewRequest(http.MethodGet, "/v1/config", nil)
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var body struct {
		EnvProviderKeys []string `json:"env_provider_keys"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, []string{providers.ProviderAiand}, body.EnvProviderKeys)
}

func TestConfigHandler_EnvProviderKeys_EmptyWithoutEnvKey(t *testing.T) {
	t.Setenv("AIAND_API_KEY", "")

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.GET("/v1/config", func(c *gin.Context) {
		c.Set("router_installation", &auth.Installation{ID: "inst-1"})
	}, admin.ConfigHandler)

	req := httptest.NewRequest(http.MethodGet, "/v1/config", nil)
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var body struct {
		EnvProviderKeys []string `json:"env_provider_keys"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Empty(t, body.EnvProviderKeys)
}
