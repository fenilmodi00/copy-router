package auth

import (
	"context"
	"crypto/rand"
	"errors"
	"time"

	"github.com/hashicorp/golang-lru/v2/expirable"

	"workweave/router/internal/observability"
	"workweave/router/internal/providers"
)

// AiandIdentity is the identity returned by validating an aiand sk- key.
type AiandIdentity struct {
	UserID         string
	OrganizationID string
	Plan           string
}

// AiandKeyVerifier validates an aiand sk- key against aiand's API. The HTTP
// call is I/O, so the concrete implementation lives in internal/providers/aiand;
// this interface keeps internal/auth I/O-free.
type AiandKeyVerifier interface {
	Validate(ctx context.Context, rawKey string) (*AiandIdentity, error)
}

// LoginSessionCookieName is the HttpOnly cookie holding a self-service dashboard
// session.
const LoginSessionCookieName = "router_account_session"

// DefaultLoginSessionTTL is the self-service session validity.
const DefaultLoginSessionTTL = 7 * 24 * time.Hour

// loginSessionTokenChars is the rejection-sampled alphabet for session tokens
// (same approach as GenerateID, no modulo bias).
const loginSessionTokenChars = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"

// ctxKeyAccount is the typed context key for the resolved account.
type ctxKeyAccount struct{}

// WithAccountRepos wires the account + session repositories. Both nil is a
// no-op (login returns ErrLoginDisabled); kept off NewService so existing
// callers/tests stay source-stable.
func (s *Service) WithAccountRepos(accounts AccountRepository, sessions LoginSessionRepository) *Service {
	s.accounts = accounts
	s.loginSessions = sessions
	return s
}

// WithKeyVerifier wires the aiand identity probe. nil is a no-op (login
// returns ErrLoginDisabled).
func (s *Service) WithKeyVerifier(v AiandKeyVerifier) *Service {
	s.keyVerifier = v
	if s.loginFailures == nil {
		// 1024 unique IPs is plenty for one router; LRU evicts oldest so a
		// key-spray can't grow memory unboundedly.
		s.loginFailures = expirable.NewLRU[string, int](1024, nil, loginFailureTTL)
	}
	return s
}

// LoginEnabled reports whether self-service login is fully wired (key verifier
// + both repositories).
func (s *Service) LoginEnabled() bool {
	return s.keyVerifier != nil && s.accounts != nil && s.loginSessions != nil
}

const (
	loginMaxFailures = 5
	loginFailureTTL  = 5 * time.Minute
)

// LoginRateLimited reports whether remoteIP has exceeded the per-IP login
// failure cap.
func (s *Service) LoginRateLimited(remoteIP string) bool {
	if s.loginFailures == nil {
		return false
	}
	count, ok := s.loginFailures.Get(remoteIP)
	return ok && count >= loginMaxFailures
}

// NoteLoginFailure records one failed login for remoteIP (no-op when the
// limiter isn't wired or the IP is empty).
func (s *Service) NoteLoginFailure(remoteIP string) {
	if s.loginFailures == nil || remoteIP == "" {
		return
	}
	count, _ := s.loginFailures.Get(remoteIP)
	s.loginFailures.Add(remoteIP, count+1)
}

// ClearLoginFailures resets the failure counter for remoteIP on success.
func (s *Service) ClearLoginFailures(remoteIP string) {
	if s.loginFailures == nil || remoteIP == "" {
		return
	}
	s.loginFailures.Remove(remoteIP)
}

