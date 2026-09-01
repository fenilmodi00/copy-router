package proxy

import (
	"context"
	"net/http"
	"strings"

	"workweave/router/internal/auth"
	"workweave/router/internal/providers"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// Credential sources, for logging and precedence reasoning. Never log the key
// itself — only the source.
const (
	credSourceBYOK   = "byok"
	credSourceClient = "client"
)

// Credentials holds the API key to use for an upstream request.
type Credentials struct {
	APIKey []byte // never logged
	Source string // credSourceBYOK | credSourceClient
	// BaseURL overrides the upstream endpoint per-request; non-empty only on BYOK
	// credentials, where the boot-time deployment URL is provider-wide, not per-key.
	BaseURL string
	// ModelAliases rewrites the outbound model ID for endpoints publishing the
	// catalog's models under their own names; non-empty only on BYOK credentials.
	ModelAliases map[string]string
	// IdentityHeader and IdentityHeaderFormat name and shape the header this
	// endpoint wants the caller's identity in; empty forwards nothing.
	IdentityHeader       string
	IdentityHeaderFormat string
}

// EffectiveBaseURL returns the BYOK key's per-request base URL if set,
// or fallback (the provider's boot-time deployment URL).
func EffectiveBaseURL(ctx context.Context, fallback string) string {
	creds := CredentialsFromContext(ctx)
	if creds == nil || creds.BaseURL == "" {
		return fallback
	}
	return strings.TrimRight(creds.BaseURL, "/")
}

// EffectiveUpstreamModel returns the aliased model name for this request's endpoint, or model
// unchanged. Only the outbound wire name changes -- routing, pricing, and telemetry use the catalog ID.
func EffectiveUpstreamModel(ctx context.Context, model string) string {
	creds := CredentialsFromContext(ctx)
	if creds == nil {
		return model
	}
	if alias, ok := creds.ModelAliases[model]; ok {
		return alias
	}
	return model
}

// ApplyModelAlias rewrites the body's top-level "model" field via the BYOK credential alias.
// Unaliased bodies are returned untouched; the envelope stays the authority on every other request.
func ApplyModelAlias(ctx context.Context, body []byte, model string) []byte {
	creds := CredentialsFromContext(ctx)
	if creds == nil || len(body) == 0 {
		return body
	}
	// Presence, not inequality: an alias equal to the catalog id still has to
	// overwrite a catalog UpstreamID an adapter already wrote into the body.
	upstreamModel, aliased := creds.ModelAliases[model]
	if !aliased {
		return body
	}
	if !gjson.GetBytes(body, "model").Exists() {
		return body
	}
	out, err := sjson.SetBytes(body, "model", upstreamModel)
	if err != nil {
		return body
	}
	return out
}

// ExternalAPIKeysContextKey is the request-context key for external API keys
// stashed by the auth middleware.
type ExternalAPIKeysContextKey struct{}

// BuildCredentialsMap builds provider -> Credentials from external keys.
// Empty-plaintext entries are dropped so the scorer doesn't route to a
// provider whose upstream call would 401.
func BuildCredentialsMap(keys []*auth.ExternalAPIKey) map[string]*Credentials {
	if len(keys) == 0 {
		return nil
	}
	m := make(map[string]*Credentials, len(keys))
	for _, key := range keys {
		if len(key.Plaintext) == 0 {
			continue
		}
		m[key.Provider] = &Credentials{
			APIKey:       key.Plaintext,
			Source:       credSourceBYOK,
			BaseURL:      key.BaseURL,
			ModelAliases: key.ModelAliases,

			IdentityHeader:       key.IdentityHeader,
			IdentityHeaderFormat: key.IdentityHeaderFormat,
		}
	}
	if len(m) == 0 {
		return nil
	}
	return m
}

// ExtractClientCredentials extracts provider credentials from request
// headers: Authorization: Bearer for OpenAI-compat upstreams.
//
// Rejects any token with auth.APIKeyPrefix — router-issued bearers (rk_...)
// use the same headers via WithAuth, and this stops them leaking upstream
// (load-bearing leak guard). Rejects Anthropic-shaped tokens so one Bearer
// header isn't misidentified as creds for a provider it doesn't belong to.
func ExtractClientCredentials(provider string, headers http.Header) *Credentials {
	if providers.FamilyFor(provider) != providers.FamilyOpenAICompat {
		return nil
	}
	if raw, found := strings.CutPrefix(headers.Get("Authorization"), "Bearer "); found {
		key := strings.TrimSpace(raw)
		if key != "" && !auth.HasAPIKeyPrefix(key) && !strings.HasPrefix(key, "sk-ant-") {
			return &Credentials{APIKey: []byte(key), Source: credSourceClient}
		}
	}
	return nil
}

// clearCredentials sets an explicit nil so CredentialsFromContext reports
// none and the provider client falls back to its deployment key. Used to
// keep a caller's key off synthetic upstream calls (e.g. the handover
// summarizer), which run on the router's own credential context.
func clearCredentials(ctx context.Context) context.Context {
	return context.WithValue(ctx, CredentialsContextKey{}, (*Credentials)(nil))
}
