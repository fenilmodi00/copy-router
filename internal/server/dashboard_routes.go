// Package server wires the HTTP engine: middleware and route registration.
package server

import (
	"net/http"

	"workweave/router/internal/analytics"
	"workweave/router/internal/api/admin"
	"workweave/router/internal/auth"
	"workweave/router/internal/billing"
	"workweave/router/internal/proxy"
	"workweave/router/internal/router/policy"
	"workweave/router/internal/server/middleware"

	"github.com/gin-gonic/gin"
)

// Services bundles the dependencies that Register wires into the dashboard
// data plane plus the non-dashboard product surfaces (readiness, HMM roster,
// analytics). Wrapping the previously-individual params in a struct keeps the
// Register signature stable as the dashboard grows; field set mirrors the
// historical Register params minus engine and mode.
type Services struct {
	Auth             *auth.Service
	Proxy            *proxy.Service
	DeployedModels   admin.DeployedModelsSource
	HMMModels        admin.HMMRosterSource
	Billing          *billing.Service
	ReadinessChecker admin.HealthChecker
	HMMRosterSource  policy.RosterSource
	Analytics        *analytics.Service
	// AiandCatalogHandler, when non-nil, mounts GET /admin/v1/aiand/models.
	// nil means AIAND_API_KEY was absent at boot — fail-closed: no route is
	// registered so the dashboard hides the Models section instead of
	// erroring per request.
	AiandCatalogHandler gin.HandlerFunc
}

// routeSection splits dashboard rows into the two auth groups the selfhosted
// mode mounts separately (metrics = cookie-or-bearer, mgmt = admin cookie
// only). Selfserve mounts both sections under the single account-cookie group.
type routeSection int8

const (
	sectionMetrics routeSection = iota
	sectionMgmt
)

// routeNeed is a bitmask of optional-mounter conditions. A row with a given
// need mounts only when the corresponding Services field is non-nil.
type routeNeed uint8

const (
	needsDeployedModels routeNeed = 1 << iota
	needsAiandCatalog
)

// routeModes is a bitmask of deployment modes a row mounts in. Rows that
// are not yet wired into selfserve are gated to modeSelfHosted only; adding
// modeSelfServe to a row's modes is what extends it to the selfserve dashboard.
type routeModes uint8

const (
	modeSelfHosted routeModes = 1 << iota
	modeSelfServe
)

// dashboardRoute is one row in the shared dashboard data-plane table. Paths
// are relative to the /admin/v1 prefix; the mounter prefixes them.
type dashboardRoute struct {
	method  string
	path    string
	section routeSection
	needs   routeNeed
	modes   routeModes
	handler gin.HandlerFunc
}

