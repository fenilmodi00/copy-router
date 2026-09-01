package auth

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"workweave/router/internal/providers"
)

// stubInstallRepo is the minimal InstallationRepository fake: it implements
// only the methods LoginWithKey/EnsureAccountInstallation use, embedding the
// nil interface for the rest (nil-interface method calls panic, which is a
// test failure signal — no valid test path reaches them).
type stubInstallRepo struct {
	InstallationRepository // nil embedded interface
	byExternalID           map[string]*Installation
}

func (s *stubInstallRepo) ListForExternalID(ctx context.Context, externalID string) ([]*Installation, error) {
	if inst, ok := s.byExternalID[externalID]; ok {
		return []*Installation{inst}, nil
	}
	return nil, nil
}

func (s *stubInstallRepo) Get(ctx context.Context, externalID, id string) (*Installation, error) {
	if inst, ok := s.byExternalID[externalID]; ok && inst.ID == id {
		return inst, nil
	}
	return nil, ErrLoginSessionInvalid
}

func (s *stubInstallRepo) Create(ctx context.Context, params CreateInstallationParams) (*Installation, error) {
	inst := &Installation{ID: "inst-" + params.ExternalID, ExternalID: params.ExternalID, Name: params.Name}
	s.byExternalID[params.ExternalID] = inst
	return inst, nil
}

// fakeVerifier returns a canned identity or a canned error.
type fakeVerifier struct {
	identity *AiandIdentity
	err      error
}

func (f *fakeVerifier) Validate(ctx context.Context, rawKey string) (*AiandIdentity, error) {
	return f.identity, f.err
}

// mutableClock is a Clock whose returned time can be advanced mid-test.
type mutableClock struct {
	t time.Time
}

func (c *mutableClock) now() time.Time {
	return c.t
}

func newLoginTestService(t *testing.T, repo *inMemoryAccountRepo) (*Service, *mutableClock, *stubInstallRepo) {
	t.Helper()
	return newLoginTestServiceWithExternal(t, repo, nil)
}

func newLoginTestServiceWithExternal(t *testing.T, repo *inMemoryAccountRepo, external ExternalAPIKeyRepository) (*Service, *mutableClock, *stubInstallRepo) {
	t.Helper()
	clock := &mutableClock{t: time.Date(2026, 4, 27, 12, 0, 0, 0, time.UTC)}
	insts := &stubInstallRepo{byExternalID: map[string]*Installation{}}
	svc := NewService(insts, nil, external, nil, NoOpAPIKeyCache{}, nil, clock.now).
		WithAccountRepos(repo, repo)
	return svc, clock, insts
}

func TestLoginWithKey_ValidKeyCreatesAccountAndInstallation(t *testing.T) {
	repo := newInMemoryAccountRepo()
	svc, _, insts := newLoginTestService(t, repo)
	svc.WithKeyVerifier(&fakeVerifier{identity: &AiandIdentity{
		UserID:         "user-aiand-1",
		OrganizationID: "org-1",
	}})

	acct, inst, token, _, err := svc.LoginWithKey(context.Background(), "sk-valid")
	require.NoError(t, err)
	require.NotNil(t, acct)
	assert.Equal(t, "user-aiand-1", acct.AiandUserID)
	require.NotNil(t, inst)
	assert.Equal(t, acct.ID, inst.ExternalID, "the account id must double as the installation external_id")
	assert.NotEmpty(t, token, "a successful login must mint a session token")
	assert.Len(t, repo.accounts, 1)
	assert.Len(t, insts.byExternalID, 1, "a first login must create exactly one installation")
}

type captureExternalKeyRepo struct {
	lastProvider string
	called       bool
}

func (r *captureExternalKeyRepo) Create(ctx context.Context, params CreateExternalAPIKeyParams) (*ExternalAPIKey, error) {
	r.lastProvider = params.Provider
	r.called = true
	return &ExternalAPIKey{ID: "ek-test", Provider: params.Provider}, nil
}

func (r *captureExternalKeyRepo) GetForInstallation(ctx context.Context, installationID string) ([]*ExternalAPIKey, error) {
	return nil, nil
}

func (r *captureExternalKeyRepo) SoftDeleteByProvider(ctx context.Context, installationID, provider string) error {
	return nil
}

func (r *captureExternalKeyRepo) SoftDelete(ctx context.Context, installationID, id string) error {
	return nil
}

func (r *captureExternalKeyRepo) UpdateModelAliases(ctx context.Context, installationID, id string, aliases map[string]string) (*ExternalAPIKey, error) {
	return nil, nil
}

func (r *captureExternalKeyRepo) MarkUsed(ctx context.Context, id string) error {
	return nil
}

