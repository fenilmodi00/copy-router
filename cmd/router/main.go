// Command router is the entry point for the router service — the composition
// root where repositories, routers, and provider clients are wired into
// auth.Service and proxy.Service.
package main

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"workweave/router/internal/analytics"
	"workweave/router/internal/api/admin"
	"workweave/router/internal/auth"
	"workweave/router/internal/billing"
	"workweave/router/internal/config"
	"workweave/router/internal/flags"
	"workweave/router/internal/observability"
	"workweave/router/internal/observability/apm"
	"workweave/router/internal/observability/otel"
	"workweave/router/internal/policyclient"
	"workweave/router/internal/postgres"
	"workweave/router/internal/providers"
	aiandProvider "workweave/router/internal/providers/aiand"
	openaiCompatProvider "workweave/router/internal/providers/openaicompat"
	"workweave/router/internal/proxy"
	"workweave/router/internal/proxy/usage"
	"workweave/router/internal/router"
	"workweave/router/internal/router/bandit"
	"workweave/router/internal/router/banditexplore"
	"workweave/router/internal/router/cache"
	"workweave/router/internal/router/catalog"
	"workweave/router/internal/router/cluster"
	"workweave/router/internal/router/handover"
	"workweave/router/internal/router/hmm"
	"workweave/router/internal/router/planner"
	"workweave/router/internal/router/policy"
	"workweave/router/internal/router/rl"
	"workweave/router/internal/router/sessionpin"
	"workweave/router/internal/server"
	"workweave/router/internal/wif"

	_ "time/tzdata"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	logger := observability.Get()
	if err := router.ValidateCatalogReasoningCapabilities(); err != nil {
		logger.Error("Refusing to boot with invalid model reasoning capabilities", "err", err)
		panic(err)
	}

	cfg, err := pgxpool.ParseConfig(config.PostgresDSN())
	if err != nil {
		logger.Error("Failed to parse postgres DSN", "err", err)
		panic(err)
	}
	// Defense-in-depth: pin every connection to the router schema so a missing
	// `?search_path=router` in DATABASE_URL can't leak into public.*.
	cfg.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
		_, err := conn.Exec(ctx, "SET search_path TO router, public")
		return err
	}
	// 6 conns covers MarkUsed writes plus session-pin traffic (auth-cache and
	// the in-proc LRU absorb most reads). If pgxpool wait p95 climbs above 1ms
	// with pinning on, that's the migrate-to-Memorystore signal, not a bigger pool.
	cfg.MaxConns = 6
	cfg.MinConns = 1
	cfg.MaxConnLifetime = 30 * time.Minute
	cfg.MaxConnIdleTime = 10 * time.Minute
	cfg.HealthCheckPeriod = 1 * time.Minute

	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		logger.Error("Failed to construct postgres pool", "err", err)
		panic(err)
	}
	defer pool.Close()

	// pgxpool connects lazily; first request otherwise pays to build the pool inside
	// its own budget. Bounded and warn-only so an unreachable DB can't stall boot.
	pingCtx, pingCancel := context.WithTimeout(context.Background(), 5*time.Second)
	pingErr := pool.Ping(pingCtx)
	pingCancel()
	if pingErr != nil {
		logger.Warn("Postgres ping at boot failed; early requests may be slow", "err", pingErr)
	}

	// Gates the self-hoster dashboard + /admin/v1/* API. Defaults to selfhosted
	// so docker-compose/bare-binary deploys work out of the box; managed Cloud
	// Run services set ROUTER_DEPLOYMENT_MODE=managed to drop that surface.
	deploymentMode := server.DeploymentMode(config.GetOr("ROUTER_DEPLOYMENT_MODE", string(server.DeploymentModeSelfHosted)))
	switch deploymentMode {
	case server.DeploymentModeSelfHosted, server.DeploymentModeManaged, server.DeploymentModeSelfServe:
	default:
		err := fmt.Errorf("Invalid ROUTER_DEPLOYMENT_MODE %q (expected %q, %q, or %q)", deploymentMode, server.DeploymentModeSelfHosted, server.DeploymentModeManaged, server.DeploymentModeSelfServe)
		logger.Error("Refusing to boot with invalid deployment mode", "err", err)
		panic(err)
	}
	logger.Info("Router deployment mode", "mode", deploymentMode)

	translationCompatibilityMode, err := config.TranslationCompatibilityMode()
	if err != nil {
		logger.Error("Invalid translation compatibility mode", "err", err)
		panic(err)
	}
	logger.Info("Translation compatibility configured", "mode", translationCompatibilityMode)

	// EXTERNAL_KEY_ENCRYPTION_KEY is optional: unset falls back to a no-op
	// encryptor (BYOK secrets stored unencrypted, fine for self-hosted/local).
	// A malformed keyset still fails closed — only a genuinely absent var bypasses.
	keysetJSON := config.GetOr("EXTERNAL_KEY_ENCRYPTION_KEY", "")
	var encryptor auth.Encryptor
	if keysetJSON == "" {
		logger.Warn("EXTERNAL_KEY_ENCRYPTION_KEY not set; BYOK secrets will be stored unencrypted at rest. Set EXTERNAL_KEY_ENCRYPTION_KEY to a Tink AES-256-GCM keyset to enable encryption.")
		encryptor = auth.NoOpEncryptor{}
	} else {
		encryptor, err = auth.NewTinkEncryptor(keysetJSON)
		if err != nil {
			logger.Error("Failed to create Tink encryptor from keyset", "err", err)
			panic(err)
		}
	}

	repo := postgres.NewRepository(pool, encryptor)

	providerMap := make(map[string]providers.Client)
	// envKeyedProviders = providers with a real deployment-level API key.
	// Feeds resolveHardPinModel so compaction only lands where deployment
	// auth actually exists — a BYOK/passthrough-only provider would 401.
	envKeyedProviders := make(map[string]struct{})

	// Wired by default in managed mode. A boot-time health-check error (e.g.
	// transient pool unreadiness) defaults to billing-enabled rather than
	// silently falling to BYOK-only, which would 400 every request.
	var billingSvc *billing.Service
	if deploymentMode == server.DeploymentModeManaged {
		byokFeeRate := parseEnvFloat("BYOK_FEE_RATE", billing.DefaultByokFeeRate)
		billingRepo := postgres.NewBillingRepo(pool)
		bootCtx, bootCancel := context.WithTimeout(context.Background(), 5*time.Second)
		tablesExist, billingCheckErr := billingRepo.BillingTablesExist(bootCtx)
		bootCancel()
		switch {
		case billingCheckErr != nil:
			logger.Warn("Boot billing health check errored; defaulting to billing-enabled in managed mode", "err", billingCheckErr)
			billingSvc = billing.NewService(billingRepo).WithByokFeeRate(byokFeeRate)
		case !tablesExist:
			logger.Warn("Billing tables missing from router schema; staying in BYOK-only mode. Apply db migration 0006_credit_billing to enable billing.")
		default:
			billingSvc = billing.NewService(billingRepo).WithByokFeeRate(byokFeeRate)
			logger.Info("Router billing enabled", "min_balance_usd_micros", billing.MinBalanceMicros, "byok_fee_rate", byokFeeRate)
		}
	}

	// Managed without billing stays BYOK-only (avoids spending platform-key
	// budget if billing fails to wire); managed with billing flips to
	// platform-key mode gated by balance checks. Self-hosted is never BYOK-only.
	byokOnly := deploymentMode == server.DeploymentModeManaged && billingSvc == nil

	// ai& is the sole upstream provider of this deployment.

	// ai& (aiand.com) OpenAI-compatible inference surface for open-weight
	// models (GLM, DeepSeek, Kimi, Qwen, Gemma, gpt-oss). Registration and the
	// dashboard's live-catalog endpoint share the deployment's key/URL.
	var aiandCatalogHandler gin.HandlerFunc
	{

		aiandBaseURL := config.GetOr("AIAND_API_URL", openaiCompatProvider.AiandBaseURL)
		registerDeploymentKeyedProvider(providerMap, envKeyedProviders, logger,
			providers.ProviderAiand, "ai&", "AIAND_API_KEY", aiandBaseURL, byokOnly,
			func(key, baseURL string) providers.Client {
				return openaiCompatProvider.NewClientWithModelIDMap(key, baseURL, upstreamIDsForProvider(providers.ProviderAiand))
			})
		deploymentAiandKey := config.GetOr("AIAND_API_KEY", "")
		// Self-serve mounts the catalog route even without a deployment key:
		// each signed-in user supplies their own aiand BYOK credential from
		// login. Self-hosted still uses AIAND_API_KEY when set.
		if deploymentAiandKey != "" || deploymentMode == server.DeploymentModeSelfServe {
			admin.AiandCatalogHandler(deploymentAiandKey, aiandBaseURL,
				// Deployment key rides this client; a 3xx must fail the != 200
				// status check instead of relaying the key to another host.
				&http.Client{
					Timeout: aiandCatalogBudget,
					CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
						return http.ErrUseLastResponse
					},
				}, time.Now)
			if deploymentAiandKey != "" {
				logger.Info("ai& catalog endpoint enabled", "base_url", aiandBaseURL)
			} else {
				logger.Info("ai& catalog endpoint enabled (per-user BYOK keys)", "base_url", aiandBaseURL)
			}
		} else {
			logger.Info("ai& catalog endpoint disabled (AIAND_API_KEY not set)")
		}
	}

	availableProviders := make(map[string]struct{}, len(providerMap))
	for name := range providerMap {
		availableProviders[name] = struct{}{}
	}

	// A provider missing a ProviderFamilies entry would silently 502 every
	// request despite looking "enabled" — panic at boot instead.
	registeredProviders := make([]string, 0, len(providerMap))
	for name := range providerMap {
		registeredProviders = append(registeredProviders, name)
	}
	if err := providers.ValidateDispatchable(registeredProviders); err != nil {
		logger.Error("Registered provider missing a translation family; refusing to boot", "err", err)
		panic(err)
	}

	rtr, defaultEmbedderID, err := buildClusterScorer(availableProviders)
	if err != nil {
		// Ops alerts on Cloud Run boot failures; silent degradation would mask quality regressions.
		logger.Error("Cluster scorer failed to build; refusing to boot", "err", err)
		panic(err)
	}
	logger.Info("Routing via cluster scorer", "embedder", defaultEmbedderID)

	cache := auth.NewLRUAPIKeyCache(10000, 50000, 5*time.Minute, 60*time.Second)
	userCache := auth.NewLRUUserCache(50000, 10*time.Minute)
	// 5-min TTL matches the API-key cache so both halves share one staleness bound.
	userClusterCache := auth.NewLRUUserClusterListCache(50000, 5*time.Minute)

	notifier := auth.InstallationChangeNotifier(auth.NoOpInstallationChangeNotifier{})
	logger.Warn("Installation cache invalidation is NoOp; single-replica OK (5-min TTL)")

	// Deployment-wide escape hatch: suppresses every per-organization flag
	// override so an incident rollback via env var always wins. Off by default,
	// i.e. per-org overrides are honored.
	flagOverridesDisabled := config.GetOr("ROUTER_FLAG_OVERRIDES_DISABLED", "false") == "true"
	if flagOverridesDisabled {
		logger.Info("Per-organization flag overrides disabled deployment-wide (ROUTER_FLAG_OVERRIDES_DISABLED=true)")
	}

	authSvc := auth.NewService(repo.Installations, repo.APIKeys, repo.ExternalAPIKeys, repo.Users, cache, userCache, time.Now).
		WithEncryptor(encryptor).
		WithInstallationChangeNotifier(notifier).
		WithClusterModelLists(repo.ClusterModelLists).
		WithUserClusterModelLists(repo.UserClusterModelLists, userClusterCache).
		WithWIFTokenSource(buildWIFTokenSource(logger)).
		WithFlagOverridesDisabled(flagOverridesDisabled)

	// Self-serve mode: the dashboard is secured by an aiand-key login instead
	// of the operator password. Each aiand user gets their own installation
	// (account id = installation external_id). The login probe validates user
	// keys against aiand's identity API; Models + Playground bill the stored
	// per-user BYOK key — AIAND_API_KEY is optional in selfserve.
	if deploymentMode == server.DeploymentModeSelfServe {
		if repo.Accounts == nil || repo.LoginSessions == nil {
			logger.Error("Self-serve mode requires account SQLC repos; refusing to boot", "err", nil)
			panic("selfserve mode: account repos not wired")
		}
		keyVerifier := &aiandProvider.KeyVerifier{
			Client: &http.Client{
				Timeout: 15 * time.Second,
				// Verifies user sk- keys as Bearer; never follow a redirect
				// carrying them. The refused 3xx fails the identity probe.
				CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
					return http.ErrUseLastResponse
				},
			},
			IdentityURL: config.GetOr("AIAND_IDENTITY_URL", aiandProvider.DefaultBaseURL),
		}
		authSvc.WithAccountRepos(repo.Accounts, repo.LoginSessions).WithKeyVerifier(keyVerifier)
		logger.Info("Self-serve login enabled", "aiand_identity_url", keyVerifier.IdentityURL)
	}

	// Managed mode doesn't mount the dashboard, so this only matters selfhosted.
	if deploymentMode == server.DeploymentModeSelfHosted {
		adminPassword := config.GetOr("ROUTER_ADMIN_PASSWORD", "")
		if adminPassword == "" {
			adminPassword = "admin"
			logger.Warn("ROUTER_ADMIN_PASSWORD not set; using default 'admin'. Set ROUTER_ADMIN_PASSWORD to secure the dashboard.")
		}
		authSvc.WithAdminPassword(adminPassword)
	}
	embedOnlyUser := config.GetOr("ROUTER_EMBED_ONLY_USER_MESSAGE", "true") == "true"
	if embedOnlyUser {
		logger.Info("Cluster scorer embedding user-role text only (ROUTER_EMBED_ONLY_USER_MESSAGE=true)")
	} else {
		logger.Info("Cluster scorer embedding concatenated stream (ROUTER_EMBED_ONLY_USER_MESSAGE=false)")
	}
	escapeNormalize := config.GetOr("ROUTER_DEEPSEEK_ESCAPE_NORMALIZE", "false") == "true"
	if escapeNormalize {
		logger.Info("Edit-tool escape-sequence repair enabled (ROUTER_DEEPSEEK_ESCAPE_NORMALIZE=true)")
	}
	emitter, err := buildOtelEmitter(string(deploymentMode))
	if err != nil {
		logger.Error("Failed to create OTel emitter", "err", err)
		panic(err)
	}
	// Keep as a true nil interface, not a nil *otel.Emitter wrapped in an
	// interface (which would be non-nil), so proxy's s.emitter == nil checks
	// work correctly.
	var telemetryEmitter proxy.TelemetryEmitter
	if emitter != nil {
		telemetryEmitter = emitter
	}

	semanticCache := buildSemanticCache(rtr)

	// Default-on: the scorer's α-blend ignores prompt-cache continuity, so
	// without pinning, mid-conversation provider switches pay a cache-write
	// penalty (Anthropic caches per-model) that dwarfs routing savings over a
	// long trajectory. Set ROUTER_SESSION_PIN_ENABLED=false to opt out.
	var pinStore sessionpin.Store
	if config.GetOr("ROUTER_SESSION_PIN_ENABLED", "true") == "true" {
		pinStore = postgres.NewSessionPinRepo(pool)
		safeGo(logger, "session-pin-sweep", func() { runSessionPinSweep(context.Background(), pinStore) })
		logger.Info("Session pin store enabled (sliding 1h TTL, hourly sweep)")
	}

	hardPinExplore := config.GetOr("ROUTER_HARD_PIN_EXPLORE", "true") == "true"
	// Hard-pin compaction runs on every installation, so it must land on a
	// provider with real deployment auth (env-keyed, excluding BYOK/passthrough)
	// or it 401s. In managed/byokOnly mode no provider has deployment auth, so
	// the boot-time pin here is just a fallback — per-request resolution below
	// does the real work.
	hardPinProvider, hardPinModel := resolveHardPinModel(envKeyedProviders, logger)
	if hardPinExplore {
		logger.Info("Explore sub-agent hard-pin enabled", "provider", hardPinProvider, "model", hardPinModel)
	}
	logger.Info("Hard-pin model resolved", "provider", hardPinProvider, "model", hardPinModel, "byok_only", byokOnly)

	// Per-request hard-pin resolver: closes over the default cluster bundle so
	// the proxy can resolve per-request against the request's enabled-providers
	// and the installation's excluded_models (the hard-pin tier bypasses the
	// scorer, the only component that otherwise applies excluded_models).
	// nil if the bundle fails to load, falling back to boot-time hardPin{Provider,Model}.
	// Not wired when ROUTER_HARD_PIN_MODEL is set — an operator override is
	// absolute and must never be silently rewritten by excluded_models.
	var hardPinResolver func(enabled, denySet map[string]struct{}) (string, string, bool)
	if config.GetOr("ROUTER_HARD_PIN_MODEL", "") == "" {
		reqVersion := config.GetOr("ROUTER_CLUSTER_VERSION", cluster.LatestVersion)
		if version, vErr := cluster.ResolveVersion(reqVersion); vErr == nil {
			if bundle, bErr := cluster.LoadBundle(version); bErr == nil {
				meta, registry := bundle.Metadata, bundle.Registry
				hardPinResolver = func(enabled, denySet map[string]struct{}) (string, string, bool) {
					return cluster.FastestModelInSet(meta, registry, enabled, denySet, nil)
				}
				logger.Info("Hard-pin resolver wired (per-request fastest model from cluster bundle, applies excluded_models)", "version", version, "byok_only", byokOnly)
			} else {
				logger.Warn("Hard-pin resolver disabled: cluster bundle failed to load; hard-pin will fall back to boot-time defaults and cannot apply excluded_models", "err", bErr, "version", version)
			}
		} else {
			logger.Warn("Hard-pin resolver disabled: could not resolve cluster version; hard-pin will fall back to boot-time defaults and cannot apply excluded_models", "err", vErr)
		}
	} else {
		logger.Info("Hard-pin resolver not wired: ROUTER_HARD_PIN_MODEL operator override is set and absolute by design", "model", hardPinModel)
	}

	// ROUTER_SUBAGENT_PROVIDER + ROUTER_SUBAGENT_MODEL pin SubAgentDispatch
	// turns to a distinct provider/model; both must be set together.
	subAgentProvider := config.GetOr("ROUTER_SUBAGENT_PROVIDER", "")
	subAgentModel := config.GetOr("ROUTER_SUBAGENT_MODEL", "")
	switch {
	case subAgentModel != "" && subAgentProvider == "":
		logger.Warn("ROUTER_SUBAGENT_MODEL set without ROUTER_SUBAGENT_PROVIDER; ignoring sub-agent override")
		subAgentModel = ""
	case subAgentProvider != "" && subAgentModel == "":
		logger.Warn("ROUTER_SUBAGENT_PROVIDER set without ROUTER_SUBAGENT_MODEL; ignoring sub-agent override")
		subAgentProvider = ""
	case subAgentModel != "":
		logger.Info("Sub-agent routing override enabled", "provider", subAgentProvider, "model", subAgentModel)
	}

	// Default-eligible set: env-keyed providers only. BYOK/client credentials
	// add to this per-request inside enabledProvidersForRequest.
	deploymentEligible := make(map[string]struct{}, len(envKeyedProviders))
	for p := range envKeyedProviders {
		deploymentEligible[p] = struct{}{}
	}

	// Planner + handover config (Prism-style cache-aware routing); each default
	// below can be overridden per deployment.
	plannerEnabled := config.GetOr("ROUTER_PLANNER_ENABLED", "true") == "true"
	scoreToolResultTurns := config.GetOr("ROUTER_SCORE_TOOL_RESULT_TURNS", "true") == "true"
	// Defensive backstop for Anthropic's cyber safety classifier: re-pin a
	// session off a model that returned a safety refusal. Default off; enabled
	// on the defensive deploy. Fallback target when the pin has no runner-up.
	cyberRefusalRepin := config.GetOr("ROUTER_CYBER_REFUSAL_REPIN", "false") == "true"
	cyberRefusalFallbackModel := config.GetOr("ROUTER_CYBER_REFUSAL_FALLBACK_MODEL", "moonshotai/kimi-k2.7")
	effortEscalation := config.GetOr("ROUTER_EFFORT_ESCALATION", "false") == "true"
	scopedSearchRequirement := config.GetOr("ROUTER_SCOPED_SEARCH_REQUIREMENT", "true") == "true"
	searchRequirementDecayTurns := parseEnvInt("ROUTER_SEARCH_REQUIREMENT_DECAY_TURNS", proxy.DefaultSearchRequirementDecayTurns)
	// Kill switch for degrading to a same-cluster candidate when the routed
	// model's bindings are all exhausted by a transient upstream fault.
	siblingFailover := config.GetOr("ROUTER_SIBLING_FAILOVER", "true") == "true"
	sseKeepalive := sseKeepaliveInterval()
	ccOrchToolsCrossVendor := config.GetOr("ROUTER_CC_ORCH_TOOLS_CROSSVENDOR", "true") == "true"
	// Per-turn large-vs-small action-classifier swap. Off by default until the
	// Layer-2 extrinsic validation clears it; enabling loads the compiled-in head.
	bandSwapEnabled := config.GetOr("ROUTER_BAND_SWAP", "false") == "true"
	// Cyclic-loop escalate-to-opus kill switch + log-not-act holdout. Turning
	// the switch off detaches the escalation ACTION without losing detection
	// telemetry; the holdout % is record-only, giving a self-recovery baseline
	// to subtract from rescue-rate claims. Parsed inline (not parseEnvInt)
	// because 0 is a legitimate "holdout off" value.
	loopEscalationEnabled := config.GetOr("ROUTER_LOOP_ESCALATION_ENABLED", "true") == "true"
	loopEscalationHoldoutPct := 10
	if raw := config.GetOr("ROUTER_LOOP_ESCALATION_HOLDOUT_PCT", ""); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 0 || n > 100 {
			logger.Warn("Invalid env var; using default", "key", "ROUTER_LOOP_ESCALATION_HOLDOUT_PCT", "value", raw, "default", loopEscalationHoldoutPct)
		} else {
			loopEscalationHoldoutPct = n
		}
	}
	// Shadow mode is log-only, so it ships enabled; the switch just sheds the
	// per-turn signal-scan cost if it misbehaves.
	spiralShadowEnabled := config.GetOr("ROUTER_SPIRAL_SHADOW_ENABLED", "true") == "true"
	// Session-level struggle detector, also log-only and shipped enabled.
	// Switch sheds the per-turn check if it misbehaves; exists for symmetry with the spiral detector.
	struggleShadowEnabled := config.GetOr("ROUTER_STRUGGLE_SHADOW_ENABLED", "true") == "true"

	struggleEscalationEnabled := config.GetOr("ROUTER_STRUGGLE_ESCALATION_ENABLED", "false") == "true"
	struggleEscalationHoldoutPct := 50
	if raw := config.GetOr("ROUTER_STRUGGLE_ESCALATION_HOLDOUT_PCT", ""); raw != "" {
		var n int
		if _, scanErr := fmt.Sscanf(raw, "%d", &n); scanErr != nil || n < 0 || n > 100 {
			logger.Warn("Invalid env var; using default", "key", "ROUTER_STRUGGLE_ESCALATION_HOLDOUT_PCT", "value", raw, "default", struggleEscalationHoldoutPct)
		} else {
			struggleEscalationHoldoutPct = n
		}
	}
	var struggleRoster proxy.StruggleEscalationRoster
	// Enforcing text-repetition break ships enabled; the switch is the kill
	// switch if it ever false-positives on legit repeated narration.
	textRepetitionBreakEnabled := config.GetOr("ROUTER_TEXT_REPETITION_BREAK_ENABLED", "true") == "true"
	plannerCfg := planner.EVConfig{
		ThresholdUSD:           parseEnvFloat("ROUTER_SWITCH_EV_THRESHOLD_USD", proxy.DefaultPlannerThresholdUSD),
		ExpectedRemainingTurns: parseEnvInt("ROUTER_SWITCH_EXPECTED_REMAINING_TURNS", proxy.DefaultPlannerExpectedRemainingTurns),
		TierUpgradeEnabled:     config.GetOr("ROUTER_SWITCH_TIER_UPGRADE_ENABLED", boolDefault(proxy.DefaultPlannerTierUpgradeEnabled)) == "true",
		ColdPinFollowFresh:     config.GetOr("ROUTER_SWITCH_COLD_PIN_FOLLOW_FRESH", boolDefault(proxy.DefaultPlannerColdPinFollowFresh)) == "true",
		CorrectedEconomics:     config.GetOr("ROUTER_SWITCH_CORRECTED_ECONOMICS", boolDefault(proxy.DefaultPlannerCorrectedEconomics)) == "true",
	}
	prefixTrimFreeSwitch := config.GetOr("ROUTER_PREFIX_TRIM_FREE_SWITCH", "true") == "true"
	hmmUpgradeConfidence := parseEnvFloat("ROUTER_HMM_UPGRADE_CONFIDENCE_THRESHOLD", 0.85)
	hmmSameTierPin := config.GetOr("ROUTER_HMM_SAME_TIER_PIN", "false") == "true"
	hmPinStickyOnArmSelectorUnavail := config.GetOr("ROUTER_HMM_PIN_STICKY_ON_ARM_SELECTOR_UNAVAIL", "false") == "true"
	// authoritativeUpgradeGate keeps the 0.85 escalation floor active for authoritative-per-turn
	// policies; kill switch for a return to verbatim policy selection.
	authoritativeUpgradeGate := config.GetOr("ROUTER_AUTHORITATIVE_UPGRADE_GATE", "true") == "true"
	// authorityCacheShadow records the HMM cache gate's counterfactual verdict on
	// authoritative-per-turn turns, which return before that gate can run. Pure
	// observation; kill switch for the added per-turn computation and log line.
	authorityCacheShadow := config.GetOr("ROUTER_AUTHORITY_CACHE_SHADOW", "true") == "true"
	// policyDeadlineFallback degrades a policy sidecar deadline/transport failure to
	// the session pin (or tier-3 default below) instead of a 503. Kill switch; off by default.
	policyDeadlineFallback := config.GetOr("ROUTER_POLICY_DEADLINE_FALLBACK", "false") == "true"
	// policyDeadlineDefaultModel is the tier-3 static fallback on a deadline miss with no pin; empty = fail-closed.
	policyDeadlineDefaultModel := config.GetOr("ROUTER_POLICY_DEADLINE_DEFAULT_MODEL", "")
	handoverProviderName := config.GetOr("ROUTER_HANDOVER_PROVIDER", providers.ProviderAiand)
	handoverModel := config.GetOr("ROUTER_HANDOVER_MODEL", "deepseek-ai/deepseek-v4-flash")
	handoverTimeout := parseEnvDurationMs("ROUTER_HANDOVER_TIMEOUT_MS", proxy.DefaultHandoverTimeout)
	// Kept as the interface type: a typed-nil *ProviderSummarizer would defeat
	// the orchestrator's `!= nil` check.
	var summarizer handover.Summarizer
	// compactionSz stays a true-nil interface unless the summarizer provider is
	// registered, so the Service's nil-check disables Tier-3 correctly (a
	// typed-nil concrete pointer would defeat it).
	var compactionSz proxy.CompactionSummarizer
	if client, ok := providerMap[handoverProviderName]; ok {
		ps := proxy.NewProviderSummarizer(client, handoverProviderName, handoverModel, handoverTimeout)
		summarizer = ps
		compactionSz = ps
		logger.Info("Handover summarizer wired", "provider", handoverProviderName, "model", handoverModel, "timeout_ms", handoverTimeout.Milliseconds())
	} else {
		logger.Info("Handover summarizer disabled (provider not registered); switch turns will preserve full history instead", "requested_provider", handoverProviderName)
	}
	compactionPct := parseEnvFloat("ROUTER_COMPACTION_PCT", proxy.DefaultCompactionTriggerPct)

	// Strategy-specific artifacts own selection membership; the legacy cluster bundle must not constrain HMM candidates.
	routingTargets := catalog.RoutingTargetSet(availableProviders)
	logger.Info("Catalog routing targets resolved", "catalog_routing_targets", len(routingTargets))

	// OFF by default: wraps only the proxy's routing entrypoint, so rtr stays
	// the *cluster.Multiversion the admin cast and semantic cache reference.
	routeEntry := buildExploringRouter(rtr, logger)

	// Off unless WV_CAPTURE_CONTENT is set; managed deploys set "full". Redactor
	// is nil (no-op) here — managed wires one.
	captureMode := proxy.ParseCaptureMode(config.GetOr("WV_CAPTURE_CONTENT", ""))
	captureMaxBytes := parseEnvInt("WV_CAPTURE_MAX_BYTES", 1<<20)
	logger.Info("Router content capture configured", "mode", captureMode.String(), "max_bytes", captureMaxBytes)
	logger.Info("Feedback HTTP pages removed; ROUTER_FEEDBACK_* env vars are ignored (no /v1/feedback routes or link headers)")

	// Wired only when ROUTER_RL_SIDECAR_URL is set; x-weave-router-strategy: rl
	// then routes through it. Unset fails closed with 503 rather than
	// silently falling back to the cluster scorer.
	var rlRouter router.Router
	if rlSidecarURL := config.GetOr("ROUTER_RL_SIDECAR_URL", ""); rlSidecarURL != "" {
		rlTimeout := parseEnvDurationMs("ROUTER_RL_SIDECAR_TIMEOUT_MS", rl.DefaultTimeout)
		rlHeaders := rlSidecarHeadersFromEnv()
		rlRouter = rl.New(
			rl.NewHTTPDeciderWithHeaders(rlSidecarURL, nil, rlTimeout, rlHeaders),
			routingTargets,
			availableProviders,
		)
		logger.Info(
			"RL policy router wired",
			"sidecar_url", rlSidecarURL,
			"timeout_ms", rlTimeout.Milliseconds(),
			"candidate_models", len(routingTargets),
			"modal_proxy_auth", len(rlHeaders) > 0,
		)
	} else {
		logger.Info("RL policy router disabled (ROUTER_RL_SIDECAR_URL unset); x-weave-router-strategy: rl will return 503")
	}

	// Wired only when ROUTER_HMM_SIDECAR_URL is set; x-weave-router-strategy:
	// hmm then routes through it. Unset fails closed with 503.
	var hmmRouter router.Router
	var hmmEmbeddingRouter router.Router
	var hmmCapabilities policy.Capabilities
	var hmmReadinessChecker admin.HealthChecker
	var hmmRosterSource policy.RosterSource
	var hmmRosterModels admin.HMMRosterSource
	if hmmSidecarURL := config.GetOr("ROUTER_HMM_SIDECAR_URL", ""); hmmSidecarURL != "" {
		hmmTimeout := parseEnvDurationMs("ROUTER_HMM_SIDECAR_TIMEOUT_MS", policyclient.DefaultTimeout)
		hmmAuthMode := config.GetOr("ROUTER_HMM_SIDECAR_AUTH", policySidecarAuthNone)
		hmmAttemptTimeout := parseEnvAttemptTimeoutMs(
			"ROUTER_HMM_SIDECAR_ATTEMPT_TIMEOUT_MS",
			policyclient.DeriveAttemptTimeout(hmmTimeout),
		)
		hmmClient, clientErr := buildHMMPolicyClient(
			hmmSidecarURL,
			hmmAuthMode,
			hmmTimeout,
			policyclient.WithAttemptTimeout(hmmAttemptTimeout),
		)
		if clientErr != nil {
			logger.Error("HMM policy sidecar client failed to build; refusing to boot", "auth_mode", hmmAuthMode, "err", clientErr)
			panic(clientErr)
		}
		hmmReadinessChecker = hmmClient
		hmmRosterSource = hmmClient
		hmmRosterModels = newHMMRosterSource(hmmClient, hmmTimeout)
		struggleRoster = proxy.NewStruggleRoster(hmmRosterSource)
		capabilityCtx, cancelCapabilityDiscovery := context.WithTimeout(context.Background(), hmmTimeout)
		var capabilityErr error
		hmmCapabilities, capabilityErr = hmmClient.Capabilities(capabilityCtx)
		cancelCapabilityDiscovery()
		if capabilityErr != nil {
			logger.Warn("HMM policy sidecar capabilities unavailable at boot; optional behavior remains disabled", "sidecar_url", hmmSidecarURL, "err", capabilityErr)
		}
		hmmPolicyRouter := hmm.New(hmmClient, availableProviders)
		hmmPolicyRouter.WithCapabilities(hmmCapabilities)
		hmmEmbeddingPolicyRouter := hmm.NewForStrategy(
			router.StrategyHMMEmbedding,
			hmmClient,
			availableProviders,
		)
		hmmEmbeddingPolicyRouter.WithCapabilities(hmmCapabilities)
		if capabilityErr != nil {
			go func() {
				retryErr := retryPolicyCapabilitiesUntilAvailable(
					context.Background(),
					hmmClient,
					hmmTimeout,
					hmmCapabilityRetryInterval,
					func(capabilities policy.Capabilities) {
						hmmPolicyRouter.WithCapabilities(capabilities)
						hmmEmbeddingPolicyRouter.WithCapabilities(capabilities)
					},
				)
				if retryErr != nil {
					logger.Warn("HMM policy sidecar capability refresh stopped", "sidecar_url", hmmSidecarURL, "err", retryErr)
					return
				}
				logger.Info("HMM policy sidecar capabilities discovered after boot", "sidecar_url", hmmSidecarURL)
			}()
		}
		hmmRouter = hmmPolicyRouter
		hmmEmbeddingRouter = hmmEmbeddingPolicyRouter
		logger.Info(
			"HMM policy routers wired",
			"sidecar_url", hmmSidecarURL,
			"auth_mode", hmmAuthMode,
			"timeout_ms", hmmTimeout.Milliseconds(),
			"attempt_timeout_ms", hmmAttemptTimeout.Milliseconds(),
			"candidate_models", len(routingTargets),
			"strategies", []router.Strategy{router.StrategyHMM, router.StrategyHMMEmbedding},
		)
	} else {
		logger.Info("HMM policy routers disabled (ROUTER_HMM_SIDECAR_URL unset); HMM strategies will return 503")
	}

	// Wired only when ROUTER_BANDIT_POSTERIOR_FILE points at a ts_posterior.json;
	// x-weave-router-strategy: bandit then routes through it. Wraps the raw
	// cluster scorer, not the explore wrapper. Unset -> nil -> 503.
	var banditRouter router.Router
	if posteriorPath := strings.TrimSpace(config.GetOr("ROUTER_BANDIT_POSTERIOR_FILE", "")); posteriorPath != "" {
		post, loadErr := bandit.LoadPosterior(posteriorPath)
		if loadErr != nil {
			logger.Error(
				"Bandit posterior load failed; x-weave-router-strategy: bandit will return 503",
				"path", posteriorPath,
				"err", loadErr,
			)
		} else {
			banditRouter = bandit.New(rtr, post)
			logger.Info("Bandit strategy router wired", "posterior_file", posteriorPath)
		}
	} else {
		logger.Info("Bandit strategy router disabled (ROUTER_BANDIT_POSTERIOR_FILE unset); x-weave-router-strategy: bandit will return 503")
	}

	configuredPolicySpecs, err := buildConfiguredPolicySidecars(
		context.Background(),
		config.GetOr("ROUTER_POLICY_SIDECARS", ""),
		config.GetOr("ROUTER_POLICY_SIDECAR_AUTH", ""),
		parseEnvDurationMs("ROUTER_POLICY_SIDECAR_TIMEOUT_MS", policyclient.DefaultTimeout),
		routingTargets,
		availableProviders,
		nil,
		logger,
	)
	if err != nil {
		logger.Error("Generic policy sidecar configuration is invalid", "err", err)
		panic(err)
	}

	// Publish the compiled-in flag registry, stamped with the defaults this
	// process actually resolved, so the Weave control plane can render and
	// validate the per-org override UI without importing router code. Best
	// effort: the table is never read on the request path, so a failure here
	// degrades that UI and nothing else.
	publishFlagRegistry(logger, repo.FlagDefinitions, map[flags.Key]string{
		flags.KeyStruggleShadowEnabled:     boolDefault(struggleShadowEnabled),
		flags.KeyStruggleEscalationEnabled: boolDefault(struggleEscalationEnabled),
		flags.KeyStruggleEscalationHoldout: strconv.Itoa(struggleEscalationHoldoutPct),
		flags.KeySpiralShadowEnabled:       boolDefault(spiralShadowEnabled),
		flags.KeyLoopEscalationEnabled:     boolDefault(loopEscalationEnabled),
		flags.KeyLoopEscalationHoldoutPct:  strconv.Itoa(loopEscalationHoldoutPct),
		flags.KeyTextRepetitionBreak:       boolDefault(textRepetitionBreakEnabled),
		flags.KeyPlannerEnabled:            boolDefault(plannerEnabled),
		flags.KeyScoreToolResultTurns:      boolDefault(scoreToolResultTurns),
		flags.KeyPrefixTrimFreeSwitch:      boolDefault(prefixTrimFreeSwitch),
		flags.KeyAuthoritativeUpgradeGate:  boolDefault(authoritativeUpgradeGate),
		flags.KeyAuthorityCacheShadow:      boolDefault(authorityCacheShadow),
		flags.KeySiblingFailover:           boolDefault(siblingFailover),
		flags.KeyEffortEscalation:          boolDefault(effortEscalation),
		flags.KeyCyberRefusalRepin:         boolDefault(cyberRefusalRepin),
		flags.KeyCyberRefusalFallback:      cyberRefusalFallbackModel,
		flags.KeyEmbedOnlyUserMessage:      boolDefault(embedOnlyUser),
	})

	proxySvc := proxy.NewService(routeEntry, providerMap, telemetryEmitter, embedOnlyUser, semanticCache, pinStore, hardPinExplore, hardPinProvider, hardPinModel, repo.Telemetry).
		WithScopedSearchRequirement(scopedSearchRequirement, searchRequirementDecayTurns).
		WithTranslationCompatibilityMode(proxy.TranslationCompatibilityMode(translationCompatibilityMode)).
		WithPolicyStrategy(policy.StrategySpec{Strategy: router.StrategyRL, Router: rlRouter, Unavailable: rl.ErrPolicyUnavailable}).
		WithPolicyStrategy(policy.StrategySpec{
			Strategy: router.StrategyHMM, Router: hmmRouter, Unavailable: hmm.ErrHMMUnavailable,
			Capabilities: hmmCapabilities,
		}).
		WithPolicyStrategy(policy.StrategySpec{
			Strategy: router.StrategyHMMEmbedding, Router: hmmEmbeddingRouter, Unavailable: hmm.ErrHMMUnavailable,
			Capabilities: hmmCapabilities,
		}).
		WithPolicyStrategy(policy.StrategySpec{Strategy: router.StrategyBandit, Router: banditRouter, Unavailable: bandit.ErrBanditUnavailable}).
		WithContentCapture(captureMode, captureMaxBytes, nil).
		WithFeedback(repo.Feedback, nil, "").
		WithByokOnly(byokOnly).
		WithDeploymentKeyedProviders(deploymentEligible).
		WithHardPinResolver(hardPinResolver).
		WithSubAgentOverride(subAgentProvider, subAgentModel).
		WithPlannerEnabled(plannerEnabled).
		WithScoreToolResultTurns(scoreToolResultTurns).
		WithCyberRefusalRepin(cyberRefusalRepin).
		WithCyberRefusalFallbackModel(cyberRefusalFallbackModel).
		WithSiblingFailover(siblingFailover).
		WithSSEKeepalive(sseKeepalive).
		WithPrefixTrimFreeSwitch(prefixTrimFreeSwitch).
		WithHMMUpgradeConfidenceThreshold(hmmUpgradeConfidence).
		WithHMMSameTierPin(hmmSameTierPin).
		WithHMPinStickyOnArmSelectorUnavail(hmPinStickyOnArmSelectorUnavail).
		WithAuthoritativeUpgradeGate(authoritativeUpgradeGate).
		WithAuthorityCacheShadow(authorityCacheShadow).
		WithPolicyDeadlineFallback(policyDeadlineFallback).
		WithPolicyDeadlineDefaultModel(policyDeadlineDefaultModel).
		WithEscapeNormalize(escapeNormalize).
		WithEffortEscalation(effortEscalation).
		WithCCOrchestrationToolsCrossVendor(ccOrchToolsCrossVendor).
		WithBandSwap(bandSwapEnabled).
		WithLoopEscalationConfig(loopEscalationEnabled, loopEscalationHoldoutPct).
		WithLoopEscalationStore(repo.Telemetry).
		WithSpiralShadowConfig(spiralShadowEnabled).
		WithSpiralShadowStore(repo.Telemetry).
		WithStruggleShadowConfig(struggleShadowEnabled).
		WithStruggleShadowStore(repo.Telemetry).
		WithStruggleEscalationConfig(struggleEscalationEnabled, struggleEscalationHoldoutPct).
		WithStruggleEscalationStore(repo.Telemetry).
		WithStruggleEscalationRoster(struggleRoster).
		WithTextRepetitionBreak(textRepetitionBreakEnabled).
		WithRouterFeedbackStore(repo.Telemetry).
		WithPlanner(plannerCfg).
		WithSummarizer(summarizer).
		WithCompaction(compactionSz, compactionPct).
		WithAvailableModels(proxyRoutableModels(routingTargets, availableProviders, hmmRouter != nil)).
		WithDefaultBaselineModel(resolveDefaultBaselineModel()).
		WithBillingService(billingSvc)
	for _, spec := range configuredPolicySpecs {
		proxySvc = proxySvc.WithPolicyStrategy(spec)
		logger.Info("Generic policy sidecar wired", "strategy", spec.Strategy, "candidate_models", len(routingTargets))
	}
	logger.Info("Effort escalation configured", "enabled", effortEscalation)
	logger.Info("Cross-vendor Claude Code orchestration tools configured", "enabled", ccOrchToolsCrossVendor)
	logger.Info("Loop escalation configured", "enabled", loopEscalationEnabled, "holdout_pct", loopEscalationHoldoutPct)
	logger.Info("Spiral shadow detector configured", "enabled", spiralShadowEnabled)
	logger.Info("Text-repetition break configured", "enabled", textRepetitionBreakEnabled)
	logger.Info("Planner configured", "enabled", plannerEnabled, "threshold_usd", plannerCfg.ThresholdUSD, "expected_remaining_turns", plannerCfg.ExpectedRemainingTurns, "tier_upgrade_enabled", plannerCfg.TierUpgradeEnabled, "cold_pin_follow_fresh", plannerCfg.ColdPinFollowFresh, "corrected_economics", plannerCfg.CorrectedEconomics, "prefix_trim_free_switch", prefixTrimFreeSwitch, "routing_targets_count", len(routingTargets))
	logger.Info("Tool-result scoring configured", "enabled", scoreToolResultTurns)

	// Fail loud if a deployed model is missing from the tier table;
	// TierUnknown would silently disable the guard for that pair.
	if plannerCfg.TierUpgradeEnabled && len(routingTargets) > 0 {
		deployed := make([]string, 0, len(routingTargets))
		for m := range routingTargets {
			deployed = append(deployed, m)
		}
		if err := catalog.ValidateDeployed(deployed); err != nil {
			logger.Error("Capability tier table incomplete; refusing to start with tier guard enabled", "err", err)
			panic(err)
		}
	}

	// ROUTER_EXCLUDED_MODELS pins a deployment-wide model exclusion list,
	// overriding per-installation DB state. Empty / unset → DB takes over.
	if excludedRaw := strings.TrimSpace(config.GetOr("ROUTER_EXCLUDED_MODELS", "")); excludedRaw != "" {
		parts := strings.Split(excludedRaw, ",")
		cleaned := make([]string, 0, len(parts))
		for _, p := range parts {
			if trimmed := strings.TrimSpace(p); trimmed != "" {
				cleaned = append(cleaned, trimmed)
			}
		}
		proxySvc = proxySvc.WithExcludedModelsOverride(cleaned)
		logger.Info("Model exclusion override active", "excluded_models", cleaned)
	}

	// ROUTER_EXCLUDED_PROVIDERS pins a deployment-wide provider exclusion
	// list, overriding per-installation DB state. Empty / unset → DB takes over.
	if excludedRaw := strings.TrimSpace(config.GetOr("ROUTER_EXCLUDED_PROVIDERS", "")); excludedRaw != "" {
		parts := strings.Split(excludedRaw, ",")
		cleaned := make([]string, 0, len(parts))
		for _, p := range parts {
			if trimmed := strings.TrimSpace(p); trimmed != "" {
				cleaned = append(cleaned, trimmed)
			}
		}
		proxySvc = proxySvc.WithExcludedProvidersOverride(cleaned)
		logger.Info("Provider exclusion override active", "excluded_providers", cleaned)
	}

	// The usage observer is always wired (cheap, side-effect-free) even though
	// the cost discount below is env-gated: it also feeds the per-installation
	// usage-bypass gate, which is DB-gated and can't know the env flag's state.
	subscriptionTTL := 10 * time.Minute
	if v, err := time.ParseDuration(config.GetOr("ROUTER_SUBSCRIPTION_OBSERVATION_TTL", "10m")); err == nil {
		subscriptionTTL = v
	}
	observerSalt := make([]byte, 16)
	if _, err := rand.Read(observerSalt); err != nil {
		logger.Error("Failed to seed subscription usage observer salt", "err", err)
		panic(err)
	}
	usageObserver := usage.NewObserver(observerSalt, subscriptionTTL, time.Now)
	// Bound memory: evict expired observations periodically (the usage package
	// spawns no goroutines of its own).
	go func() {
		t := time.NewTicker(time.Minute)
		defer t.Stop()
		for range t.C {
			usageObserver.Sweep()
		}
	}()
	proxySvc = proxySvc.WithUsageObserver(usageObserver)

	// Discounts a covered model's cost term by the caller's observed
	// subscription rate-limit headroom (~epsilon with slack, →1 as it binds).
	// Defaults ON; only affects turns with an observed subscription, so
	// blast radius is narrow. Disabling here leaves the observer/bypass gate wired.
	if config.GetOr("ROUTER_SUBSCRIPTION_AWARE_ROUTING", "true") == "true" {
		epsilon := 0.05
		if v, err := strconv.ParseFloat(config.GetOr("ROUTER_SUBSCRIPTION_COST_EPSILON", "0.05"), 64); err == nil {
			epsilon = v
		}
		gamma := 2.0
		if v, err := strconv.ParseFloat(config.GetOr("ROUTER_SUBSCRIPTION_COST_GAMMA", "2"), 64); err == nil {
			gamma = v
		}
		proxySvc = proxySvc.WithSubscriptionAwareRouting(usageObserver, epsilon, gamma)
		logger.Info("Subscription-aware routing configured", "epsilon", epsilon, "gamma", gamma, "observation_ttl", subscriptionTTL)
	} else {
		logger.Info("Usage observer wired; subscription-aware cost discount disabled", "observation_ttl", subscriptionTTL)
	}

	// No-op when WV_APM_OTLP_ENDPOINT is unset. Flushed explicitly in the
	// shutdown path below since a defer would run after SIGKILL.
	apm.Init()

	engine := gin.New()
	engine.UnescapePathValues = true
	engine.UseRawPath = true
	engine.Use(
		observability.Middleware(),
		observability.AccessLog(),
		apm.Middleware(),
		gin.Recovery(),
	)

	// Lets the admin model-selection handler surface deployed models; nil
	// fallback keeps non-cluster routers bootable.
	deployedModels, _ := rtr.(*cluster.Multiversion)
	if deployedModels != nil {
		proxySvc = proxySvc.WithLocalModelList(func() []string {
			entries := deployedModels.DefaultDeployedModels()
			ids := make([]string, 0, len(entries))
			for _, e := range entries {
				ids = append(ids, e.Model)
			}
			return ids
		})
	}
	analyticsSvc := analytics.NewService(repo.Analytics, time.Now)
	server.Register(engine, server.Services{
		Auth:                authSvc,
		Proxy:               proxySvc,
		DeployedModels:      deployedModels,
		HMMModels:           hmmRosterModels,
		Billing:             billingSvc,
		ReadinessChecker:    hmmReadinessChecker,
		HMMRosterSource:     hmmRosterSource,
		Analytics:           analyticsSvc,
		AiandCatalogHandler: aiandCatalogHandler,
	}, deploymentMode)

	srv := &http.Server{
		Addr:    ":" + config.GetOr("PORT", "8080"),
		Handler: engine,
		// ReadTimeout/WriteTimeout would break streaming; per-route gin
		// timeouts handle non-streaming routes instead.
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}

	providerNames := make([]string, 0, len(providerMap))
	for name := range providerMap {
		providerNames = append(providerNames, name)
	}
	logger.Info("Starting Weave router", "providers", providerNames, "addr", srv.Addr)

	serverErr := make(chan error, 1)
	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-serverErr:
		logger.Error("Server exited with error", "err", err)
		// A ListenAndServe failure bypasses the SIGTERM path below, so flush
		// APM here too or the traces describing the failure never reach SigNoz.
		apmFailCtx, apmFailCancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
		defer apmFailCancel()
		apm.ShutdownWithContext(apmFailCtx)
		return
	case sig := <-stop:
		logger.Info("Received shutdown signal; draining", "signal", sig.String())
	}

	// Cloud Run gives 10s between SIGTERM and SIGKILL; budget across three
	// flush stages (defer on apm.Shutdown would never run in time):
	//   srv.Shutdown 6.0s + emitter.Shutdown 1.5s + apm.Shutdown 1.5s = 9.0s,
	//   leaving ~1s slack.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("Graceful shutdown failed", "err", err)
	}
	emitterCtx, emitterCancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer emitterCancel()
	if err := emitter.Shutdown(emitterCtx); err != nil {
		logger.Warn("OTel emitter shutdown incomplete", "err", err)
	}
	apmCtx, apmCancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer apmCancel()
	apm.ShutdownWithContext(apmCtx)
}

