package main

import (
	"log/slog"
	"testing"

	"workweave/router/internal/server"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBuildAiandCatalogHandler pins the wiring contract of the dashboard's
// live ai& model-catalog route. Regression for a discarded-return bug: the
// composition root used to call admin.AiandCatalogHandler without assigning
// its return value, so aiandCatalogHandler stayed nil and GET /admin/v1/aiand/models
// never mounted — the Models page 404'd. The helper now returns the handler,
// and these assertions make a nil (route never mounting) a test failure.
func TestBuildAiandCatalogHandler(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(discardWriter{}, nil))

	t.Run("selfserve_without_deployment_key_mounts", func(t *testing.T) {
		h := buildAiandCatalogHandler("", "https://aiand.example", server.DeploymentModeSelfServe, logger)
		require.NotNil(t, h, "selfserve must mount the catalog route even without AIAND_API_KEY (per-user BYOK)")
	})

	t.Run("selfhosted_without_deployment_key_skips", func(t *testing.T) {
		h := buildAiandCatalogHandler("", "https://aiand.example", server.DeploymentModeSelfHosted, logger)
		assert.Nil(t, h, "selfhosted without AIAND_API_KEY must not mount the catalog route")
	})

	t.Run("selfhosted_with_deployment_key_mounts", func(t *testing.T) {
		h := buildAiandCatalogHandler("sk-test", "https://aiand.example", server.DeploymentModeSelfHosted, logger)
		require.NotNil(t, h, "selfhosted with AIAND_API_KEY must mount the catalog route")
	})

	t.Run("managed_without_deployment_key_skips", func(t *testing.T) {
		h := buildAiandCatalogHandler("", "https://aiand.example", server.DeploymentModeManaged, logger)
		assert.Nil(t, h, "managed without AIAND_API_KEY must not mount the catalog route")
	})
}

// discardWriter swallows log output so tests stay quiet.
type discardWriter struct{}

func (discardWriter) Write(p []byte) (int, error) { return len(p), nil }
