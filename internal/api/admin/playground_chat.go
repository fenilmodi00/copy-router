package admin

import (
	"context"
	"encoding/json"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"

	"workweave/router/internal/auth"
	"workweave/router/internal/flags"
	"workweave/router/internal/observability"
	"workweave/router/internal/proxy"
	"workweave/router/internal/server/middleware"
)

const playgroundSessionHeader = "X-Playground-Session"

type playgroundChatProxy interface {
	ProxyOpenAIChatCompletion(ctx context.Context, body []byte, w http.ResponseWriter, r *http.Request) error
}

// PlaygroundChatHandler streams OpenAI Chat Completions through the proxy for the
// dashboard playground. Session pinning is request-scoped via X-Playground-Session.
func PlaygroundChatHandler(svc playgroundChatProxy, authSvc *auth.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		log := observability.FromGin(c)

		body, err := io.ReadAll(io.LimitReader(c.Request.Body, proxy.MaxRequestBodyBytes+1))
		if err != nil {
			log.Debug("Failed to read playground chat body", "err", err)
			writePlaygroundError(c, http.StatusBadRequest, "invalid_request_error", "Failed to read request body.", nil)
			return
		}
		if len(body) > proxy.MaxRequestBodyBytes {
			writePlaygroundError(c, http.StatusRequestEntityTooLarge, "invalid_request_error", "Request body too large.", nil)
			return
		}
		body = proxy.NormalizeOpenAIPlaygroundModel(body)

		ctx, installation, ok := bindPlaygroundProxyContext(c, authSvc)
		if !ok {
			return
		}
		identity := proxy.ClientIdentityFromHeaders(c.Request.Header)
		identity.ClientApp = proxy.ClientAppPlayground
		playgroundSession := proxy.NormalizeClientIdentifier(c.GetHeader(playgroundSessionHeader))
		if playgroundSession != "" {
			identity.SessionID = playgroundSession
			body = proxy.InjectOpenAIPlaygroundSession(body, playgroundSession)
		}
		ctx = context.WithValue(ctx, proxy.ClientIdentityContextKey{}, identity)
		ctx = proxy.ResolveUserFromContext(ctx, authSvc, middleware.InstallationFrom(c))
		c.Request.Header.Set(playgroundRoutingMarkerHeader, "off")
		// The playground is a chat-shaped surface backed by the fork's own
		// upstream; keep its turns on chat/completions rather than the broad
		// Responses rollout.
		ctx = flags.WithOverrides(ctx, flags.Overrides{
			Bools: map[flags.Key]bool{flags.KeyOpenAIResponsesBroad: false},
		})

		err = svc.ProxyOpenAIChatCompletion(ctx, body, c.Writer, c.Request)
		if err == nil && installation != nil {
			authSvc.NotifyRoutedRequest(installation.ID)
		}
		if err != nil {
			cls, ok := proxy.ClassifyDispatchError(err)
			if ok && cls.Kind == proxy.DispatchErrorUpstreamStatus {
				if c.Writer.Written() {
					return
				}
				if cls.Status == http.StatusTooManyRequests {
					c.Header("Retry-After", "1")
				}
				writePlaygroundError(c, cls.Status, "api_error", cls.Message, strPtr(playgroundErrorCode(cls)))
				return
			}
			if c.Writer.Written() {
				writePlaygroundMidStreamError(c, err)
				return
			}
			if ok {
				proxy.LogDispatchErrorClass(log, cls, err)
				if cls.RetryAfter {
					c.Header("Retry-After", "1")
				}
				errType := playgroundErrorType(cls.Kind)
				writePlaygroundError(c, cls.Status, errType, cls.Message, strPtr(playgroundErrorCode(cls)))
				return
			}
			log.Error("Playground chat proxy failed", "err", err)
			writePlaygroundError(c, http.StatusBadGateway, "api_error", "Upstream call failed.", strPtr("routing_failed"))
		}
	}
}

func playgroundErrorType(kind proxy.DispatchErrorKind) string {
	if kind.IsClientError() {
		return "invalid_request_error"
	}
	return "api_error"
}

func playgroundErrorCode(cls proxy.DispatchErrorClass) string {
	switch cls.Kind {
	case proxy.DispatchErrorUpstreamStatus:
		if cls.Status == http.StatusTooManyRequests {
			return "rate_limit"
		}
		if cls.Status == http.StatusPaymentRequired {
			return "insufficient_credits"
		}
		if cls.Status == http.StatusServiceUnavailable {
			return "unavailable"
		}
		if cls.Status >= 400 && cls.Status < 500 {
			return "provider_error"
		}
		return "routing_failed"
	case proxy.DispatchErrorCreditsExhausted, proxy.DispatchErrorUserSpendLimitReached:
		return "insufficient_credits"
	case proxy.DispatchErrorTranslationProviderUnavailable,
		proxy.DispatchErrorRLPolicyUnavailable,
		proxy.DispatchErrorBanditUnavailable,
		proxy.DispatchErrorHMMUnavailable,
		proxy.DispatchErrorSpendLimitUnavailable:
		return "unavailable"
	default:
		if cls.RetryAfter || cls.Status == http.StatusTooManyRequests {
			return "rate_limit"
		}
		if cls.Status >= 400 && cls.Status < 500 {
			return "provider_error"
		}
		if cls.Status == http.StatusServiceUnavailable {
			return "unavailable"
		}
		return "routing_failed"
	}
}

func writePlaygroundMidStreamError(c *gin.Context, err error) {
	msg := err.Error()
	payload, marshalErr := json.Marshal(gin.H{
		"type":    "api_error",
		"message": msg,
		"param":   nil,
		"code":    "upstream_interrupted",
	})
	if marshalErr != nil {
		return
	}
	_, _ = c.Writer.Write([]byte("data: "))
	_, _ = c.Writer.Write(payload)
	_, _ = c.Writer.Write([]byte("\n\n"))
	if f, ok := c.Writer.(http.Flusher); ok {
		f.Flush()
	}
}
