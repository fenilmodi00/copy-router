package proxy

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func ctxWithCreds(creds *Credentials) context.Context {
	return context.WithValue(context.Background(), CredentialsContextKey{}, creds)
}

func TestCredentialKeyParts_DeploymentKeyTurnIsEmpty(t *testing.T) {
	s := &Service{}
	prefix, suffix, src := s.credentialKeyParts(context.Background())
	assert.Empty(t, prefix, "a deployment-key turn has no per-request credential to record")
	assert.Empty(t, suffix)
	assert.Empty(t, src)

	prefixNil, suffixNil, srcNil := s.credentialKeyParts(ctxWithCreds(nil))
	assert.Empty(t, prefixNil)
	assert.Empty(t, suffixNil)
	assert.Empty(t, srcNil)
}



func TestCredentialKeyParts_ShortKey(t *testing.T) {
	s := &Service{}
	prefix, suffix, src := s.credentialKeyParts(ctxWithCreds(&Credentials{APIKey: []byte("short-key"), Source: credSourceClient}))
	assert.Equal(t, "short-key", prefix)
	assert.Empty(t, suffix)
	assert.Equal(t, credSourceClient, src)
}

