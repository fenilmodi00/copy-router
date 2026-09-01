package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"workweave/router/internal/auth"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type sessionAccountRepo struct {
	accounts map[string]*auth.Account
	sessions map[string]*auth.LoginSession
	nextSess int
}

func newSessionAccountRepo() *sessionAccountRepo {
	return &sessionAccountRepo{
		accounts: map[string]*auth.Account{},
		sessions: map[string]*auth.LoginSession{},
	}
}

func (r *sessionAccountRepo) UpsertByAiandUser(ctx context.Context, p auth.AccountUpsertParams) (*auth.Account, error) {
	acct := &auth.Account{ID: p.ID, AiandUserID: p.AiandUserID}
	r.accounts[acct.ID] = acct
	return acct, nil
}

func (r *sessionAccountRepo) GetByAiandUserID(ctx context.Context, aiandUserID string) (*auth.Account, error) {
	return nil, errors.New("not implemented")
}

func (r *sessionAccountRepo) GetByID(ctx context.Context, id string) (*auth.Account, error) {
	acct, ok := r.accounts[id]
	if !ok {
		return nil, errors.New("missing account")
	}
	return acct, nil
}

func (r *sessionAccountRepo) SoftDelete(ctx context.Context, id string) error {
	return nil
}

func (r *sessionAccountRepo) Insert(ctx context.Context, s auth.LoginSession) (*auth.LoginSession, error) {
	r.nextSess++
	row := s
	row.ID = "sess-" + string(rune('0'+r.nextSess))
	r.sessions[s.TokenHash] = &row
	return &row, nil
}

func (r *sessionAccountRepo) GetActiveByTokenHash(ctx context.Context, tokenHash string) (*auth.LoginSession, error) {
	s, ok := r.sessions[tokenHash]
	if !ok {
		return nil, errors.New("missing session")
	}
	return s, nil
}

func (r *sessionAccountRepo) TouchLastSeen(ctx context.Context, accountID, sessionID string) error {
	return nil
}

func (r *sessionAccountRepo) RevokeByID(ctx context.Context, accountID, sessionID string) error {
	return nil
}

func (r *sessionAccountRepo) RevokeAllForAccount(ctx context.Context, accountID string) error {
	return nil
}

func (r *sessionAccountRepo) ListForAccount(ctx context.Context, accountID string) ([]auth.LoginSession, error) {
	return nil, nil
}

type accountSessionInstallRepo struct {
	auth.InstallationRepository
	byExternalID map[string]*auth.Installation
	getCalls     int
	listCalls    int
	ensureCalls  int
}

func (r *accountSessionInstallRepo) Get(_ context.Context, externalID, id string) (*auth.Installation, error) {
	r.getCalls++
	if inst, ok := r.byExternalID[externalID]; ok && inst.ID == id {
		return inst, nil
	}
	return nil, errors.New("installation not found")
}

func (r *accountSessionInstallRepo) ListForExternalID(_ context.Context, externalID string) ([]*auth.Installation, error) {
	r.listCalls++
	if inst, ok := r.byExternalID[externalID]; ok {
		return []*auth.Installation{inst}, nil
	}
	return nil, nil
}

func (r *accountSessionInstallRepo) Create(_ context.Context, params auth.CreateInstallationParams) (*auth.Installation, error) {
	r.ensureCalls++
	inst := &auth.Installation{ID: "inst-created", ExternalID: params.ExternalID, Name: params.Name}
	r.byExternalID[params.ExternalID] = inst
	return inst, nil
}

// WithAccountCookie must load the installation cached on the session row instead
// of calling EnsureAccountInstallation on every dashboard request.
func TestWithAccountCookie_UsesCachedSessionInstallation(t *testing.T) {
	gin.SetMode(gin.TestMode)

	inst := &auth.Installation{ID: "inst-cached", ExternalID: "acct-1", Name: "tenant"}
	installRepo := &accountSessionInstallRepo{byExternalID: map[string]*auth.Installation{"acct-1": inst}}
	repo := newSessionAccountRepo()
	acct := &auth.Account{ID: "acct-1", AiandUserID: "user-1"}
	repo.accounts[acct.ID] = acct

	svc := auth.NewService(installRepo, nil, nil, nil, auth.NoOpAPIKeyCache{}, nil, func() time.Time { return time.Now() }).
		WithAccountRepos(repo, repo).
		WithKeyVerifier(authLoginVerifier{})

	token, _, err := svc.IssueLoginSession(context.Background(), acct, inst)
	require.NoError(t, err)

	engine := gin.New()
	engine.GET("/v1/ping", WithAccountCookie(svc), func(c *gin.Context) {
		got := InstallationFrom(c)
		require.NotNil(t, got)
		assert.Equal(t, "inst-cached", got.ID)
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/ping", nil)
	req.AddCookie(&http.Cookie{Name: auth.LoginSessionCookieName, Value: token})
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, 1, installRepo.getCalls, "cached session must load installation by id")
	assert.Zero(t, installRepo.listCalls, "EnsureAccountInstallation list must not run on hot path")
	assert.Zero(t, installRepo.ensureCalls, "EnsureAccountInstallation create must not run on hot path")
}

type authLoginVerifier struct{}

func (authLoginVerifier) Validate(context.Context, string) (*auth.AiandIdentity, error) {
	return &auth.AiandIdentity{UserID: "user-1", OrganizationID: "org-1"}, nil
}