// buildExploringRouter optionally wraps rtr in the quality-tie-band explorer.
// OFF unless ROUTER_EXPLORE_ENABLED=true (band half-width ROUTER_EXPLORE_EPSILON,
// default 0.05). Intended for staging/small slices — never flip fleet-wide
// without a bake-off.
func buildExploringRouter(rtr router.Router, logger *slog.Logger) router.Router {
	if !strings.EqualFold(config.GetOr("ROUTER_EXPLORE_ENABLED", "false"), "true") {
		return rtr
	}
	multi, ok := rtr.(*cluster.Multiversion)
	if !ok {
		logger.Warn("ROUTER_EXPLORE_ENABLED set but router is not a cluster.Multiversion; exploration disabled")
		return rtr
	}

	providers := make(map[string]string)
	for _, e := range multi.DefaultDeployedModels() {
		// First binding wins; the default bundle lists a model's primary
		// provider first, which matches the scorer's default resolution.
		if _, seen := providers[e.Model]; !seen {
			providers[e.Model] = e.Provider
		}
	}
	providerFor := func(model string) (string, bool) {
		p, ok := providers[model]
		return p, ok
	}

	epsilon := float32(parseEnvFloat("ROUTER_EXPLORE_EPSILON", 0.05))
	logger.Info(
		"Quality-tie-band exploration ENABLED (non-prod / shadow expected)",
		"band_width", epsilon,
		"deployed_models", len(providers),
	)
	return banditexplore.New(rtr, providerFor, epsilon)
}

