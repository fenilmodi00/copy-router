// Package server wires the HTTP engine: middleware and route registration.
package server

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	account "workweave/router/internal/api/account"
	"workweave/router/internal/api/admin"
	analyticsapi "workweave/router/internal/api/analytics"
	anthropicapi "workweave/router/internal/api/anthropic"
	openaiapi "workweave/router/internal/api/openai"
	"workweave/router/internal/billing"
	"workweave/router/internal/policyclient"
	"workweave/router/internal/router"
	"workweave/router/internal/server/middleware"

	"github.com/gin-gonic/gin"
)

const (
	healthTimeout    = 1 * time.Second
	readinessTimeout = 2 * time.Second
	// Cold-cache key probe costs 3 sequential Postgres round trips; 1s made that a credential rejection.
	validateTimeout = 5 * time.Second

	messagesTimeout       = 600 * time.Second
	chatCompletionTimeout = 600 * time.Second
	passthroughTimeout    = 10 * time.Second
	routeTimeout          = 5 * time.Second
	adminTimeout          = 10 * time.Second
	// catalogModelsTimeout bounds GET /v1/router/models; must exceed the HMM
	// sidecar client budget (policyclient.DefaultTimeout) or a cold cache 503s.
	catalogModelsTimeout = policyclient.DefaultTimeout * 2
	// analyticsTimeout bounds an export page. Keyset scans on a high-volume
	// telemetry table warrant a batch-job budget, not an interactive one.
	analyticsTimeout = 60 * time.Second
)

// DeploymentMode gates whether the self-hoster admin dashboard and its
// /admin/v1/* API are mounted. Managed (SaaS) deployments skip it since
// keys, BYOK secrets, and config are owned by the Weave control plane.
type DeploymentMode string

const (
	// DeploymentModeSelfHosted mounts the dashboard and /admin/v1/* API. Default when ROUTER_DEPLOYMENT_MODE is unset.
	DeploymentModeSelfHosted DeploymentMode = "selfhosted"
	// DeploymentModeManaged skips the dashboard and admin API entirely so misconfig can't expose a redundant control plane.
	DeploymentModeManaged DeploymentMode = "managed"
	// DeploymentModeSelfServe mounts the dashboard driven by self-service
	// (aiand-key) login instead of the operator password. The dashboard data
	// plane is a separate /admin/v1 surface scoped to the logged-in account's
	// installation; the operator admin API is NOT mounted.
	DeploymentModeSelfServe DeploymentMode = "selfserve"
)

