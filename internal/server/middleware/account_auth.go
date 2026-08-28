package middleware

import (
	"net/http"
	"strings"

	"workweave/router/internal/auth"
	"workweave/router/internal/observability"

	"github.com/gin-gonic/gin"
)

const ctxKeyAccount = "router_account"

// WithAccountCookie authenticates an account session cookie, resolves the
// account's installation from the cached session row, and stashes BOTH on ctx
// so the existing per-installation admin handlers (metrics scoping, keys, BYOK,
// config) work unchanged. Missing or invalid cookies are 401 — the dashboard's
// fetch layer bounces to login.
//
// This is the self-serve-mode counterpart to WithAdminOnly: an account cookie is
// a dashboard identity, never a data-plane credential.
func WithAccountCookie(svc *auth.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		acct, session := tryAccountCookieDetails(c, svc)
		if acct == nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "account_session_required"})
			return
		}
		inst, err := svc.InstallationForAccountSession(c.Request.Context(), acct, session)
		if err != nil {
			observability.FromGin(c).Error("Failed to resolve account installation", "err", err, "account_id", acct.ID)
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"error": "account_unavailable"})
			return
		}
		c.Set(ctxKeyAccount, acct)
		c.Set(ctxKeyInstallation, inst)
		externalKeys := svc.ResolvedExternalAPIKeysForInstallation(c.Request.Context(), inst.ID)
		ctx := BindInstallationContext(c.Request.Context(), svc, inst, externalKeys, true)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}

// tryAccountCookie returns the resolved account for a valid session cookie, or
// nil when the cookie is absent, login is disabled, or the session is invalid.
func tryAccountCookie(c *gin.Context, svc *auth.Service) *auth.Account {
	acct, _ := tryAccountCookieDetails(c, svc)
	return acct
}

func tryAccountCookieDetails(c *gin.Context, svc *auth.Service) (*auth.Account, *auth.LoginSession) {
	if !svc.LoginEnabled() {
		return nil, nil
	}
	cookie, err := c.Cookie(auth.LoginSessionCookieName)
	if err != nil || cookie == "" {
		return nil, nil
	}
	acct, session, err := svc.VerifyLoginSessionDetails(c.Request.Context(), strings.TrimSpace(cookie))
	if err != nil {
		observability.FromGin(c).Debug("Account session verify failed", "err", err)
		return nil, nil
	}
	return acct, session
}

// AccountFrom retrieves the account set by WithAccountCookie. Returns nil for
// unauthenticated requests.
func AccountFrom(c *gin.Context) *auth.Account {
	v, ok := c.Get(ctxKeyAccount)
	if !ok {
		return nil
	}
	acct, _ := v.(*auth.Account)
	return acct
}
