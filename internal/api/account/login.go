// Package account provides the self-service (aiand-key) dashboard login
// surface mounted under /account/v1 in selfserve deployment mode.
package account

import (
	"errors"
	"net"
	"net/http"
	"os"
	"time"

	"workweave/router/internal/auth"
	"workweave/router/internal/observability"

	"github.com/gin-gonic/gin"
)

// remotePeerIP returns the immediate TCP peer's IP so per-IP login rate
// limiting can't be bypassed by spoofing X-Forwarded-For.
func remotePeerIP(c *gin.Context) string {
	addr := c.Request.RemoteAddr
	if host, _, err := net.SplitHostPort(addr); err == nil {
		return host
	}
	return addr
}

type loginRequest struct {
	Key string `json:"key"`
}

type loginResponse struct {
	OK        bool      `json:"ok"`
	ExpiresAt time.Time `json:"expires_at"`
}

type meResponse struct {
	Authenticated bool   `json:"authenticated"`
	AccountID     string `json:"account_id,omitempty"`
	DisplayName   string `json:"display_name,omitempty"`
}

// LoginHandler validates the presented aiand sk- key, creates-or-returns the
// user's account + installation, and sets a revocable HttpOnly session cookie.
func LoginHandler(authSvc *auth.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !authSvc.LoginEnabled() {
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{
				"error": "account_login_disabled",
				"hint":  "Self-service login is not wired on this deployment.",
			})
			return
		}
		var req loginRequest
		if err := c.ShouldBindJSON(&req); err != nil || req.Key == "" {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "missing_key"})
			return
		}
		// Use raw TCP peer (not c.ClientIP) — X-Forwarded-For is attacker-controlled.
		peerIP := remotePeerIP(c)
		if authSvc.LoginRateLimited(peerIP) {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{"error": "too_many_attempts"})
			return
		}
		acct, _, token, expiresAt, err := authSvc.LoginWithKey(c.Request.Context(), req.Key)
		if err != nil {
			handleLoginError(c, authSvc, err, peerIP)
			return
		}
		authSvc.ClearLoginFailures(peerIP)
		setAccountSessionCookie(c, token, expiresAt)
		observability.FromGin(c).Info("Account login succeeded", "account_id", acct.ID, "remote_ip", peerIP)
		c.JSON(http.StatusOK, loginResponse{OK: true, ExpiresAt: expiresAt})
	}
}

func handleLoginError(c *gin.Context, authSvc *auth.Service, err error, peerIP string) {
	logger := observability.FromGin(c)
	switch {
	case errors.Is(err, auth.ErrKeyInvalid):
		logger.Info("Account login rejected: invalid key", "remote_ip", peerIP)
		authSvc.NoteLoginFailure(peerIP)
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid_key"})
	case errors.Is(err, auth.ErrKeyInsufficientCredits):
		logger.Info("Account login rejected: insufficient credits", "remote_ip", peerIP)
		c.AbortWithStatusJSON(http.StatusPaymentRequired, gin.H{"error": "insufficient_credits"})
	case errors.Is(err, auth.ErrLoginRateLimited):
		c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{"error": "too_many_attempts"})
	case errors.Is(err, auth.ErrKeyUnavailable):
		logger.Error("Account login failed: aiand unavailable", "err", err)
		c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"error": "key_validation_unavailable"})
	default:
		logger.Error("Account login failed", "err", err)
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "login_failed"})
	}
}

func LogoutHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		clearAccountSessionCookie(c)
		c.JSON(http.StatusOK, gin.H{"ok": true})
	}
}

func MeHandler(authSvc *auth.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !authSvc.LoginEnabled() {
			c.JSON(http.StatusOK, meResponse{Authenticated: false})
			return
		}
		acct, err := verifyAccountCookie(c, authSvc)
		if err != nil {
			clearAccountSessionCookie(c)
			c.JSON(http.StatusOK, meResponse{Authenticated: false})
			return
		}
		resp := meResponse{Authenticated: true, AccountID: acct.ID}
		if acct.DisplayName != nil {
			resp.DisplayName = *acct.DisplayName
		}
		c.JSON(http.StatusOK, resp)
	}
}

// cookieSecure controls whether account session cookies are minted with the
// Secure flag (same policy as the admin cookie).
var cookieSecure = os.Getenv("ROUTER_COOKIE_INSECURE") != "true"

func setAccountSessionCookie(c *gin.Context, token string, expiresAt time.Time) {
	maxAge := int(time.Until(expiresAt).Seconds())
	if maxAge < 0 {
		maxAge = 0
	}
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(auth.LoginSessionCookieName, token, maxAge, "/", "", cookieSecure, true)
}

func clearAccountSessionCookie(c *gin.Context) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(auth.LoginSessionCookieName, "", -1, "/", "", cookieSecure, true)
}

func verifyAccountCookie(c *gin.Context, authSvc *auth.Service) (*auth.Account, error) {
	cookie, err := c.Cookie(auth.LoginSessionCookieName)
	if err != nil || cookie == "" {
		return nil, auth.ErrLoginSessionInvalid
	}
	return authSvc.VerifyLoginSession(c.Request.Context(), cookie)
}