// Register wires routes onto the engine. In managed mode the dashboard +
// /admin/v1/* routes are not registered at all.
//
// DeployedModels may be nil in tests; required in selfhosted prod so the
// dashboard can render the universe of routable models.
//
// HMMModels is optional; nil when no HMM sidecar is wired — falls back to the
// cluster registry.
//
// Billing is set only in managed mode when credit-billing is enabled; it
// gates every inference route on prepaid balance via WithBalanceCheck. nil
// leaves inference routes open (BYOK/platform key still controls upstream auth).
//
// ReadinessChecker gates /readyz only; /health remains process liveness.
//
// HMMRosterSource, when non-nil, mounts GET /v1/router/hmm-roster for the
// control plane's cluster allowlist UI.
//
// Analytics, when non-nil, mounts the /v1/analytics/* export surface;
// nil leaves it unmounted (tests, deployments without telemetry storage).
//
// AiandCatalogHandler, when non-nil, mounts the live ai& model catalog
// (GET /admin/v1/aiand/models) inside the dashboard metrics group. nil means
// AIAND_API_KEY was absent at boot — fail-closed: no route is registered so the
// dashboard hides the Models section instead of erroring per request.
func Register(engine *gin.Engine, s Services, mode DeploymentMode) {
	// Managed mode: BYOK is opt-in per installation (see WithAuth).
	byokRequiresOptIn := mode == DeploymentModeManaged

	engine.GET("/health", middleware.WithTimeout(healthTimeout), admin.HealthHandler)
	engine.GET("/readyz", middleware.WithTimeout(readinessTimeout), admin.ReadinessHandler(s.ReadinessChecker))

	// /v1/version reports the binary's git commit + build time (via -ldflags),
	// used by the README's managed-deployment badge. Public build metadata, unauthed like /health.
	engine.GET("/v1/version", middleware.WithTimeout(healthTimeout), admin.VersionHandler)
	var registeredStrategies []router.Strategy
	if s.Proxy != nil {
		registeredStrategies = s.Proxy.RegisteredStrategies()
	}
	defaultStrategy := router.Strategy(strings.ToLower(strings.TrimSpace(os.Getenv("ROUTER_DEFAULT_STRATEGY"))))
	if defaultStrategy == "" {
		defaultStrategy = router.StrategyCluster
	}
	defaultStrategy = middleware.NormalizeRouterStrategyDefault(defaultStrategy, registeredStrategies...)
	engine.GET(
		"/v1/router/policies",
		middleware.WithTimeout(healthTimeout),
		admin.PolicyCatalogHandler(s.Proxy, defaultStrategy),
	)

	// /v1/router/models lets the Weave control plane validate per-org exclusion
	// submissions against the live deployed-models universe instead of
	// hand-copying it per gitlink bump. Unauthed: read-only, and the list is
	// already public on the RouterArena leaderboard.
	if s.DeployedModels != nil {
		engine.GET("/v1/router/models", middleware.WithTimeout(catalogModelsTimeout), admin.CatalogModelsHandler(s.DeployedModels, s.HMMModels))

		// Projects the quality-vs-price dial's model mix across dial positions
		// for the dashboard's distribution preview. Same unauthed rationale as
		// /v1/router/models; the assertion skips sources that can't project one.
		if dist, ok := s.DeployedModels.(admin.RoutingDistributionSource); ok {
			engine.GET("/v1/router/routing-distribution", middleware.WithTimeout(healthTimeout), admin.RoutingDistributionHandler(dist))
		}
	}

	// /v1/router/hmm-roster: frozen per-cluster arm roster mapped to catalog IDs.
	// Unauthed — read-only and non-sensitive, same rationale as /v1/router/models.
	if s.HMMRosterSource != nil {
		engine.GET("/v1/router/hmm-roster", middleware.WithTimeout(readinessTimeout), admin.HMMRosterHandler(s.HMMRosterSource))
	}

	// /internal/v1/*: control-plane-to-router calls, authed by a shared secret
	// and mounted only when one is configured. This is not a second admin API —
	// it carries only work the control plane cannot do itself because the
	// credential is minted here per request (key-pair, workload identity).
	if internalToken := strings.TrimSpace(os.Getenv("ROUTER_INTERNAL_SERVICE_TOKEN")); internalToken != "" {
		internalGroup := engine.Group("/internal/v1", middleware.WithTimeout(adminTimeout), middleware.WithInternalServiceAuth(internalToken))
		internalGroup.POST("/provider-keys/models", admin.InternalListUpstreamModelsHandler(s.Auth, s.Proxy))
	}

	// /validate is a token-validity probe used by clients (not the dashboard), so it stays mounted in both modes.
	adminAuthed := engine.Group("", middleware.WithTimeout(validateTimeout), middleware.WithAuth(s.Auth, byokRequiresOptIn))
	adminAuthed.GET("/validate", admin.ValidateHandler)

	if mode == DeploymentModeSelfHosted {
		// Public — mounting inside WithAuth would be a chicken-and-egg
		// deadlock for users who don't yet have a cookie.
		authPublic := engine.Group("/admin/v1/auth", middleware.WithTimeout(adminTimeout))
		authPublic.POST("/login", admin.LoginHandler(s.Auth))
		authPublic.POST("/logout", admin.LogoutHandler())
		authPublic.GET("/me", admin.MeHandler(s.Auth))
	}

	if mode == DeploymentModeSelfServe {
		// Public — login must be reachable without a session cookie.
		accountPublic := engine.Group("/account/v1", middleware.WithTimeout(adminTimeout))
		accountPublic.POST("/login", account.LoginHandler(s.Auth))
		accountPublic.POST("/logout", account.LogoutHandler())
		accountPublic.GET("/me", account.MeHandler(s.Auth))
	}

	// Dashboard data plane (metrics, keys, provider-keys, config, excluded-models,
	// content-capture): single source of truth in dashboard_routes.go, mounted by
	// the helper for both selfhosted and selfserve. Managed mode is a no-op.
	// Login surfaces above are genuinely mode-specific and stay here.
	mountDashboardRoutes(engine, s, mode, byokRequiresOptIn)

	messagesMiddleware := []gin.HandlerFunc{
		middleware.WithTimingEntry(),
		middleware.WithTimeout(messagesTimeout),
		middleware.WithAuth(s.Auth, byokRequiresOptIn),
		middleware.WithAgentShadowEvaluation(),
	}
	if s.Billing != nil {
		messagesMiddleware = append(messagesMiddleware, middleware.WithBalanceCheck(s.Billing, billing.MinBalanceMicros), middleware.WithAPIKeySpendCap(s.Billing), middleware.WithOrgMonthlySpendCap(s.Billing))
	}
	messagesMiddleware = append(messagesMiddleware,
		middleware.WithEmbedOnlyUserMessageOverride(),
		middleware.WithClusterVersionOverride(),
		middleware.WithRouterStrategyDefault(defaultStrategy, registeredStrategies...),
		middleware.WithPolicyDebugOverride(),
		middleware.WithRoutingKnobsOverride(),
		middleware.WithForceEffortOverride(),
	)
	messagesGroup := engine.Group("", messagesMiddleware...)
	messagesGroup.POST("/v1/messages", anthropicapi.MessagesHandler(s.Proxy, s.Auth))

	chatCompletionMiddleware := []gin.HandlerFunc{
		middleware.WithTimingEntry(),
		middleware.WithTimeout(chatCompletionTimeout),
		middleware.WithAuth(s.Auth, byokRequiresOptIn),
	}
	if s.Billing != nil {
		chatCompletionMiddleware = append(chatCompletionMiddleware, middleware.WithBalanceCheck(s.Billing, billing.MinBalanceMicros), middleware.WithAPIKeySpendCap(s.Billing), middleware.WithOrgMonthlySpendCap(s.Billing))
	}
	chatCompletionMiddleware = append(chatCompletionMiddleware,
		middleware.WithEmbedOnlyUserMessageOverride(),
		middleware.WithClusterVersionOverride(),
		middleware.WithRouterStrategyDefault(defaultStrategy, registeredStrategies...),
		middleware.WithPolicyDebugOverride(),
		middleware.WithRoutingKnobsOverride(),
		middleware.WithForceEffortOverride(),
	)
	chatCompletionGroup := engine.Group("", chatCompletionMiddleware...)
	chatCompletionGroup.POST("/v1/chat/completions", openaiapi.ChatCompletionHandler(s.Proxy, s.Auth))
	// Responses surface required by Codex CLI after wire_api="chat" was retired;
	// translated internally to chat completions so the turn loop is reused.
	chatCompletionGroup.POST("/v1/responses", openaiapi.ResponsesHandler(s.Proxy, s.Auth))

	// Passthrough endpoints cost no upstream tokens, so they stay open even
	// with billing enabled — count_tokens is the SDK's pre-flight call before
	// /v1/messages, and gating it would break client negotiation.
	passthroughGroup := engine.Group("",
		middleware.WithTimeout(passthroughTimeout),
		middleware.WithAuth(s.Auth, byokRequiresOptIn),
	)
	passthroughGroup.POST("/v1/messages/count_tokens", anthropicapi.PassthroughHandler(s.Proxy))
	passthroughGroup.GET("/v1/models", openaiapi.ModelsHandler(anthropicapi.PassthroughHandler(s.Proxy)))
	passthroughGroup.GET("/v1/models/:model", anthropicapi.PassthroughHandler(s.Proxy))
	// Rides the passthrough group (cheap, no billing middleware) — read-only, no routing side-effects.
	passthroughGroup.GET("/v1/display-settings", admin.DisplaySettingsHandler)

	routeMiddleware := []gin.HandlerFunc{
		middleware.WithTimeout(routeTimeout),
		middleware.WithAuth(s.Auth, byokRequiresOptIn),
	}
	if s.Billing != nil {
		routeMiddleware = append(routeMiddleware, middleware.WithBalanceCheck(s.Billing, billing.MinBalanceMicros), middleware.WithAPIKeySpendCap(s.Billing), middleware.WithOrgMonthlySpendCap(s.Billing))
	}
	routeMiddleware = append(routeMiddleware,
		middleware.WithEmbedOnlyUserMessageOverride(),
		middleware.WithClusterVersionOverride(),
		middleware.WithRouterStrategyDefault(defaultStrategy, registeredStrategies...),
		middleware.WithPolicyDebugOverride(),
		middleware.WithRoutingKnobsOverride(),
		middleware.WithForceEffortOverride(),
	)
	routeGroup := engine.Group("", routeMiddleware...)
	routeGroup.POST("/v1/route", anthropicapi.RouteHandler(s.Proxy))

	previewGroup := engine.Group("",
		middleware.WithTimingEntry(),
		middleware.WithTimeout(routeTimeout),
		middleware.WithAuth(s.Auth, byokRequiresOptIn),
		middleware.WithEmbedOnlyUserMessageOverride(),
		middleware.WithRouterStrategyDefault(defaultStrategy, registeredStrategies...),
		middleware.WithPolicyDebugOverride(),
		middleware.WithRoutingKnobsOverride(),
	)
	previewGroup.POST("/v1/route/preview", anthropicapi.PreviewRouteHandler(s.Proxy))

	// Read-only routing-decision export. Product surface, so it mounts in both
	// modes; ra_ keys only, no spend path.
	if s.Analytics != nil {
		analyticsGroup := engine.Group("/v1/analytics",
			middleware.WithTimeout(analyticsTimeout),
			middleware.WithAnalyticsKey(s.Auth),
			middleware.WithAnalyticsRateLimit(middleware.AnalyticsRequestsPerMinute),
		)
		analyticsGroup.GET("/routing-decisions", analyticsapi.RoutingDecisionsHandler(s.Analytics))
		analyticsGroup.GET("/models", analyticsapi.ModelsHandler())
		analyticsGroup.GET("/schema", analyticsapi.SchemaHandler())
	}
}

