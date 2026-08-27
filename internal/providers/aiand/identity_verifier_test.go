package aiand_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"workweave/router/internal/auth"
	aiand "workweave/router/internal/providers/aiand"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestKeyVerifierValidateSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/me", r.URL.Path, "probe must hit GET /api/v1/me")
		assert.Equal(t, "Bearer sk-test", r.Header.Get("Authorization"), "probe must forward the raw key as Authorization: Bearer")
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"user_id":"u-1","organization_id":"o-1","plan":"pro"}`))
	}))
	defer srv.Close()

	v := &aiand.KeyVerifier{Client: srv.Client(), BaseURL: srv.URL}
	identity, err := v.Validate(context.Background(), "sk-test")
	require.NoError(t, err)
	require.NotNil(t, identity)
	assert.Equal(t, "u-1", identity.UserID)
	assert.Equal(t, "o-1", identity.OrganizationID)
	assert.Equal(t, "pro", identity.Plan)
}

func TestKeyVerifierValidateInvalidKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":"invalid key"}`))
	}))
	defer srv.Close()

	v := &aiand.KeyVerifier{Client: srv.Client(), BaseURL: srv.URL}
	_, err := v.Validate(context.Background(), "sk-bad")
	require.Error(t, err)
	assert.ErrorIs(t, err, auth.ErrKeyInvalid)
}

func TestKeyVerifierValidateInsufficientCredits(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusPaymentRequired)
		w.Write([]byte(`{"error":"insufficient balance"}`))
	}))
	defer srv.Close()

	v := &aiand.KeyVerifier{Client: srv.Client(), BaseURL: srv.URL}
	_, err := v.Validate(context.Background(), "sk-broke")
	require.Error(t, err)
	assert.ErrorIs(t, err, auth.ErrKeyInsufficientCredits)
}

func TestKeyVerifierValidateUpstreamDown(t *testing.T) {
	v := &aiand.KeyVerifier{Client: &http.Client{}, BaseURL: "http://127.0.0.1:1"}
	_, err := v.Validate(context.Background(), "sk-any")
	require.Error(t, err)
	assert.ErrorIs(t, err, auth.ErrKeyUnavailable)
}