// dashboardRoutes is the single source of truth for the /admin/v1/* data
// plane in both selfhosted and selfserve. New dashboard endpoints are added as
// a row here, not as a new route inside a mode block.
func dashboardRoutes(s Services) []dashboardRoute {
	return []dashboardRoute{
		// Metrics — read-only; dashboard cookie OR rk_ bearer (selfhosted),
		// account cookie (selfserve). Per-installation scoping inside handlers.
		{method: "GET", path: "/metrics/summary", section: sectionMetrics, modes: modeSelfHosted | modeSelfServe, handler: admin.MetricsSummaryHandler(s.Proxy)},
		{method: "GET", path: "/metrics/timeseries", section: sectionMetrics, modes: modeSelfHosted | modeSelfServe, handler: admin.MetricsTimeseriesHandler(s.Proxy)},
		{method: "GET", path: "/metrics/details", section: sectionMetrics, modes: modeSelfHosted | modeSelfServe, handler: admin.MetricsDetailsHandler(s.Proxy)},
		{method: "GET", path: "/metrics/model-breakdown", section: sectionMetrics, modes: modeSelfHosted | modeSelfServe, handler: admin.MetricsModelBreakdownHandler(s.Proxy)},

		// Live ai& catalog for the Models section; display source-of-truth
		// only (the routing catalog is untouched). Registered only when the
		// boot-time AIAND_API_KEY is set — fail-closed: absent key means no route.
		{method: "GET", path: "/aiand/models", section: sectionMetrics, needs: needsAiandCatalog, modes: modeSelfHosted | modeSelfServe, handler: s.AiandCatalogHandler},

		// API keys — installation-scoped via resolveInstallation.
		{method: "GET", path: "/keys", section: sectionMgmt, modes: modeSelfHosted | modeSelfServe, handler: admin.ListAPIKeysHandler(s.Auth)},
		{method: "POST", path: "/keys", section: sectionMgmt, modes: modeSelfHosted | modeSelfServe, handler: admin.IssueAPIKeyHandler(s.Auth)},
		{method: "POST", path: "/keys/:id/rotate", section: sectionMgmt, modes: modeSelfHosted | modeSelfServe, handler: admin.RotateAPIKeyHandler(s.Auth)},
		{method: "DELETE", path: "/keys/:id", section: sectionMgmt, modes: modeSelfHosted | modeSelfServe, handler: admin.DeleteAPIKeyHandler(s.Auth)},

		// Provider (BYOK) keys. List/upsert/delete plus aliases,
		// :id/models, and discover-models mount in both dashboard modes;
		// the aliases handler tolerates a nil DeployedModels (alias
		// validation skips), and discovery/list never needed it.
		{method: "GET", path: "/provider-keys", section: sectionMgmt, modes: modeSelfHosted | modeSelfServe, handler: admin.ListExternalKeysHandler(s.Auth)},
		{method: "POST", path: "/provider-keys", section: sectionMgmt, modes: modeSelfHosted | modeSelfServe, handler: admin.UpsertExternalKeyHandler(s.Auth, s.DeployedModels)},
		{method: "PUT", path: "/provider-keys/:id/model-aliases", section: sectionMgmt, modes: modeSelfHosted | modeSelfServe, handler: admin.UpdateExternalKeyAliasesHandler(s.Auth, s.DeployedModels)},
		{method: "GET", path: "/provider-keys/:id/models", section: sectionMgmt, modes: modeSelfHosted | modeSelfServe, handler: admin.ListUpstreamModelsHandler(s.Auth, s.Proxy)},
		{method: "POST", path: "/provider-keys/discover-models", section: sectionMgmt, modes: modeSelfHosted | modeSelfServe, handler: admin.DiscoverModelsHandler(s.Proxy)},
		{method: "DELETE", path: "/provider-keys/:id", section: sectionMgmt, modes: modeSelfHosted | modeSelfServe, handler: admin.DeleteExternalKeyHandler(s.Auth)},

		// Config + onboarding + routing preferences.
		{method: "GET", path: "/config", section: sectionMgmt, modes: modeSelfHosted | modeSelfServe, handler: admin.ConfigHandler},
		{method: "GET", path: "/onboarding", section: sectionMgmt, modes: modeSelfHosted | modeSelfServe, handler: admin.OnboardingHandler(s.Auth)},
		{method: "GET", path: "/routing-preferences", section: sectionMgmt, modes: modeSelfHosted | modeSelfServe, handler: admin.GetRoutingPreferencesHandler(s.Auth)},
		{method: "PUT", path: "/routing-preferences", section: sectionMgmt, modes: modeSelfHosted | modeSelfServe, handler: admin.UpdateRoutingPreferencesHandler(s.Auth)},

		// Content capture — installation-scoped; capture source is *proxy.Service.
		{method: "GET", path: "/content-capture", section: sectionMgmt, modes: modeSelfHosted | modeSelfServe, handler: admin.GetContentCaptureHandler(s.Auth, s.Proxy)},
		{method: "PUT", path: "/content-capture", section: sectionMgmt, modes: modeSelfHosted | modeSelfServe, handler: admin.UpdateContentCaptureHandler(s.Auth, s.Proxy)},

		// Excluded/allowed models + providers. The 6 rows need a non-nil
		// DeployedModels (the handlers use it, or its 3-arg form); they mount
		// only when one is wired. The override/routable source is *proxy.Service.
		{method: "GET", path: "/excluded-models", section: sectionMgmt, needs: needsDeployedModels, modes: modeSelfHosted | modeSelfServe, handler: admin.GetExcludedModelsHandler(s.Auth, s.DeployedModels, s.Proxy)},
		{method: "PUT", path: "/excluded-models", section: sectionMgmt, needs: needsDeployedModels, modes: modeSelfHosted | modeSelfServe, handler: admin.UpdateExcludedModelsHandler(s.Auth, s.DeployedModels, s.Proxy)},
		{method: "GET", path: "/allowed-models", section: sectionMgmt, needs: needsDeployedModels, modes: modeSelfHosted | modeSelfServe, handler: admin.GetAllowedModelsHandler(s.Auth, s.DeployedModels)},
		{method: "PUT", path: "/allowed-models", section: sectionMgmt, needs: needsDeployedModels, modes: modeSelfHosted | modeSelfServe, handler: admin.UpdateAllowedModelsHandler(s.Auth, s.DeployedModels, s.Proxy)},
		{method: "GET", path: "/excluded-providers", section: sectionMgmt, needs: needsDeployedModels, modes: modeSelfHosted | modeSelfServe, handler: admin.GetExcludedProvidersHandler(s.Auth, s.DeployedModels, s.Proxy)},
		{method: "PUT", path: "/excluded-providers", section: sectionMgmt, needs: needsDeployedModels, modes: modeSelfHosted | modeSelfServe, handler: admin.UpdateExcludedProvidersHandler(s.Auth, s.DeployedModels, s.Proxy)},
	}
}

