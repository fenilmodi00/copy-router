package auth

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// inMemoryAccountRepo implements AccountRepository + LoginSessionRepository so
// the login service can be unit-tested with no DB.
type inMemoryAccountRepo struct {
	accounts map[string]*Account      // key = aiand_user_id
	sessions map[string]*LoginSession // key = token_hash
	byID     map[string]*Account      // key = account id
}

func newInMemoryAccountRepo() *inMemoryAccountRepo {
	return &inMemoryAccountRepo{
		accounts: map[string]*Account{},
		sessions: map[string]*LoginSession{},
		byID:     map[string]*Account{},
	}
}

func (r *inMemoryAccountRepo) UpsertByAiandUser(ctx context.Context, p AccountUpsertParams) (*Account, error) {
	if existing, ok := r.accounts[p.AiandUserID]; ok {
		existing.LastLoginAt = time.Now()
		if p.DisplayName != nil {
			existing.DisplayName = p.DisplayName
		}
		return existing, nil
	}
	acct := &Account{
		ID:                  p.ID,
		AiandUserID:         p.AiandUserID,
		AiandOrganizationID: p.AiandOrganizationID,
		DisplayName:         p.DisplayName,
		CreatedAt:           time.Now(),
		LastLoginAt:         time.Now(),
	}
	r.accounts[p.AiandUserID] = acct
	r.byID[acct.ID] = acct
	return acct, nil
}

func (r *inMemoryAccountRepo) GetByAiandUserID(ctx context.Context, aiandUserID string) (*Account, error) {
	return r.accounts[aiandUserID], nil
}

func (r *inMemoryAccountRepo) GetByID(ctx context.Context, id string) (*Account, error) {
	return r.byID[id], nil
}

func (r *inMemoryAccountRepo) SoftDelete(ctx context.Context, id string) error {
	if acct, ok := r.byID[id]; ok {
		now := time.Now()
		acct.DeletedAt = &now
	}
	return nil
}

func (r *inMemoryAccountRepo) Insert(ctx context.Context, s LoginSession) (*LoginSession, error) {
	s.ID = GenerateID("sess")
	r.sessions[s.TokenHash] = &s
	return &s, nil
}

func (r *inMemoryAccountRepo) GetActiveByTokenHash(ctx context.Context, tokenHash string) (*LoginSession, error) {
	return r.sessions[tokenHash], nil
}

func (r *inMemoryAccountRepo) TouchLastSeen(ctx context.Context, accountID, sessionID string) error {
	return nil
}

func (r *inMemoryAccountRepo) RevokeByID(ctx context.Context, accountID, sessionID string) error {
	for _, s := range r.sessions {
		if s.ID == sessionID {
			now := time.Now()
			s.RevokedAt = &now
		}
	}
	return nil
}

func (r *inMemoryAccountRepo) RevokeAllForAccount(ctx context.Context, accountID string) error {
	for _, s := range r.sessions {
		if s.AccountID == accountID {
			now := time.Now()
			s.RevokedAt = &now
		}
	}
	return nil
}

func (r *inMemoryAccountRepo) ListForAccount(ctx context.Context, accountID string) ([]LoginSession, error) {
	var out []LoginSession
	for _, s := range r.sessions {
		if s.AccountID == accountID {
			out = append(out, *s)
		}
	}
	return out, nil
}

func TestAccountUpsertCreatesOnce(t *testing.T) {
	repo := newInMemoryAccountRepo()
	ctx := context.Background()

	first, err := repo.UpsertByAiandUser(ctx, AccountUpsertParams{
		ID:                  "acct-1",
		AiandUserID:         "user-aiand-1",
		AiandOrganizationID: "org-1",
	})
	require.NoError(t, err)
	assert.Equal(t, "acct-1", first.ID)

	second, err := repo.UpsertByAiandUser(ctx, AccountUpsertParams{
		ID:                  "acct-2",
		AiandUserID:         "user-aiand-1",
		AiandOrganizationID: "org-1",
	})
	require.NoError(t, err)
	// The second upsert passes a different candidate id ("acct-2"), but the
	// prod code must return the EXISTING row's id — this is exactly the
	// concurrent-first-login contract the login service relies on.
	assert.Equal(t, first.ID, second.ID, "upsert must be idempotent on aiand_user_id")
	assert.Len(t, repo.accounts, 1, "two upserts of the same aiand user must yield one account")
}
