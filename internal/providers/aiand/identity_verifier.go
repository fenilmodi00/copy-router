// Package aiand provides the aiand (aiand.com) identity probe used by the
// self-service dashboard login: validating an sk- key against GET /api/v1/me.
package aiand

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"workweave/router/internal/auth"
)

// DefaultBaseURL is aiand's API root. Note: the OpenAI-compatible inference
// base URL (openaicompat.AiandBaseURL) already includes /v1; the identity
// endpoint lives at the API root + /api/v1/me.
const DefaultBaseURL = "https://api.aiand.com"

// KeyVerifier validates an aiand sk- key by probing GET /api/v1/me.
type KeyVerifier struct {
	Client  *http.Client
	BaseURL string
}

// Validate calls aiand's GET /api/v1/me with the key as a bearer token and
// maps the response to auth sentinels. Never returns a partial identity on
// error.
func (v *KeyVerifier) Validate(ctx context.Context, rawKey string) (*auth.AiandIdentity, error) {
	if v.Client == nil {
		return nil, errors.New("aiand: nil http client")
	}
	base := strings.TrimSuffix(v.BaseURL, "/")
	if base == "" {
		base = DefaultBaseURL
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/api/v1/me", nil)
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

	switch resp.StatusCode {
	case http.StatusOK:
		var body struct {
			UserID         string `json:"user_id"`
			OrganizationID string `json:"organization_id"`
			Plan           string `json:"plan"`
		}
		if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&body); err != nil {
			return nil, fmt.Errorf("aiand: decode /me response: %w", err)
		}
		if body.UserID == "" {
			return nil, auth.ErrKeyInvalid
		}
		return &auth.AiandIdentity{
			UserID:         body.UserID,
			OrganizationID: body.OrganizationID,
			Plan:           body.Plan,
		}, nil
	case http.StatusUnauthorized, http.StatusForbidden:
		return nil, auth.ErrKeyInvalid
	case http.StatusPaymentRequired:
		return nil, auth.ErrKeyInsufficientCredits
	case http.StatusTooManyRequests:
		return nil, auth.ErrLoginRateLimited
	default:
		return nil, fmt.Errorf("%w: aiand /me returned %d", auth.ErrKeyUnavailable, resp.StatusCode)
	}
}
