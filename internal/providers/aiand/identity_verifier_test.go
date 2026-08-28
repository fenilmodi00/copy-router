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
		assert.Equal(t, "/v1/models", r.URL.Path, "probe must hit GET /v1/models (key-auth)")
		assert.Equal(t, "Bearer sk-test", r.Header.Get("Authorization"), "probe must forward the raw key as Authorization: Bearer")
		w.Header().Set("X-Org-Id", "org-abc")
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"object":"list","data":[]}`))
	}))
	defer srv.Close()

	v := &aiand.KeyVerifier{Client: srv.Client(), IdentityURL: srv.URL}
	identity, err := v.Validate(context.Background(), "sk-test")
	require.NoError(t, err)
	require.NotNil(t, identity)
	assert.Equal(t, "org-abc", identity.UserID)
	assert.Equal(t, "org-abc", identity.OrganizationID)
}

func TestKeyVerifierValidateMissingOrgHeader(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"object":"list","data":[]}`))
	}))
	defer srv.Close()

	v := &aiand.KeyVerifier{Client: srv.Client(), IdentityURL: srv.URL}
	_, err := v.Validate(context.Background(), "sk-test")
	require.Error(t, err)
	assert.ErrorIs(t, err, auth.ErrKeyUnavailable)
}

func TestKeyVerifierValidateInvalidKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":"invalid key"}`))
	}))
	defer srv.Close()

	v := &aiand.KeyVerifier{Client: srv.Client(), IdentityURL: srv.URL}
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

	v := &aiand.KeyVerifier{Client: srv.Client(), IdentityURL: srv.URL}
	_, err := v.Validate(context.Background(), "sk-broke")
	require.Error(t, err)
	assert.ErrorIs(t, err, auth.ErrKeyInsufficientCredits)
}

func TestKeyVerifierValidateUpstreamDown(t *testing.T) {
	v := &aiand.KeyVerifier{Client: &http.Client{}, IdentityURL: "http://127.0.0.1:1"}
	_, err := v.Validate(context.Background(), "sk-any")
	require.Error(t, err)
	assert.ErrorIs(t, err, auth.ErrKeyUnavailable)
}

// Identity and inference URLs are configured separately: AIAND_IDENTITY_URL
// must point at the API root; AIAND_API_URL (…/v1) is for OpenAI-compat only.
func TestKeyVerifierValidateUsesIdentityURLNotInferenceBase(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("X-Org-Id", "org-from-identity-root")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"object":"list","data":[]}`))
	}))
	t.Cleanup(srv.Close)

	v := &aiand.KeyVerifier{Client: srv.Client(), IdentityURL: srv.URL}
	identity, err := v.Validate(context.Background(), "sk-test")
	require.NoError(t, err)
	require.NotNil(t, identity)
	assert.Equal(t, "/v1/models", gotPath)
	assert.Equal(t, "org-from-identity-root", identity.UserID)
}

func TestKeyVerifierValidateInferenceBaseURLDoesNotDoubleV1(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)

	// Mis-wiring the inference base into IdentityURL must not silently strip /v1.
	v := &aiand.KeyVerifier{Client: srv.Client(), IdentityURL: srv.URL + "/v1"}
	_, err := v.Validate(context.Background(), "sk-test")
	require.Error(t, err)
	assert.ErrorIs(t, err, auth.ErrKeyUnavailable)
	assert.Equal(t, "/v1/v1/models", gotPath)
}
