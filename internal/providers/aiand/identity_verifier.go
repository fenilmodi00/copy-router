// Package aiand provides the aiand (aiand.com) identity probe used by the
// self-service dashboard login: validating an sk- key against a key-auth
// endpoint and reading the org from the response.
package aiand

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"workweave/router/internal/auth"
)

// DefaultBaseURL is aiand's API root. Note: the OpenAI-compatible inference
// base URL (openaicompat.AiandBaseURL) already includes /v1; identity probing
// uses the API root + /v1/models (key-auth), not /api/v1/me (JWT-only).
const DefaultBaseURL = "https://api.aiand.com"

// KeyVerifier validates an aiand sk- key by probing GET /v1/models.
//
// Live ai& docs: /api/v1/me requires a JWT session token (console cookie),
// not an sk- API key. sk- keys authenticate OpenAI-compat + analytics
// surfaces and stamp X-Org-Id on the response. We probe /v1/models (cheap,
// key-auth) and treat that header as the stable tenant identity — API keys
// are org-scoped, so one org → one selfserve account (key rotation within
// the same org reuses the same installation).
type KeyVerifier struct {
	Client  *http.Client
	BaseURL string
}

// identityAPIRoot returns the aiand API root for the key-auth probe.
// AIAND_API_URL is shared with OpenAI-compat inference, which already includes
// /v1 (openaicompat.AiandBaseURL). Stripping a trailing /v1 keeps the probe at
// GET {root}/v1/models instead of the broken /v1/v1/models that maps to
// ErrKeyUnavailable → HTTP 503 key_validation_unavailable.
func identityAPIRoot(baseURL string) string {
	base := strings.TrimSuffix(strings.TrimSpace(baseURL), "/")
	base = strings.TrimSuffix(base, "/v1")
	if base == "" {
		return DefaultBaseURL
	}
	return base
}

// Validate calls aiand's GET /v1/models with the key as a bearer token and
// maps the response to auth sentinels. Never returns a partial identity on
// error.
func (v *KeyVerifier) Validate(ctx context.Context, rawKey string) (*auth.AiandIdentity, error) {
	if v.Client == nil {
		return nil, errors.New("aiand: nil http client")
	}
	base := identityAPIRoot(v.BaseURL)
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/v1/models", nil)
	if err != nil {
		return nil, fmt.Errorf("aiand: build probe request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+rawKey)
	req.Header.Set("Accept", "application/json")

	resp, err := v.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", auth.ErrKeyUnavailable, err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))

	switch resp.StatusCode {
	case http.StatusOK:
		orgID := strings.TrimSpace(resp.Header.Get("X-Org-Id"))
		if orgID == "" {
			return nil, fmt.Errorf("%w: aiand /v1/models missing X-Org-Id", auth.ErrKeyUnavailable)
		}
		// sk- keys are org-scoped; there is no separate user_id on the
		// key-auth path. Use the org as the account upsert key so rotating
		// to another key in the same org keeps the same installation.
		return &auth.AiandIdentity{
			UserID:         orgID,
			OrganizationID: orgID,
		}, nil
	case http.StatusUnauthorized, http.StatusForbidden:
		return nil, auth.ErrKeyInvalid
	case http.StatusPaymentRequired:
		return nil, auth.ErrKeyInsufficientCredits
	case http.StatusTooManyRequests:
		return nil, auth.ErrLoginRateLimited
	default:
		return nil, fmt.Errorf("%w: aiand /v1/models returned %d", auth.ErrKeyUnavailable, resp.StatusCode)
	}
}
