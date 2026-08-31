package admin

import (
	"net/http"

	"workweave/router/internal/config"
	"workweave/router/internal/providers"

	"github.com/gin-gonic/gin"
)

type configResponse struct {
	ClusterVersion       string `json:"cluster_version"`
	EmbedOnlyUserMsg     bool   `json:"embed_only_user_message"`
	StickyDecisionTTL    string `json:"sticky_decision_ttl_ms"`
	OtelEnabled          bool   `json:"otel_enabled"`
	SemanticCacheEnabled bool   `json:"semantic_cache_enabled"`
	// EnvProviderKeys lists provider names whose upstream API key is set
	// via env var on the deployment. The dashboard renders these as
	// read-only entries — they aren't stored in Postgres and can only be
	// unset by editing the deployment env + restarting.
	EnvProviderKeys []string `json:"env_provider_keys"`
}

func ConfigHandler(c *gin.Context) {
	var envKeyed []string
	if config.GetOr("AIAND_API_KEY", "") != "" {
		envKeyed = []string{providers.ProviderAiand}
	}
	c.JSON(http.StatusOK, configResponse{
		ClusterVersion:       config.GetOr("ROUTER_CLUSTER_VERSION", "artifacts/latest"),
		EmbedOnlyUserMsg:     config.GetOr("ROUTER_EMBED_ONLY_USER_MESSAGE", "true") == "true",
		StickyDecisionTTL:    config.GetOr("ROUTER_STICKY_DECISION_TTL_MS", "0"),
		OtelEnabled:          config.GetOr("OTEL_EXPORTER_OTLP_ENDPOINT", "") != "",
		SemanticCacheEnabled: config.GetOr("ROUTER_SEMANTIC_CACHE_ENABLED", "true") == "true",
		EnvProviderKeys:      envKeyed,
	})
}
