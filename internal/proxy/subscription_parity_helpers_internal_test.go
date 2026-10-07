package proxy

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"weave-os/router/internal/auth"
	"weave-os/router/internal/providers"
	"weave-os/router/internal/router"
)

const (
	parityAnthropicToken = "sk-ant-oat01-parity-subscription-token"
	parityAnthropicBYOK  = "sk-ant-api-parity-byok"
	parityAnthropicModel = "moonshotai/kimi-k3"
	parityInstallationID = "22222222-2222-2222-2222-222222222222"
)

// parityIngress describes one ingress' subscription surface: the credential it
// recognizes, the predicates that gate the rescue, and how to drive the ingress
// end to end.
type parityIngress struct {
	name     string
	provider string
	// model is the subscription's covered model.
	model string
	token string

	// subCtx carries the caller's subscription as the auth middleware would.
	subCtx func() context.Context
	// servedOnSub is the ingress' "is this turn on a subscription?" predicate.
	servedOnSub func(context.Context) bool
	// fallbackAvailable is the paid-fallback probe.
	fallbackAvailable func(*Service, context.Context) bool
	// exhausted is the pre-dispatch suppression predicate.
	exhausted func(*Service, context.Context, http.Header) bool
	// suppress is the provider-scoped suppression key.
	suppress func(context.Context) context.Context
	// tokenRejected is the "the OAuth credential itself is bad" classifier.
	tokenRejected func(error) bool

	// requestBody builds an ingress-native request body.
	requestBody func(stream bool) string
	// upstreamOK is an upstream success payload the ingress' translator accepts.
	upstreamOK func(stream bool) string
	// call drives the ingress.
	call func(*Service, context.Context, []byte, http.ResponseWriter, *http.Request) error
	path string
}

func parityAnthropicIngress() parityIngress {
	return parityIngress{
		name:     "anthropic",
		provider: providers.ProviderAnthropic,
		model:    parityAnthropicModel,
		token:    parityAnthropicToken,
		subCtx: func() context.Context {
			ctx := context.WithValue(context.Background(), AnthropicSubscriptionContextKey{}, parityAnthropicToken)
			return context.WithValue(ctx, InstallationIDContextKey{}, parityInstallationID)
		},
		servedOnSub:       servedOnSubscription,
		fallbackAvailable: (*Service).anthropicFallbackKeyAvailable,
		exhausted:         (*Service).claudeSubscriptionExhausted,
		suppress:          withSuppressedClaudeSubscription,
		tokenRejected:     anthropicOAuthCredentialRejected,
		// A tool-bearing main-loop turn: a bare one-liner is hard-pinned as a
		// classifier turn, which short-circuits the routing stages under test.
		requestBody: func(stream bool) string {
			return `{"model":"` + parityAnthropicModel + `","max_tokens":4096,"stream":` + boolLit(stream) +
				`,"tools":[{"name":"Bash","description":"run a command","input_schema":{"type":"object","properties":{}}}]` +
				`,"messages":[{"role":"user","content":"investigate the failing dispatch and report back"}]}`
		},
		upstreamOK: func(stream bool) string {
			if stream {
				return "event: message_start\n" +
					`data: {"type":"message_start","message":{"usage":{"input_tokens":5,"output_tokens":1}}}` + "\n\n" +
					"event: message_delta\n" +
					`data: {"type":"message_delta","usage":{"output_tokens":2}}` + "\n\n"
			}
			return `{"id":"msg_1","type":"message","role":"assistant","content":[{"type":"text","text":"hi"}],` +
				`"model":"` + parityAnthropicModel + `","stop_reason":"end_turn","usage":{"input_tokens":5,"output_tokens":2}}`
		},
		call: (*Service).ProxyMessages,
		path: "/v1/messages",
	}
}

func boolLit(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

// byokCtx adds a BYOK key for the ingress' provider.
func (in parityIngress) byokCtx(ctx context.Context) context.Context {
	return context.WithValue(ctx, ExternalAPIKeysContextKey{}, []*auth.ExternalAPIKey{
		{Provider: in.provider, Plaintext: []byte(parityAnthropicBYOK)},
	})
}

// resolved resolves credentials for the ingress' covered model, as the ingress
// does before dispatching.
func (in parityIngress) resolved(ctx context.Context) context.Context {
	return resolveAndInjectCredentials(ctx, in.provider, in.model, http.Header{})
}

// deploymentKeyedService builds a Service with the ingress' provider wired to a
// deployment key — the paid fallback a rescue spends.
func (in parityIngress) deploymentKeyedService() *Service {
	return &Service{deploymentKeyedProviders: map[string]struct{}{in.provider: {}}}
}

// upstreamErr builds a buffered upstream error, the shape both classifiers read.
func upstreamErr(status int, body string) error {
	return &providers.UpstreamErrorResponse{Status: status, Body: []byte(body)}
}

// parityUpstream answers each dispatch according to the credential it carries,
// not to a call index: the subscription attempt and the paid rescue are scripted
// independently, so the assertions stay meaningful regardless of how many
// same-binding retries the dispatcher interposes between them.
type parityUpstream struct {
	// subErr fails every dispatch that carries the caller's subscription.
	subErr error
	// paidErr fails every dispatch that serves on a paid key.
	paidErr error
	// okBody is written by a dispatch scripted to succeed.
	okBody string
	// preErrBody is written before returning an error: provider bytes on the
	// wire commit the attempt.
	preErrBody string

	subDispatches  int
	paidDispatches int
	servedModels   []string
}

func (p *parityUpstream) Proxy(ctx context.Context, decision router.Decision, _ providers.PreparedRequest, w http.ResponseWriter, _ *http.Request) error {
	p.servedModels = append(p.servedModels, decision.Model)
	// No credential in context means the provider adapter falls back to the
	// deployment key, which is a paid dispatch.
	creds := CredentialsFromContext(ctx)
	onSubscription := creds != nil && creds.OAuth
	err := p.paidErr
	if onSubscription {
		p.subDispatches++
		err = p.subErr
	} else {
		p.paidDispatches++
	}
	if err != nil {
		if p.preErrBody != "" {
			_, _ = io.WriteString(w, p.preErrBody)
		}
		return err
	}
	_, _ = io.WriteString(w, p.okBody)
	return nil
}

func (p *parityUpstream) Passthrough(context.Context, providers.PreparedRequest, http.ResponseWriter, *http.Request) error {
	return providers.ErrNotImplemented
}

// parityService wires the ingress' provider to upstream with a deployment key,
// which is the paid fallback the rescue spends.
func (in parityIngress) parityService(upstream providers.Client) *Service {
	svc := NewService(
		staticRouter{decision: router.Decision{Provider: in.provider, Model: in.model, Reason: "test"}},
		map[string]providers.Client{in.provider: upstream},
		nil, false, nil, nil, false, in.provider, in.model, nil,
	).WithDeploymentKeyedProviders(map[string]struct{}{in.provider: {}})
	svc.retrySleep = noopSleep
	return svc
}

func (in parityIngress) request(t *testing.T, stream bool) (*httptest.ResponseRecorder, *http.Request, []byte) {
	t.Helper()
	body := in.requestBody(stream)
	return httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, in.path, strings.NewReader(body)), []byte(body)
}
