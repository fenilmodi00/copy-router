package admin_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"weave-os/router/internal/api/admin"
	"weave-os/router/internal/auth"
	"weave-os/router/internal/flags"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fastModeInstallationRepo records the last fast-mode list written.
type fastModeInstallationRepo struct {
	stored     []string
	externalID string
}

var errFastModeRepoNotUsed = errors.New("not used")

func (*fastModeInstallationRepo) Create(context.Context, auth.CreateInstallationParams) (*auth.Installation, error) {
	return nil, errFastModeRepoNotUsed
}
func (*fastModeInstallationRepo) Get(context.Context, string, string) (*auth.Installation, error) {
	return nil, errFastModeRepoNotUsed
}
func (*fastModeInstallationRepo) ListForExternalID(context.Context, string) ([]*auth.Installation, error) {
	return nil, errFastModeRepoNotUsed
}
func (*fastModeInstallationRepo) SoftDelete(context.Context, string, string) error {
	return errFastModeRepoNotUsed
}
func (*fastModeInstallationRepo) MarkFirstRequestServed(context.Context, string) error {
	return errFastModeRepoNotUsed
}
func (r *fastModeInstallationRepo) UpdateFastModeModels(_ context.Context, externalID, _ string, models []string) error {
	r.stored = append([]string{}, models...)
	r.externalID = externalID
	return nil
}
func (*fastModeInstallationRepo) UpdateExcludedModels(context.Context, string, string, []string) error {
	return errFastModeRepoNotUsed
}
func (*fastModeInstallationRepo) UpdateAllowedModels(context.Context, string, string, []string) error {
	return errFastModeRepoNotUsed
}
func (*fastModeInstallationRepo) UpdateExcludedProviders(context.Context, string, string, []string) error {
	return errFastModeRepoNotUsed
}
func (*fastModeInstallationRepo) UpdateRoutingPreference(context.Context, string, string, *float64) error {
	return errFastModeRepoNotUsed
}
func (*fastModeInstallationRepo) UpdateUsageBypass(context.Context, string, string, bool, *float64) error {
	return errFastModeRepoNotUsed
}
func (*fastModeInstallationRepo) UpdateSubscriptionRoutingDisabled(context.Context, string, string, bool) error {
	return errFastModeRepoNotUsed
}
func (*fastModeInstallationRepo) UpdateContentCaptureMode(context.Context, string, string, *string) error {
	return errFastModeRepoNotUsed
}
func (*fastModeInstallationRepo) UpdateHideTerminalSurfaces(context.Context, string, string, bool) error {
	return errFastModeRepoNotUsed
}
func (*fastModeInstallationRepo) UpdateShowModelSelectionReasoning(context.Context, string, string, bool) error {
	return errFastModeRepoNotUsed
}
func (*fastModeInstallationRepo) UpdateFlagOverrides(context.Context, string, string, flags.Overrides) error {
	return errFastModeRepoNotUsed
}

type fastModeModelsBody struct {
	Available []struct {
		Model    string `json:"model"`
		Provider string `json:"provider"`
		FastMode bool   `json:"fast_mode"`
	} `json:"available"`
	FastMode []string `json:"fast_mode"`
}

func fastModeEngine(repo *fastModeInstallationRepo, installation *auth.Installation) *gin.Engine {
	gin.SetMode(gin.TestMode)
	authSvc := auth.NewService(repo, nil, nil, nil, auth.NoOpAPIKeyCache{}, nil, func() time.Time { return time.Unix(0, 0) })
	inject := func(c *gin.Context) { c.Set("router_installation", installation) }
	engine := gin.New()
	engine.GET("/admin/v1/fast-mode-models", inject, admin.GetFastModeModelsHandler(authSvc))
	engine.PUT("/admin/v1/fast-mode-models", inject, admin.UpdateFastModeModelsHandler(authSvc))
	return engine
}

func putFastModeModels(t *testing.T, engine *gin.Engine, models []string) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(map[string][]string{"fast_mode": models})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPut, "/admin/v1/fast-mode-models", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	return rec
}

func TestGetFastModeModelsHandler_ListsOnlyFastCapableCatalog(t *testing.T) {
	engine := fastModeEngine(&fastModeInstallationRepo{}, &auth.Installation{ID: "inst-1"})

	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/admin/v1/fast-mode-models", nil))
	require.Equal(t, http.StatusOK, rec.Code)

	var got fastModeModelsBody
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	assert.Empty(t, got.Available, "no AIand roster row carries a fast tier")
	assert.Empty(t, got.FastMode)
}

func TestUpdateFastModeModelsHandler_PersistsFastCapableModels(t *testing.T) {
	repo := &fastModeInstallationRepo{}
	engine := fastModeEngine(repo, &auth.Installation{ID: "inst-1", ExternalID: "org-1"})

	// No roster row has a fast tier, so any non-empty opt-in is refused.
	rec := putFastModeModels(t, engine, []string{"zai-org/glm-5.3"})
	require.Equal(t, http.StatusBadRequest, rec.Code)

	// Clearing the list stays allowed.
	rec = putFastModeModels(t, engine, nil)
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Empty(t, repo.stored)
	assert.Equal(t, "org-1", repo.externalID)
}

func TestUpdateFastModeModelsHandler_RejectsModelWithoutFastTier(t *testing.T) {
	repo := &fastModeInstallationRepo{}
	engine := fastModeEngine(repo, &auth.Installation{ID: "inst-1", ExternalID: "org-1"})

	rec := putFastModeModels(t, engine, []string{"deepseek-ai/deepseek-v4-pro"})
	require.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Nil(t, repo.stored, "nothing persisted on validation failure")

	var body map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Contains(t, body["error"], "deepseek-ai/deepseek-v4-pro")
}

func TestUpdateFastModeModelsHandler_EmptyListClears(t *testing.T) {
	repo := &fastModeInstallationRepo{stored: []string{"zai-org/glm-5.3"}}
	engine := fastModeEngine(repo, &auth.Installation{ID: "inst-1", ExternalID: "org-1"})

	rec := putFastModeModels(t, engine, []string{})
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, []string{}, repo.stored)
}
