package middleware

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"workweave/router/internal/auth"
	"workweave/router/internal/flags"
	"workweave/router/internal/observability"
	"workweave/router/internal/proxy"
	"workweave/router/internal/router"

	"github.com/gin-gonic/gin"
)

const (
	ctxKeyInstallation = "router_installation"
	ctxKeyAPIKey       = "router_api_key"
)

// RouterKeyHeader carries the Weave Router key when clients need to preserve Authorization / x-api-key for the upstream provider.
const RouterKeyHeader = "X-Weave-Router-Key"

// WithAuth validates the inbound request via a bearer rk_ token only. Used
// on data-plane routes (`/v1/*`). On failure, short-circuits 401.
func WithAuth(svc *auth.Service) gin.HandlerFunc {
	return withAPIKey(svc)
}

// withAPIKey is the bearer-only auth path behind WithAuth. Every downstream
// BYOK consumer (credential resolution, provider gating, usage bookkeeping)
// reads the single ctx key, so gating it here decides the whole path in one
// place.
func withAPIKey(svc *auth.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		token := extractToken(c)
		installation, apiKey, externalKeys, clusterModelLists, err := svc.VerifyAPIKey(c.Request.Context(), token)
		if err != nil {
			handleAuthError(c, err)
			return
		}
		c.Set(ctxKeyInstallation, installation)
		c.Set(ctxKeyAPIKey, apiKey)
		ctx := c.Request.Context()
		if apiKey != nil {
			ctx = context.WithValue(ctx, proxy.APIKeyIDContextKey{}, apiKey.ID)
		}
		if installation != nil {
			if installation.ExternalID != "" {
				ctx = context.WithValue(ctx, proxy.ExternalIDContextKey{}, installation.ExternalID)
			}
			if installation.ID != "" {
				ctx = context.WithValue(ctx, proxy.InstallationIDContextKey{}, installation.ID)
			}
			if len(installation.ExcludedModels) > 0 {
				ctx = context.WithValue(ctx, proxy.InstallationExcludedModelsContextKey{}, installation.ExcludedModels)
			}
			if len(installation.AllowedModels) > 0 {
				ctx = context.WithValue(ctx, proxy.InstallationAllowedModelsContextKey{}, installation.AllowedModels)
			}
			if len(installation.ExcludedProviders) > 0 {
				ctx = context.WithValue(ctx, proxy.InstallationExcludedProvidersContextKey{}, installation.ExcludedProviders)
			}
			if len(installation.PreferredModels) > 0 {
				ctx = context.WithValue(ctx, proxy.InstallationPreferredModelsContextKey{}, installation.PreferredModels)
			}
			if installation.RoutingQualityWeight != nil {
				// User-facing dial position flows in as QualityBias (per-cluster,
				// dispersion-aware), not the uniform Alpha. See router.Overrides.
				ctx = context.WithValue(ctx, proxy.InstallationRoutingKnobsContextKey{}, &router.Overrides{
					QualityBias: installation.RoutingQualityWeight,
				})
			}
			if installation.HideTerminalSurfaces {
				ctx = context.WithValue(ctx, proxy.InstallationHideTerminalSurfacesContextKey{}, true)
			}
			if installation.RoutingRolloutID != "" {
				ctx = context.WithValue(ctx, proxy.PolicyRolloutIDContextKey{}, installation.RoutingRolloutID)
			}
			if installation.PolicyShadowStrategy != "" {
				ctx = context.WithValue(ctx, proxy.PolicyShadowStrategyContextKey{}, installation.PolicyShadowStrategy)
			}
			if installation.PolicyDebugEnabled {
				ctx = context.WithValue(ctx, proxy.PolicyDebugEnabledContextKey{}, true)
			}
			if installation.PolicyRoutingIntent != "" {
				ctx = context.WithValue(ctx, proxy.PolicyRoutingIntentContextKey{}, installation.PolicyRoutingIntent)
			}
			if installation.AITrainingAllowed {
				ctx = context.WithValue(ctx, proxy.PolicyTrainingAllowedContextKey{}, true)
			}
			if installation.ContentCaptureMode != nil {
				ctx = context.WithValue(ctx, proxy.InstallationCaptureModeContextKey{},
					proxy.ParseCaptureMode(*installation.ContentCaptureMode))
			}
			// Per-organization behavioral flag overrides. Skipped entirely when
			// the deployment-wide escape hatch is set, so an env-var rollback
			// can't be defeated by a stored per-org row. WithOverrides is a
			// no-op for an empty set, which is the common case.
			if !svc.FlagOverridesDisabled() {
				ctx = flags.WithOverrides(ctx, installation.FlagOverrides)
			}
		}
		if externalKeys != nil {
			ctx = context.WithValue(ctx, proxy.ExternalAPIKeysContextKey{}, externalKeys)
		}
		if len(clusterModelLists) > 0 {
			overrides := make(map[string][]string, len(clusterModelLists))
			for _, list := range clusterModelLists {
				if len(list.Models) == 0 {
					continue
				}
				overrides[list.ClusterLabel] = list.Models
			}
			if len(overrides) > 0 {
				ctx = context.WithValue(ctx, proxy.ClusterModelListsContextKey{}, overrides)
			}
		}
		if installation != nil && installation.ID != "" {
			ctx = context.WithValue(ctx, proxy.InstallationIDContextKey{}, installation.ID)
		}
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}

// extractToken pulls the router token from RouterKeyHeader first, then falls back to Authorization: Bearer or x-api-key.
func extractToken(c *gin.Context) string {
	if t := strings.TrimSpace(c.GetHeader(RouterKeyHeader)); t != "" {
		return t
	}
	if t := extractBearer(c.GetHeader("Authorization")); t != "" {
		return t
	}
	return strings.TrimSpace(c.GetHeader("x-api-key"))
}

func extractBearer(header string) string {
	if header == "" {
		return ""
	}
	const prefix = "Bearer "
	if len(header) > len(prefix) && strings.EqualFold(header[:len(prefix)], prefix) {
		return strings.TrimSpace(header[len(prefix):])
	}
	return ""
}

func handleAuthError(c *gin.Context, err error) {
	logger := observability.FromGin(c)
	switch {
	case errors.Is(err, auth.ErrInvalidPrefix):
		logger.Debug("Auth rejected: invalid bearer prefix (expected rk_...)")
	case errors.Is(err, auth.ErrInvalidToken):
		logger.Debug("Auth rejected: bearer token did not match an active key")
	case errors.Is(err, auth.ErrWrongKeyScope):
		logger.Debug("Auth rejected: bearer key scope does not cover this surface")
	default:
		// Infra failure — not a bad key. 503 so clients retry instead of treating it as terminal.
		logger.Error("Auth check errored", "err", err)
		c.Header("Retry-After", "1")
		c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"error": "auth_unavailable"})
		return
	}
	c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid_key"})
}

// InstallationFrom retrieves the authed installation set by WithAuth. Returns nil for admin-cookie sessions and unauthed requests.
func InstallationFrom(c *gin.Context) *auth.Installation {
	v, ok := c.Get(ctxKeyInstallation)
	if !ok {
		return nil
	}
	installation, _ := v.(*auth.Installation)
	return installation
}

func APIKeyFrom(c *gin.Context) *auth.APIKey {
	v, ok := c.Get(ctxKeyAPIKey)
	if !ok {
		return nil
	}
	apiKey, _ := v.(*auth.APIKey)
	return apiKey
}