// parseOtelHeaders parses a comma-separated key=value string into a map.
func parseOtelHeaders(raw string) map[string]string {
	if raw == "" {
		return nil
	}
	out := make(map[string]string)
	for _, pair := range strings.Split(raw, ",") {
		k, v, ok := strings.Cut(strings.TrimSpace(pair), "=")
		if ok && k != "" {
			out[k] = v
		}
	}
	return out
}

// buildClusterScorer constructs the cluster.Multiversion router, sharing one
// ONNX embedder across versions. Errors force the caller to panic rather
// than silently degrade to a default model. Also returns the default
// version's embedder ID so the caller can log the resolved value rather than
// a hardcoded literal.
func buildClusterScorer(availableProviders map[string]struct{}) (router.Router, string, error) {
	logger := observability.Get()

	requestedVersion := config.GetOr("ROUTER_CLUSTER_VERSION", cluster.LatestVersion)
	defaultVersion, err := cluster.ResolveVersion(requestedVersion)
	if err != nil {
		return nil, "", fmt.Errorf("Resolve cluster version %q: %w", requestedVersion, err)
	}

	// Builds only the served version by default. ROUTER_CLUSTER_BUILD_ALL_VERSIONS=true
	// powers the eval harness's per-request x-weave-cluster-version A/B; prod
	// skips it to avoid the extra memory and the header-override foot-gun.
	buildAll := strings.EqualFold(config.GetOr("ROUTER_CLUSTER_BUILD_ALL_VERSIONS", "false"), "true")
	var versions []string
	if buildAll {
		versions, err = cluster.ListVersions()
		if err != nil {
			return nil, "", fmt.Errorf("List cluster versions: %w", err)
		}
	} else {
		versions = []string{defaultVersion}
	}

	embedders, err := cluster.NewEmbedderSet()
	if err != nil {
		return nil, "", err
	}

	cfg := cluster.DefaultConfig()
	if v := config.GetOr("ROUTER_CLUSTER_EMBED_TIMEOUT_MS", ""); v != "" {
		ms, parseErr := time.ParseDuration(v + "ms")
		if parseErr != nil || ms <= 0 {
			logger.Warn("Invalid ROUTER_CLUSTER_EMBED_TIMEOUT_MS; using default", "value", v, "default_ms", cfg.EmbedTimeout.Milliseconds())
		} else {
			cfg.EmbedTimeout = ms
			logger.Info("Cluster embed timeout overridden", "embed_timeout_ms", ms.Milliseconds())
		}
	}
	if v := config.GetOr("ROUTER_CLUSTER_TOP_P", ""); v != "" {
		n, parseErr := strconv.Atoi(v)
		if parseErr != nil || n <= 0 {
			logger.Warn("Invalid ROUTER_CLUSTER_TOP_P; using default", "value", v, "default", cfg.TopP)
		} else {
			cfg.TopP = n
			logger.Info("Cluster top_p overridden", "top_p", n)
		}
	}
	scorers := make(map[string]*cluster.Scorer, len(versions))
	warmed := make(map[string]cluster.Embedder)
	var defaultEmbedderID string
	for _, v := range versions {
		bundle, err := cluster.LoadBundle(v)
		if err != nil {
			_ = embedders.Close()
			return nil, "", fmt.Errorf("Load bundle %s: %w", v, err)
		}
		if v == defaultVersion {
			defaultEmbedderID = bundle.EmbedderID()
		}

		missingProviders := map[string][]string{}
		for _, e := range bundle.Registry.DeployedModels {
			if _, ok := availableProviders[e.Provider]; !ok {
				missingProviders[e.Provider] = append(missingProviders[e.Provider], e.Model)
			}
		}
		for prov, models := range missingProviders {
			logger.Warn(
				"Cluster artifact references unregistered provider; affected models will be excluded from argmax",
				"cluster_version", v,
				"missing_provider", prov,
				"affected_models", models,
				"hint", "set "+envVarHint(prov)+" to keep these in the routing pool",
			)
		}

		// Lazily construct only the embedders the built versions need:
		// prod (single default version) loads exactly one model.
		embedder, err := embedders.Get(bundle.EmbedderID())
		if err != nil {
			// The default version's embedder must construct; sibling
			// versions degrade like any other per-version build failure.
			if v == defaultVersion {
				_ = embedders.Close()
				return nil, "", fmt.Errorf("Construct embedder %q for default cluster version %s: %w", bundle.EmbedderID(), v, err)
			}
			logger.Warn("Cluster scorer version skipped; embedder unavailable", "cluster_version", v, "embedder", bundle.EmbedderID(), "err", err)
			continue
		}
		warmed[embedder.ID()] = embedder

		scorer, err := cluster.NewScorer(bundle, cfg, embedder, availableProviders)
		if err != nil {
			logger.Warn("Cluster scorer version skipped", "cluster_version", v, "err", err)
			continue
		}
		scorers[v] = scorer
		logger.Info("Cluster scorer version built", "cluster_version", v, "embedder", bundle.EmbedderID(), "models", bundle.Registry.Models())
	}

	if _, ok := scorers[defaultVersion]; !ok {
		_ = embedders.Close()
		return nil, "", fmt.Errorf("Default cluster version %q failed to build (likely no registered provider covers its deployed_models); set ROUTER_CLUSTER_VERSION to a version that does, or register the missing provider key", defaultVersion)
	}

	multi, err := cluster.NewMultiversion(defaultVersion, scorers)
	if err != nil {
		_ = embedders.Close()
		return nil, "", fmt.Errorf("Build multiversion router: %w", err)
	}
	logger.Info(
		"Cluster multiversion router ready",
		"default_version", defaultVersion,
		"built_versions", multi.Built(),
		"built_embedders", embedders.Built(),
		"requested_version", requestedVersion,
		"build_all_versions", buildAll,
	)

	// Warmup: burn each embedder's lazy ONNX graph-optimization cost at boot.
	for id, embedder := range warmed {
		type warmupResult struct {
			err error
		}
		warmupDone := make(chan warmupResult, 1)
		go func(e cluster.Embedder) {
			_, err := e.Embed(context.Background(), "warmup")
			warmupDone <- warmupResult{err: err}
		}(embedder)
		select {
		case res := <-warmupDone:
			if res.err != nil {
				_ = embedders.Close()
				return nil, "", fmt.Errorf("Warm embedder %q: %w", id, res.err)
			}
		case <-time.After(15 * time.Second):
			// Drain the goroutine before closing to avoid use-after-free.
			go func() {
				<-warmupDone
				_ = embedders.Close()
			}()
			return nil, "", fmt.Errorf("Cluster embedder %q warmup timed out after 15s", id)
		}
		logger.Info("Cluster embedder warmed", "embedder", id, "embed_dim", embedder.Dim())
	}

	return multi, defaultEmbedderID, nil
}

