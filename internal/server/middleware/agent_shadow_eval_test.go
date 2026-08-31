package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"workweave/router/internal/auth"
	"workweave/router/internal/proxy"
	"workweave/router/internal/server/middleware"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func runAgentShadowMiddleware(t *testing.T, installation *auth.Installation, headers http.Header) (int, bool) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(func(c *gin.Context) {
		if installation != nil {
			c.Set("router_installation", installation)
		}
		c.Next()
	})
	engine.Use(middleware.WithAgentShadowEvaluation())
	var observed bool
	engine.POST("/v1/messages", func(c *gin.Context) {
		_, observed = proxy.AgentShadowEvalFromContext(c.Request.Context())
		c.Status(http.StatusOK)
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	req.Header = headers
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	return rec.Code, observed
}

func TestAgentShadowEvaluation_RequiresAuthorizedCompleteTriplet(t *testing.T) {
	headers := http.Header{}
	headers.Set(proxy.AgentShadowModelHeader, "claude-opus-4-8")
	headers.Set(proxy.AgentShadowRolloutHeader, "pilot-1")
	headers.Set(proxy.AgentShadowStateHeader, "state-1")

	status, observed := runAgentShadowMiddleware(t, &auth.Installation{PolicyHeaderOverridesEnabled: true}, headers)
	require.Equal(t, http.StatusOK, status)
	assert.True(t, observed)

	status, observed = runAgentShadowMiddleware(t, &auth.Installation{}, headers)
	assert.Equal(t, http.StatusForbidden, status)
	assert.False(t, observed)

	headers.Del(proxy.AgentShadowStateHeader)
	status, observed = runAgentShadowMiddleware(t, &auth.Installation{PolicyHeaderOverridesEnabled: true}, headers)
	assert.Equal(t, http.StatusBadRequest, status)
	assert.False(t, observed)
}
