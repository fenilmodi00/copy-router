package auth_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"workweave/router/internal/auth"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type playgroundKeyRepo struct {
	mu       sync.Mutex
	keys     map[string][]*auth.APIKey
	created  []auth.CreateAPIKeyParams
	nextID   int
}

func (r *playgroundKeyRepo) Create(_ context.Context, params auth.CreateAPIKeyParams) (*auth.APIKey, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.created = append(r.created, params)
	r.nextID++
	key := &auth.APIKey{
		ID:             params.ExternalID,
		InstallationID: params.InstallationID,
		ExternalID:     params.ExternalID,
		Name:           params.Name,
		Scope:          params.Scope,
	}
	if r.keys == nil {
		r.keys = make(map[string][]*auth.APIKey)
	}
	r.keys[params.InstallationID] = append(r.keys[params.InstallationID], key)
	return key, nil
}

func (r *playgroundKeyRepo) GetActiveByHashWithInstallation(context.Context, string) (*auth.APIKey, *auth.Installation, error) {
	return nil, nil, nil
}

func (r *playgroundKeyRepo) ListForInstallation(_ context.Context, installationID string) ([]*auth.APIKey, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]*auth.APIKey(nil), r.keys[installationID]...), nil
}

func (r *playgroundKeyRepo) MarkUsed(context.Context, string) error { return nil }

func (r *playgroundKeyRepo) SoftDelete(context.Context, string, string) (int64, error) {
	return 0, nil
}

func TestResolvePlaygroundAPIKeyID_ReusesNamedKey(t *testing.T) {
	const installID = "11111111-1111-1111-1111-111111111111"
	name := auth.PlaygroundAPIKeyName
	repo := &playgroundKeyRepo{
		keys: map[string][]*auth.APIKey{
			installID: {{
				ID:             "key-playground",
				InstallationID: installID,
				Name:           &name,
				Scope:          auth.ScopeRouting,
			}},
		},
	}
	clock := func() time.Time { return time.Now() }
	svc := auth.NewService(nil, repo, nil, nil, auth.NoOpAPIKeyCache{}, nil, clock)

	id, err := svc.ResolvePlaygroundAPIKeyID(context.Background(), installID)
	require.NoError(t, err)
	assert.Equal(t, "key-playground", id)
	assert.Empty(t, repo.created)
}

func TestResolvePlaygroundAPIKeyID_CreatesWhenMissing(t *testing.T) {
	const installID = "22222222-2222-2222-2222-222222222222"
	repo := &playgroundKeyRepo{keys: map[string][]*auth.APIKey{}}
	clock := func() time.Time { return time.Now() }
	svc := auth.NewService(nil, repo, nil, nil, auth.NoOpAPIKeyCache{}, nil, clock)

	id, err := svc.ResolvePlaygroundAPIKeyID(context.Background(), installID)
	require.NoError(t, err)
	require.NotEmpty(t, id)
	require.Len(t, repo.created, 1)
	require.NotNil(t, repo.created[0].Name)
	assert.Equal(t, auth.PlaygroundAPIKeyName, *repo.created[0].Name)
}