// buildWIFTokenSource constructs the workload attestation source backing BYOK
// keys with auth_type=wif. Returns nil when ROUTER_WIF_PROVIDER is unset — the
// identity is the deployment's own, so only the operator can configure it.
func buildWIFTokenSource(logger *slog.Logger) auth.WIFTokenSource {
	provider := strings.ToUpper(strings.TrimSpace(config.GetOr("ROUTER_WIF_PROVIDER", "")))
	audience := config.GetOr("ROUTER_WIF_AUDIENCE", auth.WIFAudience)
	switch provider {
	case "":
		return nil
	case auth.WIFProviderGCP:
		logger.Info("Workload identity federation enabled", "provider", provider, "audience", audience)
		return wif.NewGoogleTokenSource(audience)
	case auth.WIFProviderOIDC:
		path := config.GetOr("ROUTER_WIF_OIDC_TOKEN_FILE", "")
		if path == "" {
			err := fmt.Errorf("ROUTER_WIF_PROVIDER=%s requires ROUTER_WIF_OIDC_TOKEN_FILE", provider)
			logger.Error("Refusing to boot with incomplete workload identity configuration", "err", err)
			panic(err)
		}
		logger.Info("Workload identity federation enabled", "provider", provider, "token_file", path)
		return wif.NewFileTokenSource(path)
	default:
		err := fmt.Errorf("Invalid ROUTER_WIF_PROVIDER %q (expected %q or %q)", provider, auth.WIFProviderGCP, auth.WIFProviderOIDC)
		logger.Error("Refusing to boot with invalid workload identity provider", "err", err)
		panic(err)
	}
}

