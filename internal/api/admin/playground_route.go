package admin

import (
	"errors"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"workweave/router/internal/auth"
	"workweave/router/internal/observability"
	"workweave/router/internal/proxy"
	"workweave/router/internal/router/cluster"
	"workweave/router/internal/translate"
)

// PlaygroundRouteHandler exposes the playground preview decision payload: model,
// provider, reason, requested_cost_usd, actual_cost_usd, cache_savings_usd, and id.
// Sanitized — no metadata leak.
func PlaygroundRouteHandler(svc *proxy.Service, authSvc *auth.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		log := observability.FromGin(c)

		body, err := io.ReadAll(io.LimitReader(c.Request.Body, proxy.MaxRequestBodyBytes+1))
		if err != nil {
			log.Debug("Failed to read playground request body", "err", err)
			writePlaygroundError(c, http.StatusRequestEntityTooLarge, "invalid_request_error", "Request body too large.", nil)
			return
		}
		if len(body) > proxy.MaxRequestBodyBytes {
			log.Debug("Request body exceeds MaxRequestBodyBytes", "path", c.Request.URL.Path)
			writePlaygroundError(c, http.StatusRequestEntityTooLarge, "invalid_request_error", "Request body too large.", nil)
			return
		}

		ctx, _, ok := bindPlaygroundProxyContext(c, authSvc)
		if !ok {
			return
		}
		decision, routeErr := svc.PreviewOpenAIRoute(ctx, body, c.Request.Header)
		if routeErr != nil {
			if errors.Is(routeErr, proxy.ErrRequestNotJSONObject) {
				writePlaygroundError(c, http.StatusBadRequest, "invalid_request_error", "Request body must be a JSON object.", nil)
				return
			}
			if errors.Is(routeErr, proxy.ErrForcedModelUnknown) {
				cls, _ := proxy.ClassifyDispatchError(routeErr)
				writePlaygroundError(c, cls.Status, "invalid_request_error", cls.Message, strPtr("forced_model_unknown"))
				return
			}
			if errors.Is(routeErr, cluster.ErrInvalidRoutingKnobs) {
				writePlaygroundError(c, http.StatusBadRequest, "invalid_request_error", "Invalid routing knobs supplied.", nil)
				return
			}
			log.Error("Playground routing failed", "err", routeErr)
			writePlaygroundError(c, http.StatusBadGateway, "api_error", "Routing failed.", strPtr("routing_failed"))
			return
		}

		decision.Model = proxy.CanonicalDecisionModel(decision.Model)
		routingTextTokens := 0
		maxTokens := 0
		if env, parseErr := translate.ParseOpenAI(body); parseErr == nil {
			feats := env.RoutingFeatures(false)
			routingTextTokens = feats.Tokens
			maxTokens = feats.MaxTokens
		}
		inputTokens, outputTokens := proxy.DecisionCostTokens(body, routingTextTokens, maxTokens)
		requestedCost, actualCost, costErr := svc.CostForDecision(ctx, decision, inputTokens, outputTokens)
		if costErr != nil {
			log.Error("Cost computation failed", "err", costErr)
			requestedCost, actualCost = 0, 0
		}

		id := observability.RequestIDFromContext(ctx)
		if id == "" {
			id = uuid.New().String()
		}

		c.JSON(http.StatusOK, gin.H{
			"model":              decision.Model,
			"provider":           decision.Provider,
			"reason":             proxy.PlaygroundReasonShort(decision.Reason),
			"requested_cost_usd": requestedCost,
			"actual_cost_usd":    actualCost,
			"cache_savings_usd":  proxy.CacheInputSavings(decision, 0).Float64(),
			"id":                 id,
		})
	}
}

func writePlaygroundError(c *gin.Context, status int, errType, message string, code *string) {
	c.AbortWithStatusJSON(status, gin.H{
		"error": gin.H{
			"message": message,
			"type":    errType,
			"param":   nil,
			"code":    code,
		},
	})
}

func strPtr(s string) *string {
	return &s
}
