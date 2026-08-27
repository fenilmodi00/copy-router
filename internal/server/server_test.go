package server_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"workweave/router/internal/analytics"
	"workweave/router/internal/router/cluster"
	"workweave/router/internal/server"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

// fakeDeployedModelsSource is a stand-in for *cluster.Multiversion in route
// registration tests; the handler closures it backs are never invoked.
type fakeDeployedModelsSource struct{}

func (fakeDeployedModelsSource) DefaultDeployedModels() []cluster.DeployedEntry { return nil }

type healthCheckerFunc func(context.Context) error

func (f healthCheckerFunc) CheckHealth(ctx context.Context) error {
	return f(ctx)
}

// routeSet collects "METHOD path" pairs so assertions are robust to additions of unrelated product routes.
func routeSet(engine *gin.Engine) map[string]struct{} {
	out := make(map[string]struct{}, len(engine.Routes()))
	for _, r := range engine.Routes() {
		out[r.Method+" "+r.Path] = struct{}{}
	}
	return out
}

// adminDataPlaneRoutes returns the /admin/v1/* data-plane routes, EXCLUDING
// the login surfaces (/admin/v1/auth/* selfhosted, /account/v1/* selfserve)
// that legitimately differ across modes. Used by the parity assertions.
func adminDataPlaneRoutes(engine *gin.Engine) map[string]struct{} {
	const authLoginPrefix = "/admin/v1/auth/" // 15 chars
	out := make(map[string]struct{})
	for _, r := range engine.Routes() {
		path := r.Path
		// Skip operator-password login surface (selfhosted only).
		if strings.HasPrefix(path, authLoginPrefix) {
			continue
		}
		// Skip selfserve account login surface (different prefix entirely).
		if strings.HasPrefix(path, "/account/v1") {
			continue
		}
		// Only /admin/v1/* data plane.
		if strings.HasPrefix(path, "/admin/v1") {
			out[r.Method+" "+path] = struct{}{}
		}
	}
	return out
}

// selfservePreviouslyMissingRoutes are the 5 routes that were historically
// mounted in selfhosted but missing from selfserve, causing the frontend
// Provider Keys settings page and content-capture controls to 404 in
// selfserve. They now mount in both modes; TestRegister_DashboardParity
// asserts they are present in selfserve and that the selfserve data plane
// equals the selfhosted data plane.
var selfservePreviouslyMissingRoutes = []string{
	"PUT /admin/v1/provider-keys/:id/model-aliases",
	"GET /admin/v1/provider-keys/:id/models",
	"POST /admin/v1/provider-keys/discover-models",
	"GET /admin/v1/content-capture",
	"PUT /admin/v1/content-capture",
}

func TestRegister_DeploymentMode(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Product surface — always mounted regardless of deployment mode.
	productRoutes := []string{
		"GET /health",
		"GET /readyz",
		"GET /validate",
		"GET /v1/router/models",
		"POST /v1/messages",
		"POST /v1/chat/completions",
		"POST /v1/responses",
		"POST /v1/route",
		"POST /v1/route/preview",
		"POST /v1/messages/count_tokens",
		"GET /v1/models",
		"GET /v1/models/:model",
	}

	// Self-hoster dashboard surface — gated by DeploymentModeSelfHosted.
	dashboardRoutes := []string{
		"GET /",
		"GET /ui/*filepath",
		"HEAD /ui/*filepath",
		"POST /admin/v1/auth/login",
		"POST /admin/v1/auth/logout",
		"GET /admin/v1/auth/me",
		"GET /admin/v1/metrics/summary",
		"GET /admin/v1/metrics/timeseries",
		"GET /admin/v1/keys",
		"POST /admin/v1/keys",
		"DELETE /admin/v1/keys/:id",
		"GET /admin/v1/provider-keys",
		"POST /admin/v1/provider-keys",
		"PUT /admin/v1/provider-keys/:id/model-aliases",
		"DELETE /admin/v1/provider-keys/:id",
		"GET /admin/v1/config",
		"GET /admin/v1/excluded-models",
		"PUT /admin/v1/excluded-models",
	}

	t.Run("selfhosted mounts dashboard and product routes", func(t *testing.T) {
		engine := gin.New()
		// Nil services are fine: engine.Routes() inspection never invokes the closure-captured handlers.
		server.Register(engine, server.Services{DeployedModels: fakeDeployedModelsSource{}}, server.DeploymentModeSelfHosted)
		got := routeSet(engine)
		for _, want := range productRoutes {
			assert.Contains(t, got, want, "product route missing in selfhosted mode")
		}
		for _, want := range dashboardRoutes {
			assert.Contains(t, got, want, "dashboard route missing in selfhosted mode")
		}
	})

	t.Run("managed skips dashboard but keeps product routes", func(t *testing.T) {
		engine := gin.New()
		// Pass a non-nil DeployedModelsSource: managed prod always boots a
		// *cluster.Multiversion router, so the catalog endpoint must mount
		// even though the dashboard does not.
		server.Register(engine, server.Services{DeployedModels: fakeDeployedModelsSource{}}, server.DeploymentModeManaged)
		got := routeSet(engine)
		for _, want := range productRoutes {
			assert.Contains(t, got, want, "product route missing in managed mode")
		}
		for _, unwanted := range dashboardRoutes {
			assert.NotContains(t, got, unwanted, "dashboard route must not be mounted in managed mode")
		}
	})

	t.Run("nil deployed-models source skips catalog endpoint", func(t *testing.T) {
		engine := gin.New()
		server.Register(engine, server.Services{}, server.DeploymentModeManaged)
		got := routeSet(engine)
		assert.NotContains(t, got, "GET /v1/router/models", "catalog endpoint must not mount without a deployed-models source")
	})
}