// buildOtelEmitter constructs the OTel span emitter from environment
// variables. Returns (nil, nil) when OTEL_EXPORTER_OTLP_ENDPOINT is unset.
func buildOtelEmitter(deploymentMode string) (*otel.Emitter, error) {
	logger := observability.Get()

	endpoint := config.GetOr("OTEL_EXPORTER_OTLP_ENDPOINT", "")
	if endpoint == "" {
		return nil, nil
	}

	// router.deployment_mode lets the collector branch ingest behavior
	// (e.g. redaction / content-opt-out) without inspecting per-record attrs.
	resourceAttrs := parseOtelHeaders(config.GetOr("OTEL_RESOURCE_ATTRIBUTES", ""))
	if resourceAttrs == nil {
		resourceAttrs = map[string]string{}
	}
	resourceAttrs["router.deployment_mode"] = deploymentMode

	cfg := otel.EmitterConfig{
		Endpoint:      endpoint,
		Headers:       parseOtelHeaders(config.GetOr("OTEL_EXPORTER_OTLP_HEADERS", "")),
		ServiceName:   config.GetOr("OTEL_SERVICE_NAME", "router"),
		ResourceAttrs: resourceAttrs,
		Workers:       parseEnvInt("OTEL_EXPORT_WORKERS", 2),
		QueueSize:     parseEnvInt("OTEL_BSP_MAX_QUEUE_SIZE", 1000),
		BatchSize:     parseEnvInt("OTEL_BSP_MAX_EXPORT_BATCH_SIZE", 50),
		FlushInterval: parseEnvDurationMs("OTEL_BSP_SCHEDULE_DELAY", 500*time.Millisecond),
		ExportTimeout: parseEnvDurationMs("OTEL_EXPORTER_OTLP_TIMEOUT", 10*time.Second),
	}

	emitter, err := otel.NewEmitter(cfg)
	if err != nil {
		return nil, err
	}

	safeEndpoint := endpoint
	if u, err := url.Parse(endpoint); err == nil {
		u.User = nil
		u.RawQuery = ""
		u.Fragment = ""
		safeEndpoint = u.String()
	}
	logger.Info("OTel export enabled",
		"endpoint", safeEndpoint,
		"workers", cfg.Workers,
		"queue_size", cfg.QueueSize,
		"batch_size", cfg.BatchSize,
		"flush_interval_ms", cfg.FlushInterval.Milliseconds(),
		"export_timeout_ms", cfg.ExportTimeout.Milliseconds(),
	)
	return emitter, nil
}