// LoginWithKey validates an aiand key, creates-or-returns the aiand user's
// account AND its router installation, mints a session token, and returns
// (account, installation, raw token, expiry, nil). When the same aiand user logs
// in with a NEW key, the existing account + installation are returned (data
// retention across rotation lives entirely on aiand's side).
func (s *Service) LoginWithKey(ctx context.Context, rawKey string) (*Account, *Installation, string, time.Time, error) {
	if !s.LoginEnabled() {
		return nil, nil, "", time.Time{}, ErrLoginDisabled
	}
	identity, err := s.keyVerifier.Validate(ctx, rawKey)
	if err != nil {
		if errors.Is(err, ErrKeyInvalid) ||
			errors.Is(err, ErrKeyInsufficientCredits) ||
			errors.Is(err, ErrLoginRateLimited) ||
			errors.Is(err, ErrKeyUnavailable) {
			return nil, nil, "", time.Time{}, err
		}
		return nil, nil, "", time.Time{}, err
	}
	if identity == nil || identity.UserID == "" {
		return nil, nil, "", time.Time{}, ErrKeyInvalid
	}

	// Candidate id doubles as the installation external_id. On a returning
	// login the upsert returns the EXISTING row's id (see the query's ON
	// CONFLICT), so both callers converge on one installation.
	account, err := s.accounts.UpsertByAiandUser(ctx, AccountUpsertParams{
		ID:                  GenerateID("acct"),
		AiandUserID:         identity.UserID,
		AiandOrganizationID: identity.OrganizationID,
	})
	if err != nil {
		return nil, nil, "", time.Time{}, err
	}

	installation, err := s.EnsureAccountInstallation(ctx, account)
	if err != nil {
		return nil, nil, "", time.Time{}, err
	}
	if err := s.storeLoginAiandKey(ctx, installation.ID, rawKey); err != nil {
		return nil, nil, "", time.Time{}, err
	}

	token, expiresAt, err := s.IssueLoginSession(ctx, account, installation)
	if err != nil {
		return nil, nil, "", time.Time{}, err
	}
	return account, installation, token, expiresAt, nil
}

// InstallationForAccountSession returns the installation bound to a dashboard
// session. When installation_id was cached at login, loads that row directly;
// otherwise falls back to EnsureAccountInstallation (repair path for sessions
// minted before the column existed).
func (s *Service) InstallationForAccountSession(ctx context.Context, account *Account, session *LoginSession) (*Installation, error) {
	if account == nil {
		return nil, ErrLoginSessionInvalid
	}
	if session != nil && session.InstallationID != "" {
		inst, err := s.installations.Get(ctx, account.ID, session.InstallationID)
		if err == nil && inst != nil && inst.DeletedAt == nil {
			return inst, nil
		}
	}
	return s.EnsureAccountInstallation(ctx, account)
}

// EnsureAccountInstallation returns the installation whose external_id equals
// the account's id, creating it on first call. Mirrors EnsureAdminInstallation:
// on a concurrent first-hit the loser re-lists and returns the winner's row.
func (s *Service) EnsureAccountInstallation(ctx context.Context, account *Account) (*Installation, error) {
	if inst, ok, err := s.findAccountInstallation(ctx, account.ID); err != nil {
		return nil, err
	} else if ok {
		return inst, nil
	}
	name := account.AiandUserID
	if account.DisplayName != nil && *account.DisplayName != "" {
		name = *account.DisplayName
	}
	created, createErr := s.installations.Create(ctx, CreateInstallationParams{
		ExternalID: account.ID,
		Name:       name,
	})
	if createErr == nil {
		return created, nil
	}
	// Lost a concurrent race.
	if inst, ok, err := s.findAccountInstallation(ctx, account.ID); err == nil && ok {
		return inst, nil
	}
	return nil, createErr
}

func (s *Service) findAccountInstallation(ctx context.Context, accountID string) (*Installation, bool, error) {
	existing, err := s.installations.ListForExternalID(ctx, accountID)
	if err != nil {
		return nil, false, err
	}
	for _, inst := range existing {
		if inst != nil && inst.DeletedAt == nil {
			return inst, true, nil
		}
	}
	return nil, false, nil
}