// The export is a product surface, so it must reach managed installations too
// — mounting it inside the selfhosted block would strand every managed customer.
func TestRegisterMountsAnalyticsExportInBothModes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	analyticsRoutes := []string{
		"GET /v1/analytics/routing-decisions",
		"GET /v1/analytics/models",
		"GET /v1/analytics/schema",
	}

	for _, mode := range []server.DeploymentMode{server.DeploymentModeSelfHosted, server.DeploymentModeManaged} {
		t.Run(string(mode), func(t *testing.T) {
			engine := gin.New()
			server.Register(engine, server.Services{Analytics: analytics.NewService(nil, nil)}, mode)
			got := routeSet(engine)
			for _, want := range analyticsRoutes {
				assert.Contains(t, got, want)
			}
		})
	}

	t.Run("nil service leaves the surface unmounted", func(t *testing.T) {
		engine := gin.New()
		server.Register(engine, server.Services{}, server.DeploymentModeSelfHosted)
		got := routeSet(engine)
		for _, unwanted := range analyticsRoutes {
			assert.NotContains(t, got, unwanted)
		}
	})
}

func TestRegisterSeparatesLivenessFromReadiness(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	checker := healthCheckerFunc(func(context.Context) error {
		return errors.New("dependency unavailable")
	})
	server.Register(engine, server.Services{ReadinessChecker: checker}, server.DeploymentModeManaged)

	for _, test := range []struct {
		path       string
		wantStatus int
	}{
		{path: "/health", wantStatus: http.StatusOK},
		{path: "/readyz", wantStatus: http.StatusServiceUnavailable},
	} {
		t.Run(test.path, func(t *testing.T) {
			response := httptest.NewRecorder()
			engine.ServeHTTP(response, httptest.NewRequest(http.MethodGet, test.path, nil))
			assert.Equal(t, test.wantStatus, response.Code)
		})
	}
}

// TestRegister_DashboardParity asserts the selfserve /admin/v1/* data-plane
// route set EQUALS the selfhosted one (excluding the legitimately-different
// login surfaces: /admin/v1/auth/* operator password vs /account/v1/* aiand
// key). It also pins that the 5 routes historically missing from selfserve
// (provider-key model discovery/aliases + content-capture) are now present.
func TestRegister_DashboardParity(t *testing.T) {
	gin.SetMode(gin.TestMode)

	mkEngine := func(mode server.DeploymentMode) *gin.Engine {
		engine := gin.New()
		// Non-nil DeployedModels so the 6 excluded/allowed/providers rows
		// mount in both modes (they're needsDeployedModels). Route
		// inspection never invokes the handler bodies.
		server.Register(engine, server.Services{DeployedModels: fakeDeployedModelsSource{}}, mode)
		return engine
	}

	selfhostedData := adminDataPlaneRoutes(mkEngine(server.DeploymentModeSelfHosted))
	selfserveData := adminDataPlaneRoutes(mkEngine(server.DeploymentModeSelfServe))

	// Parity: the selfserve data-plane set equals the selfhosted one.
	assert.Equal(t, selfhostedData, selfserveData, "selfserve /admin/v1/* data plane must equal selfhosted (login surfaces are intentionally excluded)")

	// The 5 routes historically missing from selfserve are now present.
	for _, r := range selfservePreviouslyMissingRoutes {
		assert.Contains(t, selfserveData, r, "previously-missing route must now mount in selfserve: %s", r)
		assert.Contains(t, selfhostedData, r, "previously-missing route must remain mounted in selfhosted: %s", r)
	}
}

