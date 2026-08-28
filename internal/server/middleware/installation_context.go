package middleware

import (
	"context"

	"workweave/router/internal/auth"
	"workweave/router/internal/flags"
	"workweave/router/internal/proxy"
	"workweave/router/internal/router"
)

// BindInstallationContext stashes installation-scoped proxy context values the
// same way bearer auth does, without an rk_ API key. Dashboard and playground
// handlers call this after resolving the caller's installation.
func BindInstallationContext(
	ctx context.Context,
	svc *auth.Service,
	installation *auth.Installation,
	externalKeys []*auth.ExternalAPIKey,
	byokAllowed bool,
) context.Context {
	if installation == nil {
		return ctx
	}
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
		ctx = context.WithValue(ctx, proxy.InstallationRoutingKnobsContextKey{}, &router.Overrides{
			QualityBias: installation.RoutingQualityWeight,
		})
	}
	if installation.UsageBypassEnabled {
		ctx = context.WithValue(ctx, proxy.InstallationUsageBypassContextKey{}, proxy.UsageBypassConfig{
			Enabled:   true,
			Threshold: installation.UsageBypassThreshold,
		})
	}
	if installation.SubscriptionRoutingDisabled {
		ctx = context.WithValue(ctx, proxy.InstallationSubscriptionRoutingDisabledContextKey{}, true)
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
	if svc != nil && !svc.FlagOverridesDisabled() {
		ctx = flags.WithOverrides(ctx, installation.FlagOverrides)
	}
	if byokAllowed && len(externalKeys) > 0 {
		ctx = context.WithValue(ctx, proxy.ExternalAPIKeysContextKey{}, externalKeys)
	}
	return ctx
}