// IssueLoginSession mints an opaque token, stores its SHA-256 hash, and returns
// the raw token + expiry. The raw token is returned once and never stored.
func (s *Service) IssueLoginSession(ctx context.Context, account *Account, installation *Installation) (string, time.Time, error) {
	if s.loginSessions == nil {
		return "", time.Time{}, ErrLoginDisabled
	}
	if installation == nil || installation.ID == "" {
		return "", time.Time{}, ErrLoginSessionInvalid
	}
	now := s.now()
	expiresAt := now.Add(DefaultLoginSessionTTL)
	raw := generateLoginSessionToken()
	hash, prefix, suffix := APITokenFingerprint(raw)
	session, err := s.loginSessions.Insert(ctx, LoginSession{
		AccountID:      account.ID,
		InstallationID: installation.ID,
		TokenHash:      hash,
		TokenPrefix:    prefix,
		TokenSuffix:    suffix,
		IssuedAt:       now,
		ExpiresAt:      expiresAt,
	})
	if err != nil {
		return "", time.Time{}, err
	}
	if session == nil || session.ID == "" {
		return "", time.Time{}, ErrLoginSessionInvalid
	}
	return raw, expiresAt, nil
}

// VerifyLoginSession resolves a cookie token to its account. Returns
// ErrLoginSessionInvalid for unknown, expired, or revoked tokens.
func (s *Service) VerifyLoginSession(ctx context.Context, token string) (*Account, error) {
	acct, _, err := s.VerifyLoginSessionDetails(ctx, token)
	return acct, err
}

// VerifyLoginSessionDetails resolves a cookie token to its account and session
// row. Returns ErrLoginSessionInvalid for unknown, expired, or revoked tokens.
func (s *Service) VerifyLoginSessionDetails(ctx context.Context, token string) (*Account, *LoginSession, error) {
	if s.loginSessions == nil || s.accounts == nil {
		return nil, nil, ErrLoginSessionInvalid
	}
	hash := HashAPIKeySHA256(token)
	session, err := s.loginSessions.GetActiveByTokenHash(ctx, hash)
	if err != nil {
		return nil, nil, ErrLoginSessionInvalid
	}
	if session == nil || session.AccountID == "" {
		return nil, nil, ErrLoginSessionInvalid
	}
	account, err := s.accounts.GetByID(ctx, session.AccountID)
	if err != nil {
		return nil, nil, ErrLoginSessionInvalid
	}
	return account, session, nil
}

// AccountFromContext returns the account stashed on ctx by the middleware, or
// nil.
func AccountFromContext(ctx context.Context) *Account {
	v, ok := ctx.Value(ctxKeyAccount{}).(*Account)
	if !ok {
		return nil
	}
	return v
}

// storeLoginAiandKey persists the presented aiand sk- key as the installation's
// aiand BYOK credential so dashboard inference (playground, etc.) bills the
// signed-in user instead of the deployment key.
func (s *Service) storeLoginAiandKey(ctx context.Context, installationID, rawKey string) error {
	if s.externalKeys == nil || installationID == "" || rawKey == "" {
		return nil
	}
	_, err := s.UpsertExternalAPIKey(ctx, installationID, UpsertExternalAPIKeyParams{
		Provider: providers.ProviderAiand,
		RawKey:   rawKey,
	})
	if err != nil {
		observability.FromContext(ctx).Error("Failed to store login aiand key", "installation_id", installationID, "err", err)
	}
	return err
}

// generateLoginSessionToken returns a rejection-sampled 32-char base62 token.
func generateLoginSessionToken() string {
	buf := make([]byte, 32)
	limit := byte(256 - (256 % len(loginSessionTokenChars)))
	out := make([]byte, 0, 32)
	for len(out) < 32 {
		if _, err := rand.Read(buf); err != nil {
			panic(err)
		}
		for _, x := range buf {
			if x >= limit {
				continue
			}
			out = append(out, loginSessionTokenChars[int(x)%len(loginSessionTokenChars)])
			if len(out) == 32 {
				break
			}
		}
	}
	return string(out)
}