// routeNeeds reports whether row r's needs are satisfied by s.
func routeNeeds(s Services, needs routeNeed) bool {
	if needs&needsDeployedModels != 0 && s.DeployedModels == nil {
		return false
	}
	if needs&needsAiandCatalog != 0 && s.AiandCatalogHandler == nil {
		return false
	}
	return true
}

// mountDashboardRoutes wires the shared dashboard data plane for selfhosted
// and selfserve. Managed mode is a no-op (its control plane lives elsewhere).
//
// byokRequiresOptIn stays computed in Register (it gates the product surface
// too) and is passed in; selfhosted uses WithAdminOrAuth/WithAdminOnly,
// selfserve uses WithAccountCookie. The login surfaces (operator password vs
// account cookie) are genuinely different and stay in Register, not here.
func mountDashboardRoutes(engine *gin.Engine, s Services, mode DeploymentMode, byokRequiresOptIn bool) {
	var modeBit routeModes
	switch mode {
	case DeploymentModeSelfHosted:
		modeBit = modeSelfHosted
	case DeploymentModeSelfServe:
		modeBit = modeSelfServe
	default:
		return
	}

	// Shared shell: root redirect + static dashboard at /ui.
	engine.GET("/", func(c *gin.Context) { c.Redirect(http.StatusFound, "/ui") })
	registerUIStatic(engine, "./assets/ui")

	routes := dashboardRoutes(s)

	switch mode {
	case DeploymentModeSelfHosted:
		metrics := engine.Group("/admin/v1", middleware.WithTimeout(adminTimeout), middleware.WithAdminOrAuth(s.Auth, byokRequiresOptIn))
		mgmt := engine.Group("/admin/v1", middleware.WithTimeout(adminTimeout), middleware.WithAdminOnly(s.Auth))
		for _, r := range routes {
			if r.modes&modeBit == 0 {
				continue
			}
			if !routeNeeds(s, r.needs) {
				continue
			}
			if r.section == sectionMetrics {
				metrics.Handle(r.method, r.path, r.handler)
			} else {
				mgmt.Handle(r.method, r.path, r.handler)
			}
		}
	case DeploymentModeSelfServe:
		accountAuthed := engine.Group("/admin/v1", middleware.WithTimeout(adminTimeout), middleware.WithAccountCookie(s.Auth))
		for _, r := range routes {
			if r.modes&modeBit == 0 {
				continue
			}
			if !routeNeeds(s, r.needs) {
				continue
			}
			accountAuthed.Handle(r.method, r.path, r.handler)
		}
	}
}