// TestRegister_DashboardConditionalRoutes verifies the needs bitmask: a nil
// DeployedModels skips the 6 excluded/allowed/providers rows but keeps the
// alias/discover/list/content-capture rows (which tolerate nil); a nil
// AiandCatalogHandler skips GET /aiand/models. Behavior must match across
// modes.
func TestRegister_DashboardConditionalRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)

	deployedGatedRoutes := []string{
		"GET /admin/v1/excluded-models",
		"PUT /admin/v1/excluded-models",
		"GET /admin/v1/allowed-models",
		"PUT /admin/v1/allowed-models",
		"GET /admin/v1/excluded-providers",
		"PUT /admin/v1/excluded-providers",
	}
	// Routes that mount unconditionally (needs: 0) — must appear even with
	// nil DeployedModels and nil AiandCatalogHandler, in both modes.
	alwaysMounted := []string{
		"PUT /admin/v1/provider-keys/:id/model-aliases",
		"GET /admin/v1/provider-keys/:id/models",
		"POST /admin/v1/provider-keys/discover-models",
		"GET /admin/v1/content-capture",
		"PUT /admin/v1/content-capture",
	}

	for _, mode := range []server.DeploymentMode{server.DeploymentModeSelfHosted, server.DeploymentModeSelfServe} {
		t.Run(string(mode)+"/nil_deployed", func(t *testing.T) {
			engine := gin.New()
			// No DeployedModels, no AiandCatalogHandler.
			server.Register(engine, server.Services{}, mode)
			got := routeSet(engine)
			for _, r := range deployedGatedRoutes {
				assert.NotContains(t, got, r, "deployed-gated route must not mount with nil DeployedModels: %s", r)
			}
			// The deployed-gated rows aside, the always-mounted rows
			// (needs: 0) — including the previously-selfserve-missing
			// discovery/aliases/content-capture rows — mount in both
			// modes even with nil DeployedModels. aiand/models
			// requires needsAiandCatalog; absent here.
			for _, r := range alwaysMounted {
				assert.Contains(t, got, r, "needs-0 route must mount even with nil DeployedModels: %s", r)
			}
			assert.NotContains(t, got, "GET /admin/v1/aiand/models")
		})
	}

	t.Run("nil_aiand_catalog_skips_aiand_models", func(t *testing.T) {
		engine := gin.New()
		server.Register(engine, server.Services{DeployedModels: fakeDeployedModelsSource{}}, server.DeploymentModeSelfHosted)
		got := routeSet(engine)
		assert.NotContains(t, got, "GET /admin/v1/aiand/models", "aiand/models must not mount with nil AiandCatalogHandler")
	})
}