// parseEnvInt reads an env var as a positive integer. Returns fallback when
// the var is unset, empty, or unparseable. Logs a warning on bad values.
func parseEnvInt(key string, fallback int) int {
	raw := config.GetOr(key, "")
	if raw == "" {
		return fallback
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		observability.Get().Warn("Invalid env var; using default", "key", key, "value", raw, "default", fallback)
		return fallback
	}
	return n
}

// parseEnvFloat reads an env var as a float64, falling back on unset/empty/
// unparseable. Zero and negative values are valid — e.g. operators set
// ROUTER_SWITCH_EV_THRESHOLD_USD <= 0 to force aggressive planner switching.
// proxyRoutableModels is RoutingTargetSet plus HMM-only rows when HMM is wired.
func proxyRoutableModels(generic, providers map[string]struct{}, hmmWired bool) map[string]struct{} {
	if !hmmWired {
		return generic
	}
	out := catalog.HMMRoutingTargetSet(providers)
	if len(generic) == 0 {
		return out
	}
	for m := range generic {
		out[m] = struct{}{}
	}
	return out
}

func parseEnvFloat(key string, fallback float64) float64 {
	raw := config.GetOr(key, "")
	if raw == "" {
		return fallback
	}
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		observability.Get().Warn("Invalid env var; using default", "key", key, "value", raw, "default", fallback)
		return fallback
	}
	return v
}