func TestLoginWithKey_StoresAiandKeyForInstallation(t *testing.T) {
	repo := newInMemoryAccountRepo()
	ext := &captureExternalKeyRepo{}
	svc, _, _ := newLoginTestServiceWithExternal(t, repo, ext)
	svc.WithKeyVerifier(&fakeVerifier{identity: &AiandIdentity{
		UserID:         "user-aiand-1",
		OrganizationID: "org-1",
	}})

	_, _, _, _, err := svc.LoginWithKey(context.Background(), "sk-login-key")
	require.NoError(t, err)
	assert.True(t, ext.called)
	assert.Equal(t, providers.ProviderAiand, ext.lastProvider)
}

func TestLoginWithKey_InvalidKeyIsRejected(t *testing.T) {
	repo := newInMemoryAccountRepo()
	svc, _, _ := newLoginTestService(t, repo)
	svc.WithKeyVerifier(&fakeVerifier{err: ErrKeyInvalid})

	acct, _, _, _, err := svc.LoginWithKey(context.Background(), "sk-invalid")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrKeyInvalid)
	assert.Nil(t, acct)
	assert.Len(t, repo.accounts, 0, "an invalid key must not create an account")
}

func TestLoginWithKey_SameUserReturnsSameAccountAndInstallation(t *testing.T) {
	repo := newInMemoryAccountRepo()
	svc, clock, insts := newLoginTestService(t, repo)
	svc.WithKeyVerifier(&fakeVerifier{identity: &AiandIdentity{
		UserID:         "user-aiand-1",
		OrganizationID: "org-1",
	}})

	first, firstInst, _, _, err := svc.LoginWithKey(context.Background(), "sk-one")
	require.NoError(t, err)
	clock.t = clock.t.Add(time.Hour)
	second, secondInst, _, _, err := svc.LoginWithKey(context.Background(), "sk-two")
	require.NoError(t, err)
	assert.Equal(t, first.ID, second.ID, "a different aiand key for the same aiand user must map to one account")
	assert.Equal(t, firstInst.ID, secondInst.ID, "and to one installation")
	assert.Len(t, repo.accounts, 1)
	assert.Len(t, insts.byExternalID, 1, "returning login must not create a second installation")
}

func TestLoginWithKey_UnavailableIsPropagated(t *testing.T) {
	repo := newInMemoryAccountRepo()
	svc, _, _ := newLoginTestService(t, repo)
	svc.WithKeyVerifier(&fakeVerifier{err: ErrKeyUnavailable})

	_, _, _, _, err := svc.LoginWithKey(context.Background(), "sk-any")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrKeyUnavailable)
	assert.Len(t, repo.accounts, 0)
}

func TestLoginSession_IssueAndVerifyRoundTrip(t *testing.T) {
	repo := newInMemoryAccountRepo()
	svc, _, _ := newLoginTestService(t, repo)
	acct := &Account{ID: "acct-1", AiandUserID: "user-aiand-1"}
	repo.byID[acct.ID] = acct
	inst := &Installation{ID: "inst-1", ExternalID: acct.ID}

	token, _, err := svc.IssueLoginSession(context.Background(), acct, inst)
	require.NoError(t, err)
	require.NotEmpty(t, token)

	got, err := svc.VerifyLoginSession(context.Background(), token)
	require.NoError(t, err)
	assert.Equal(t, acct.ID, got.ID, "the session must resolve to the account that issued it")
}

func TestInstallationForAccountSession_UsesCachedInstallationID(t *testing.T) {
	repo := newInMemoryAccountRepo()
	svc, _, insts := newLoginTestService(t, repo)
	acct := &Account{ID: "acct-1", AiandUserID: "user-aiand-1"}
	repo.byID[acct.ID] = acct
	cached := &Installation{ID: "inst-cached", ExternalID: acct.ID}
	insts.byExternalID[acct.ID] = cached

	inst, err := svc.InstallationForAccountSession(context.Background(), acct, &LoginSession{InstallationID: cached.ID})
	require.NoError(t, err)
	assert.Equal(t, cached.ID, inst.ID)
}

func TestInstallationForAccountSession_RepairsMissingCache(t *testing.T) {
	repo := newInMemoryAccountRepo()
	svc, _, insts := newLoginTestService(t, repo)
	acct := &Account{ID: "acct-1", AiandUserID: "user-aiand-1"}
	repo.byID[acct.ID] = acct

	inst, err := svc.InstallationForAccountSession(context.Background(), acct, &LoginSession{})
	require.NoError(t, err)
	require.NotNil(t, inst)
	assert.Len(t, insts.byExternalID, 1, "repair path must ensure installation exists")
}

func TestLoginSession_InvalidTokenRejected(t *testing.T) {
	repo := newInMemoryAccountRepo()
	svc, _, _ := newLoginTestService(t, repo)

	_, err := svc.VerifyLoginSession(context.Background(), "bogus-token")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrLoginSessionInvalid)
}