// TestRegister_DashboardRouteSet_ByteIdentical_BeforeAfter pins the exact
// selfhosted and selfserve /admin/v1/* route sets. Any drift fails. When a gap
// row is extended to selfserve (modeSelfServe added to its modes), update
// selfserveWant to include the newly-mounted route.
func TestRegister_DashboardRouteSet_ByteIdentical_BeforeAfter(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// selfhosted /admin/v1/* (data plane + auth login surface), with
	// aiand/models added when a catalog handler is wired (the stub
	// below mounts it).
	selfhostedWant := []string{
		"DELETE /admin/v1/keys/:id",
		"DELETE /admin/v1/provider-keys/:id",
		"GET /admin/v1/aiand/models",
		"GET /admin/v1/allowed-models",
		"GET /admin/v1/auth/me",
		"GET /admin/v1/config",
		"GET /admin/v1/content-capture",
		"GET /admin/v1/excluded-models",
		"GET /admin/v1/excluded-providers",
		"GET /admin/v1/keys",
		"GET /admin/v1/metrics/details",
		"GET /admin/v1/metrics/model-breakdown",
		"GET /admin/v1/metrics/summary",
		"GET /admin/v1/metrics/timeseries",
		"GET /admin/v1/onboarding",
		"GET /admin/v1/provider-keys",
		"GET /admin/v1/provider-keys/:id/models",
		"GET /admin/v1/routing-preferences",
		"POST /admin/v1/auth/login",
		"POST /admin/v1/auth/logout",
		"POST /admin/v1/keys",
		"POST /admin/v1/keys/:id/rotate",
		"POST /admin/v1/provider-keys",
		"POST /admin/v1/provider-keys/discover-models",
		"PUT /admin/v1/allowed-models",
		"PUT /admin/v1/content-capture",
		"PUT /admin/v1/excluded-models",
		"PUT /admin/v1/excluded-providers",
		"PUT /admin/v1/provider-keys/:id/model-aliases",
		"PUT /admin/v1/routing-preferences",
	}
	// selfserve /admin/v1/* (data plane; account login is at /account/v1,
	// deliberately out of scope). Full parity with selfhosted's data
	// plane: the provider-key model discovery/aliases and content-capture
	// rows now mount in selfserve too.
	selfserveWant := []string{
		"DELETE /admin/v1/keys/:id",
		"DELETE /admin/v1/provider-keys/:id",
		"GET /admin/v1/aiand/models",
		"GET /admin/v1/allowed-models",
		"GET /admin/v1/config",
		"GET /admin/v1/content-capture",
		"GET /admin/v1/excluded-models",
		"GET /admin/v1/excluded-providers",
		"GET /admin/v1/keys",
		"GET /admin/v1/metrics/details",
		"GET /admin/v1/metrics/model-breakdown",
		"GET /admin/v1/metrics/summary",
		"GET /admin/v1/metrics/timeseries",
		"GET /admin/v1/onboarding",
		"GET /admin/v1/provider-keys",
		"GET /admin/v1/provider-keys/:id/models",
		"GET /admin/v1/routing-preferences",
		"POST /admin/v1/keys",
		"POST /admin/v1/keys/:id/rotate",
		"POST /admin/v1/provider-keys",
		"POST /admin/v1/provider-keys/discover-models",
		"PUT /admin/v1/allowed-models",
		"PUT /admin/v1/content-capture",
		"PUT /admin/v1/excluded-models",
		"PUT /admin/v1/excluded-providers",
		"PUT /admin/v1/provider-keys/:id/model-aliases",
		"PUT /admin/v1/routing-preferences",
	}

	stubCatalog := func(c *gin.Context) {}

	shEngine := gin.New()
	server.Register(shEngine, server.Services{
		DeployedModels:      fakeDeployedModelsSource{},
		AiandCatalogHandler: stubCatalog,
	}, server.DeploymentModeSelfHosted)
	shAdmin := setOf(filterAdminV1(routeSet(shEngine)))
	wantSH := setOf(selfhostedWant)
	assert.Equal(t, wantSH, shAdmin, "selfhosted /admin/v1/* set drifted from pre-refactor baseline")

	ssEngine := gin.New()
	server.Register(ssEngine, server.Services{
		DeployedModels:      fakeDeployedModelsSource{},
		AiandCatalogHandler: stubCatalog,
	}, server.DeploymentModeSelfServe)
	ssAdmin := setOf(filterAdminV1(routeSet(ssEngine)))
	wantSS := setOf(selfserveWant)
	assert.Equal(t, wantSS, ssAdmin, "selfserve /admin/v1/* set drifted (a gap row's modes bitmask should be updated in lockstep with this want set)")
}

// hasAdminV1Prefix reports whether a "METHOD /path" route string's path starts
// with /admin/v1. Used to scope the byte-identical guard to the dashboard surface.
func hasAdminV1Prefix(r string) bool {
	for i := 0; i < len(r); i++ {
		if r[i] == ' ' {
			path := r[i+1:]
			return len(path) >= 9 && path[:9] == "/admin/v1"
		}
	}
	return false
}

func filterAdminV1(set map[string]struct{}) []string {
	out := make([]string, 0, len(set))
	for r := range set {
		if hasAdminV1Prefix(r) {
			out = append(out, r)
		}
	}
	return out
}

func setOf(xs []string) map[string]struct{} {
	out := make(map[string]struct{}, len(xs))
	for _, x := range xs {
		out[x] = struct{}{}
	}
	return out
}