// sseKeepaliveInterval parses ROUTER_SSE_KEEPALIVE_INTERVAL_SECONDS.
// Handled inline because 0 is a valid kill-switch value that parseEnvInt rejects.
func sseKeepaliveInterval() time.Duration {
	const defaultSec = 15
	raw := config.GetOr("ROUTER_SSE_KEEPALIVE_INTERVAL_SECONDS", "")
	if raw == "" {
		return defaultSec * time.Second
	}
	sec, err := strconv.Atoi(raw)
	if err != nil || sec < 0 {
		observability.Get().Warn("Invalid env var; using default", "key", "ROUTER_SSE_KEEPALIVE_INTERVAL_SECONDS", "value", raw, "default", defaultSec)
		return defaultSec * time.Second
	}
	return time.Duration(sec) * time.Second
}

// boolDefault renders a bool default for config.GetOr on bool envs.
func boolDefault(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

// rlSidecarHeadersFromEnv builds auth headers for the RL sidecar when both
// ROUTER_RL_SIDECAR_MODAL_KEY and ROUTER_RL_SIDECAR_MODAL_SECRET are set.
//
// Sends X-Weave-Band-Key / X-Weave-Band-Secret (and Authorization Bearer) for
// the Modal tip_k3 band serve — Modal's edge strips Modal-Key/Modal-Secret
// before they reach ASGI when requires_proxy_auth is false. Also still sends
// Modal-Key/Modal-Secret for true Modal proxy-auth endpoints.
func rlSidecarHeadersFromEnv() map[string]string {
	key := strings.TrimSpace(config.GetOr("ROUTER_RL_SIDECAR_MODAL_KEY", ""))
	secret := strings.TrimSpace(config.GetOr("ROUTER_RL_SIDECAR_MODAL_SECRET", ""))
	if key == "" && secret == "" {
		return nil
	}
	if key == "" || secret == "" {
		observability.Get().Error(
			"Partial Modal RL sidecar proxy auth config; omitting headers",
			"has_key", key != "",
			"has_secret", secret != "",
			"hint", "set both ROUTER_RL_SIDECAR_MODAL_KEY and ROUTER_RL_SIDECAR_MODAL_SECRET",
		)
		return nil
	}
	return map[string]string{
		"X-Weave-Band-Key":    key,
		"X-Weave-Band-Secret": secret,
		"Authorization":       "Bearer " + key + ":" + secret,
		"Modal-Key":           key,
		"Modal-Secret":        secret,
	}
}

// parseEnvDurationMs reads an env var as a millisecond integer and returns
// it as a time.Duration. Returns fallback when unset, empty, or unparseable.
func parseEnvDurationMs(key string, fallback time.Duration) time.Duration {
	raw := config.GetOr(key, "")
	if raw == "" {
		return fallback
	}
	ms, err := strconv.Atoi(raw)
	if err != nil || ms <= 0 {
		observability.Get().Warn("Invalid env var; using default", "key", key, "value", raw, "default_ms", fallback.Milliseconds())
		return fallback
	}
	return time.Duration(ms) * time.Millisecond
}

// parseEnvAttemptTimeoutMs is parseEnvDurationMs with 0 kept as a real value:
// it is how an operator disables the per-attempt bound, which the shared parser
// would otherwise reject and invert into the derived bound.
func parseEnvAttemptTimeoutMs(key string, fallback time.Duration) time.Duration {
	if ms, err := strconv.Atoi(config.GetOr(key, "")); err == nil && ms == 0 {
		return 0
	}
	return parseEnvDurationMs(key, fallback)
}

// buildSemanticCache constructs the cross-request semantic cache, or nil when
// disabled (ROUTER_SEMANTIC_CACHE_ENABLED=false). Per-cluster cosine thresholds
// come from the default version's metadata.yaml, so promoting `artifacts/latest`
// flips cache thresholds atomically along with the cluster version.
func buildSemanticCache(rtr router.Router) *cache.Cache {
	logger := observability.Get()
	if config.GetOr("ROUTER_SEMANTIC_CACHE_ENABLED", "true") != "true" {
		logger.Info("Semantic cache disabled (ROUTER_SEMANTIC_CACHE_ENABLED=false)")
		return nil
	}
	multi, ok := rtr.(*cluster.Multiversion)
	if !ok {
		// Defensive: guards against a future router shape silently disabling the cache.
		logger.Warn("Semantic cache disabled: router is not a cluster.Multiversion")
		return nil
	}
	defaultScorer, ok := multi.Versions[multi.Default]
	if !ok {
		// Defensive: NewMultiversion guarantees the default is built.
		logger.Warn("Semantic cache disabled: default cluster scorer not found", "default_version", multi.Default)
		return nil
	}
	perCluster, defaultThreshold := defaultScorer.CacheThresholds()

	cfg := cache.DefaultConfig()
	cfg.PerClusterThreshold = perCluster
	if defaultThreshold > 0 {
		cfg.DefaultThreshold = defaultThreshold
	}
	if v := config.GetOr("ROUTER_SEMANTIC_CACHE_TTL_SEC", ""); v != "" {
		secs, err := strconv.Atoi(v)
		if err != nil || secs <= 0 {
			logger.Warn("Invalid ROUTER_SEMANTIC_CACHE_TTL_SEC; using default", "value", v, "default_sec", int(cfg.TTL.Seconds()))
		} else {
			cfg.TTL = time.Duration(secs) * time.Second
		}
	}
	if v := config.GetOr("ROUTER_SEMANTIC_CACHE_BUCKET", ""); v != "" {
		size, err := strconv.Atoi(v)
		if err != nil || size <= 0 {
			logger.Warn("Invalid ROUTER_SEMANTIC_CACHE_BUCKET; using default", "value", v, "default", cfg.BucketSize)
		} else {
			cfg.BucketSize = size
		}
	}
	logger.Info(
		"Semantic cache enabled",
		"default_threshold", cfg.DefaultThreshold,
		"per_cluster_overrides", len(cfg.PerClusterThreshold),
		"bucket_size", cfg.BucketSize,
		"ttl_sec", int(cfg.TTL.Seconds()),
		"max_body_kib", cfg.MaxBodyBytes/1024,
		"version", multi.Default,
	)
	return cache.New(cfg)
}

// safeGo launches fn in a goroutine with panic recovery so a bug in a
// long-running background task (Pub/Sub listener, sweep loop, etc.) logs and
// dies quietly instead of crashing the whole process.
func safeGo(logger *slog.Logger, name string, fn func()) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				logger.Error("Background goroutine panicked", "goroutine", name, "panic", r)
			}
		}()
		fn()
	}()
}

