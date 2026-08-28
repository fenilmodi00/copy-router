package admin

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"

	"workweave/router/internal/auth"
	"workweave/router/internal/observability"
	"workweave/router/internal/proxy"
	"workweave/router/internal/server/middleware"
)

const playgroundRoutingMarkerHeader = "X-Weave-Routing-Marker"

// bindPlaygroundProxyContext resolves the caller's installation and stashes the
// proxy context values (installation id, BYOK keys, playground api key) the
// dispatch path expects. Returns false when resolveInstallation already wrote an
// error response.
func bindPlaygroundProxyContext(c *gin.Context, authSvc *auth.Service) (context.Context, *auth.Installation, bool) {
	installation, ok := resolveInstallation(c, authSvc)
	if !ok {
		return c.Request.Context(), nil, false
	}
	externalKeys := authSvc.ResolvedExternalAPIKeysForInstallation(c.Request.Context(), installation.ID)
	ctx := middleware.BindInstallationContext(c.Request.Context(), authSvc, installation, externalKeys, true)
	apiKeyID, err := authSvc.ResolvePlaygroundAPIKeyID(ctx, installation.ID)
	if err != nil {
		observability.FromGin(c).Error("Failed to resolve playground API key", "err", err, "installation_id", installation.ID)
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "Failed to resolve playground identity."})
		return ctx, nil, false
	}
	if apiKeyID != "" {
		ctx = context.WithValue(ctx, proxy.APIKeyIDContextKey{}, apiKeyID)
	}
	ctx = proxy.WithRespondRoutingMetadata(ctx)
	return ctx, installation, true
}
