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

func TestRegister_HostedMode(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Product surface — always mounted.
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

	// Hosted dashboard surface: data plane under /v1/*, login at
	// /account/v1/*, and NO admin surface anywhere.
	dashboardRoutes := []string{
		"GET /",
		"GET /ui/*filepath",
		"HEAD /ui/*filepath",
		"POST /account/v1/login",
		"POST /account/v1/logout",
		"GET /account/v1/me",
		"GET /v1/metrics/summary",
		"GET /v1/metrics/timeseries",
		"GET /v1/keys",
		"POST /v1/keys",
		"DELETE /v1/keys/:id",
		"GET /v1/provider-keys",
		"POST /v1/provider-keys",
		"PUT /v1/provider-keys/:id/model-aliases",
		"DELETE /v1/provider-keys/:id",
		"GET /v1/config",
		"GET /v1/excluded-models",
		"PUT /v1/excluded-models",
	}

	engine := gin.New()
	// Nil services are fine: engine.Routes() inspection never invokes the closure-captured handlers.
	server.Register(engine, server.Services{DeployedModels: fakeDeployedModelsSource{}})
	got := routeSet(engine)
	for _, want := range productRoutes {
		assert.Contains(t, got, want, "product route missing in hosted mode")
	}
	for _, want := range dashboardRoutes {
		assert.Contains(t, got, want, "dashboard route missing in hosted mode")
	}

	// No admin surface exists anywhere: no path contains the segment /admin.
	for r := range got {
		path := r[strings.Index(r, " ")+1:]
		assert.NotContains(t, path, "/admin", "no admin surface may exist: %s", r)
	}
}

// The export is a product surface and must mount in hosted mode.
func TestRegisterMountsAnalyticsExport(t *testing.T) {
	gin.SetMode(gin.TestMode)
	analyticsRoutes := []string{
		"GET /v1/analytics/routing-decisions",
		"GET /v1/analytics/models",
		"GET /v1/analytics/schema",
	}

	engine := gin.New()
	server.Register(engine, server.Services{Analytics: analytics.NewService(nil, nil)})
	_ = engine
	got := routeSet(engine)
	for _, want := range analyticsRoutes {
		assert.Contains(t, got, want)
	}

	t.Run("nil service leaves the surface unmounted", func(t *testing.T) {
		engine := gin.New()
		server.Register(engine, server.Services{})
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
	server.Register(engine, server.Services{ReadinessChecker: checker})

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

// TestRegister_DashboardConditionalRoutes verifies the needs bitmask: a nil
// DeployedModels skips the excluded/allowed/providers rows but keeps the
// alias/discover/list/content-capture rows (which tolerate nil); a nil
// AiandCatalogHandler skips GET /aiand/models.
func TestRegister_DashboardConditionalRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)

	deployedGatedRoutes := []string{
		"GET /v1/excluded-models",
		"PUT /v1/excluded-models",
		"GET /v1/allowed-models",
		"PUT /v1/allowed-models",
	}
	// Routes that mount unconditionally (needs: 0) — must appear even with
	// nil DeployedModels and nil AiandCatalogHandler.
	alwaysMounted := []string{
		"PUT /v1/provider-keys/:id/model-aliases",
		"GET /v1/provider-keys/:id/models",
		"POST /v1/provider-keys/discover-models",
		"GET /v1/content-capture",
		"PUT /v1/content-capture",
	}

	t.Run("nil_deployed", func(t *testing.T) {
		engine := gin.New()
		// No DeployedModels, no AiandCatalogHandler.
		server.Register(engine, server.Services{})
		got := routeSet(engine)
		for _, r := range deployedGatedRoutes {
			assert.NotContains(t, got, r, "deployed-gated route must not mount with nil DeployedModels: %s", r)
		}
		for _, r := range alwaysMounted {
			assert.Contains(t, got, r, "needs-0 route must mount even with nil DeployedModels: %s", r)
		}
		assert.NotContains(t, got, "GET /v1/aiand/models")
	})

	t.Run("nil_aiand_catalog_skips_aiand_models", func(t *testing.T) {
		engine := gin.New()
		server.Register(engine, server.Services{DeployedModels: fakeDeployedModelsSource{}})
		got := routeSet(engine)
		assert.NotContains(t, got, "GET /v1/aiand/models", "aiand/models must not mount with nil AiandCatalogHandler")
	})
}

// TestRegister_DashboardRouteSet pins the exact hosted /v1/* dashboard route
// set. Any drift fails; update hostedWant when a dashboard row is added or
// removed in dashboard_routes.go.
func TestRegister_DashboardRouteSet(t *testing.T) {
	gin.SetMode(gin.TestMode)

	hostedWant := []string{
		"DELETE /v1/keys/:id",
		"DELETE /v1/provider-keys/:id",
		"GET /v1/aiand/models",
		"GET /v1/allowed-models",
		"GET /v1/config",
		"GET /v1/content-capture",
		"GET /v1/excluded-models",
		"GET /v1/keys",
		"GET /v1/metrics/details",
		"GET /v1/metrics/model-breakdown",
		"GET /v1/metrics/summary",
		"GET /v1/metrics/timeseries",
		"GET /v1/onboarding",
		"GET /v1/provider-keys",
		"GET /v1/provider-keys/:id/models",
		"GET /v1/routing-preferences",
		"POST /v1/keys",
		"POST /v1/keys/:id/rotate",
		"POST /v1/provider-keys",
		"POST /v1/provider-keys/discover-models",
		"POST /v1/playground/route",
		"POST /v1/playground/chat",
		"PUT /v1/allowed-models",
		"PUT /v1/content-capture",
		"PUT /v1/excluded-models",
		"PUT /v1/provider-keys/:id/model-aliases",
		"PUT /v1/routing-preferences",
	}

	stubCatalog := func(c *gin.Context) {}

	engine := gin.New()
	server.Register(engine, server.Services{
		DeployedModels:      fakeDeployedModelsSource{},
		AiandCatalogHandler: stubCatalog,
	})
	got := routeSet(engine)

	// Every dashboard row must be present, and the dashboard set must be
	// exact: the mounted /v1/* dashboard rows equal hostedWant exactly
	// (product routes under /v1 are asserted by TestRegister_HostedMode).
	want := setOf(hostedWant)
	dashboard := map[string]struct{}{}
	for r := range got {
		path := r[strings.Index(r, " ")+1:]
		if _, ok := want[r]; ok {
			dashboard[r] = struct{}{}
			continue
		}
		assert.False(t, strings.HasPrefix(path, "/v1/metrics") || strings.HasPrefix(path, "/v1/keys") ||
			strings.HasPrefix(path, "/v1/provider-keys") || strings.HasPrefix(path, "/v1/playground") ||
			strings.HasPrefix(path, "/v1/onboarding") || strings.HasPrefix(path, "/v1/routing-preferences") ||
			strings.HasPrefix(path, "/v1/content-capture") || strings.HasPrefix(path, "/v1/excluded") ||
			strings.HasPrefix(path, "/v1/allowed") || strings.HasPrefix(path, "/v1/aiand") ||
			strings.HasPrefix(path, "/v1/config"), "unexpected dashboard row: %s", r)
	}
	assert.Equal(t, want, dashboard, "hosted /v1/* dashboard set drifted")
}

func setOf(xs []string) map[string]struct{} {
	out := make(map[string]struct{}, len(xs))
	for _, x := range xs {
		out[x] = struct{}{}
	}
	return out
}