// runSessionPinSweep deletes pins expired >24h on an hourly cadence. Uses
// context.Background() internally so the (idempotent, short) sweep keeps
// draining during graceful shutdown.
func runSessionPinSweep(ctx context.Context, store sessionpin.Store) {
	logger := observability.Get()
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			sweepCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			if err := store.SweepExpired(sweepCtx); err != nil {
				logger.Error("Session pin sweep failed", "err", err)
			}
			cancel()
		}
	}
}

// defaultHardPinProvider and defaultHardPinModel are the fallback (provider,
// model) used by resolveHardPinModel when the cluster bundle can't be loaded.
const (
	defaultHardPinProvider = providers.ProviderAiand
	defaultHardPinModel    = "deepseek-ai/deepseek-v4-flash"
	// aiandCatalogBudget bounds the dashboard's live ai& catalog fetch. It
	// mirrors AiandCatalogRequestBudget (the per-request upstream timeout the
	// handler enforces internally); keeping them in sync means a slow catalog
	// never strands the dashboard's request-handler goroutine waiting on the
	// client long after the request context has been cancelled.
	aiandCatalogBudget = 5 * time.Second
)

// resolveDefaultBaselineModel returns the cost-comparison baseline used when
// RequestedModel has no pricing entry. Uses os.LookupEnv directly (not
// config.GetOr) to distinguish unset (-> deepseek-ai/deepseek-v4-flash) from
// explicit "" (-> no substitution), per the contract in .env.example.
func resolveDefaultBaselineModel() string {
	v, ok := os.LookupEnv("ROUTER_DEFAULT_BASELINE_MODEL")
	if !ok {
		return "deepseek-ai/deepseek-v4-flash"
	}
	return strings.TrimSpace(v)
}

// resolveHardPinModel returns the (provider, model) for compaction and
// Explore hard-pins: operator override wins, else the fastest available
// model in the default bundle, else (defaultHardPinProvider, defaultHardPinModel).
func resolveHardPinModel(available map[string]struct{}, logger *slog.Logger) (provider, model string) {
	if m := config.GetOr("ROUTER_HARD_PIN_MODEL", ""); m != "" {
		p := config.GetOr("ROUTER_HARD_PIN_PROVIDER", defaultHardPinProvider)
		return p, m
	}

	reqVersion := config.GetOr("ROUTER_CLUSTER_VERSION", cluster.LatestVersion)
	defaultVersion, err := cluster.ResolveVersion(reqVersion)
	if err != nil {
		logger.Warn("Hard-pin model: could not resolve cluster version; using default", "err", err, "default_model", defaultHardPinModel)
		return defaultHardPinProvider, defaultHardPinModel
	}
	bundle, err := cluster.LoadBundle(defaultVersion)
	if err != nil {
		logger.Warn("Hard-pin model: could not load bundle; using default", "err", err, "default_model", defaultHardPinModel)
		return defaultHardPinProvider, defaultHardPinModel
	}
	p, m, ok := cluster.FastestModel(bundle.Metadata, bundle.Registry, available)
	if !ok {
		logger.Warn("Hard-pin model: no model found for available providers; using default", "default_model", defaultHardPinModel)
		return defaultHardPinProvider, defaultHardPinModel
	}
	return p, m
}

// envVarHint returns the env var name for a provider's API key, for log
// warnings; falls back to a readable "unknown provider" string.
func envVarHint(provider string) string {
	if v := providers.APIKeyEnvVar(provider); v != "" {
		return v
	}
	return "<unknown provider " + provider + ">"
}

// upstreamIDsForProvider maps public model ID -> upstream model ID for a
// provider's bindings with a non-empty UpstreamID; nil if no rewriting is
// needed (e.g. OpenRouter, where the slug IS the upstream ID).
// registerDeploymentKeyedProvider resolves a provider's deployment-level API
// key (respecting byokOnly), constructs its client via newClient, registers
// it in providerMap, and logs its BYOK/keyed/passthrough state. Shared by the
// providers whose registration collapses to "resolve key -> build client ->
// three-way log switch" (Fireworks, Makora, Together, Bedrock, Google);
// OpenRouter and Anthropic/OpenAI have genuinely different gating
// logic and stay bespoke. extraLogAttrs are appended only to the
// deployment-keyed log line (e.g. Bedrock's region).
func registerDeploymentKeyedProvider(
	providerMap map[string]providers.Client,
	envKeyedProviders map[string]struct{},
	logger *slog.Logger,
	name, displayName, keyEnvVar, baseURL string,
	byokOnly bool,
	newClient func(key, baseURL string) providers.Client,
	extraLogAttrs ...any,
) {
	key := ""
	if !byokOnly {
		key = config.GetOr(keyEnvVar, "")
	}
	providerMap[name] = newClient(key, baseURL)
	switch {
	case byokOnly:
		logger.Info(displayName+" provider enabled (BYOK only)", "base_url", baseURL)
	case key != "":
		envKeyedProviders[name] = struct{}{}
		logger.Info(displayName+" provider enabled", append([]any{"base_url", baseURL}, extraLogAttrs...)...)
	default:
		logger.Info(displayName+" provider registered (BYOK only — set "+keyEnvVar+" for deployment-level use)", "base_url", baseURL)
	}
}

func upstreamIDsForProvider(provider string) map[string]string {
	out := make(map[string]string)
	for _, m := range catalog.Models {
		for _, b := range m.Providers {
			if b.Provider == provider && b.UpstreamID != "" {
				out[m.ID] = b.UpstreamID
			}
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
