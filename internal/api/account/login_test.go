package account_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"workweave/router/internal/api/account"
	"workweave/router/internal/auth"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The fixtures below duplicate the auth-package in-memory fakes because those
// live in package-internal test files and can't be imported. Keep them minimal
// — the auth package's own tests already assert repo/verifier semantics; here
// they only need to make LoginWithKey + VerifyLoginSession behave.

type fakeKeyVerifier struct {
	identity *auth.AiandIdentity
	err      error
}

func (f *fakeKeyVerifier) Validate(ctx context.Context, rawKey string) (*auth.AiandIdentity, error) {
	if rawKey == "sk-bad" {
		return nil, auth.ErrKeyInvalid
	}
	if f.err != nil {
		return nil, f.err
	}
	if f.identity == nil {
		f.identity = &auth.AiandIdentity{UserID: "u-1", OrganizationID: "o-1"}
	}
	return f.identity, nil
}

// fakeInstallRepo implements only ListForExternalID + Create (what the login +
// cookie middleware need); the remaining InstallationRepository methods embed
// the interface so they never resolve (the handlers + login service don't call
// them).
type fakeInstallRepo struct {
	auth.InstallationRepository
	byExternalID map[string]*auth.Installation
}

type fakeCreateParams = auth.CreateInstallationParams

func (f *fakeInstallRepo) ListForExternalID(ctx context.Context, externalID string) ([]*auth.Installation, error) {
	if inst, ok := f.byExternalID[externalID]; ok {
		return []*auth.Installation{inst}, nil
	}
	return nil, nil
}

func (f *fakeInstallRepo) Create(ctx context.Context, params fakeCreateParams) (*auth.Installation, error) {
	inst := &auth.Installation{ID: "inst-" + params.ExternalID, ExternalID: params.ExternalID, Name: params.Name}
	f.byExternalID[params.ExternalID] = inst
	return inst, nil
}

// fakeAccountRepo implements auth.AccountRepository + auth.LoginSessionRepository
// in memory.
type fakeAccountRepo struct {
	accounts map[string]*auth.Account
	sessions map[string]*auth.LoginSession
	byID     map[string]*auth.Account
}

func newFakeAccountRepo() *fakeAccountRepo {
	return &fakeAccountRepo{
		accounts: map[string]*auth.Account{},
		sessions: map[string]*auth.LoginSession{},
		byID:     map[string]*auth.Account{},
	}
}

func (r *fakeAccountRepo) UpsertByAiandUser(ctx context.Context, p auth.AccountUpsertParams) (*auth.Account, error) {
	if existing, ok := r.accounts[p.AiandUserID]; ok {
		return existing, nil
	}
	acct := &auth.Account{
		ID:                  p.ID,
		AiandUserID:         p.AiandUserID,
		AiandOrganizationID: p.AiandOrganizationID,
		CreatedAt:           time.Now(),
		LastLoginAt:         time.Now(),
	}
	r.accounts[p.AiandUserID] = acct
	r.byID[acct.ID] = acct
	return acct, nil
}

func (r *fakeAccountRepo) GetByAiandUserID(ctx context.Context, aiandUserID string) (*auth.Account, error) {
	return r.accounts[aiandUserID], nil
}

func (r *fakeAccountRepo) GetByID(ctx context.Context, id string) (*auth.Account, error) {
	return r.byID[id], nil
}

func (r *fakeAccountRepo) SoftDelete(ctx context.Context, id string) error { return nil }

func (r *fakeAccountRepo) Insert(ctx context.Context, s auth.LoginSession) (*auth.LoginSession, error) {
	s.ID = auth.GenerateID("sess")
	r.sessions[s.TokenHash] = &s
	return &s, nil
}

func (r *fakeAccountRepo) GetActiveByTokenHash(ctx context.Context, tokenHash string) (*auth.LoginSession, error) {
	return r.sessions[tokenHash], nil
}

func (r *fakeAccountRepo) TouchLastSeen(ctx context.Context, accountID, sessionID string) error {
	return nil
}

func (r *fakeAccountRepo) RevokeByID(ctx context.Context, accountID, sessionID string) error {
	return nil
}

func (r *fakeAccountRepo) RevokeAllForAccount(ctx context.Context, accountID string) error {
	return nil
}

func (r *fakeAccountRepo) ListForAccount(ctx context.Context, accountID string) ([]auth.LoginSession, error) {
	return nil, nil
}

func newAccountTestService() (*auth.Service, *fakeInstallRepo, *fakeAccountRepo) {
	insts := &fakeInstallRepo{byExternalID: map[string]*auth.Installation{}}
	acctRepo := newFakeAccountRepo()
	svc := auth.NewService(insts, nil, nil, nil, auth.NoOpAPIKeyCache{}, nil, time.Now).
		WithAccountRepos(acctRepo, acctRepo).
		WithKeyVerifier(&fakeKeyVerifier{})
	return svc, insts, acctRepo
}

func TestLoginHandler_ValidKeySetsCookie(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc, _, _ := newAccountTestService()
	r := gin.New()
	r.POST("/account/v1/login", account.LoginHandler(svc))
	body, _ := json.Marshal(map[string]string{"key": "sk-valid"})
	req := httptest.NewRequest(http.MethodPost, "/account/v1/login", bytes.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	resp := struct {
		OK        bool   `json:"ok"`
		ExpiresAt string `json:"expires_at"`
	}{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.True(t, resp.OK)
	assert.NotEmpty(t, resp.ExpiresAt)
	cookies := w.Result().Cookies()
	require.Len(t, cookies, 1, "a successful login must set the session cookie")
	assert.Equal(t, auth.LoginSessionCookieName, cookies[0].Name)
	assert.True(t, cookies[0].HttpOnly, "the session cookie must be HttpOnly")
}

func TestLoginHandler_InvalidKeyIs401(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc, _, acctRepo := newAccountTestService()
	r := gin.New()
	r.POST("/account/v1/login", account.LoginHandler(svc))
	body, _ := json.Marshal(map[string]string{"key": "sk-bad"})
	req := httptest.NewRequest(http.MethodPost, "/account/v1/login", bytes.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Empty(t, w.Result().Cookies(), "no cookie on failed login")
	assert.Empty(t, acctRepo.accounts, "an invalid key must not create an account")
}

func TestMeHandler_NoCookieIsUnauthenticated(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc, _, _ := newAccountTestService()
	r := gin.New()
	r.GET("/account/v1/me", account.MeHandler(svc))
	req := httptest.NewRequest(http.MethodGet, "/account/v1/me", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp struct {
		Authenticated bool `json:"authenticated"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.False(t, resp.Authenticated)
}
