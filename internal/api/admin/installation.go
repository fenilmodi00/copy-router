package admin

import (
	"net/http"

	"workweave/router/internal/auth"
	"workweave/router/internal/server/middleware"

	"github.com/gin-gonic/gin"
)

// resolveInstallation returns the installation dashboard operations act
// on: the caller's own (cookie-authed account session or rk_-keyed request).
// Writes a 401 when no identity is present; callers return when ok == false.
func resolveInstallation(c *gin.Context, authSvc *auth.Service) (*auth.Installation, bool) {
	if installation := middleware.InstallationFrom(c); installation != nil {
		return installation, true
	}
	c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid_key"})
	return nil, false
}