// registerUIStatic mounts the exported Next.js dashboard at /ui with
// clean-URL semantics (no trailing slash, no .html extension).
//
// Next's static export (trailingSlash:false) writes `settings.html`, not
// `settings/index.html`, so plain gin.Static/http.FileServer would 404 or
// redirect wrong on `/ui/settings`. Resolution order for `/ui/<path>`:
//  1. Trailing slash -> redirect to slashless form (308).
//  2. Empty or `index` -> serve index.html.
//  3. `<path>` exists as a file -> serve it.
//  4. `<path>.html` exists -> serve that.
//  5. Otherwise 404.
//
// Resolved paths are clamped under `root` via filepath.Clean against `..` traversal.
func registerUIStatic(engine *gin.Engine, root string) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		absRoot = root
	}
	handler := func(c *gin.Context) {
		raw := c.Param("filepath")
		raw = strings.TrimPrefix(raw, "/")

		// Strip trailing slash so bookmarked /ui/settings/ collapses to
		// /ui/settings. The matched param does not include the /ui prefix.
		if strings.HasSuffix(raw, "/") && raw != "" {
			target := "/ui/" + strings.TrimSuffix(raw, "/")
			c.Redirect(http.StatusPermanentRedirect, target)
			return
		}

		if raw == "" || raw == "index" {
			http.ServeFile(c.Writer, c.Request, filepath.Join(absRoot, "index.html"))
			return
		}

		cleaned := filepath.Clean("/" + raw)
		fullPath := filepath.Join(absRoot, cleaned)
		// Reject any path that escaped the root after cleaning.
		if !strings.HasPrefix(fullPath, absRoot+string(filepath.Separator)) && fullPath != absRoot {
			c.AbortWithStatus(http.StatusNotFound)
			return
		}

		if info, statErr := os.Stat(fullPath); statErr == nil && !info.IsDir() {
			http.ServeFile(c.Writer, c.Request, fullPath)
			return
		}
		// Clean-URL fallback: /ui/settings → assets/ui/settings.html.
		htmlPath := fullPath + ".html"
		if info, statErr := os.Stat(htmlPath); statErr == nil && !info.IsDir() {
			http.ServeFile(c.Writer, c.Request, htmlPath)
			return
		}
		c.AbortWithStatus(http.StatusNotFound)
	}
	engine.GET("/ui", handler)
	engine.HEAD("/ui", handler)
	engine.GET("/ui/*filepath", handler)
	engine.HEAD("/ui/*filepath", handler)
}
