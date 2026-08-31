package proxy

import (
	"context"
	"testing"
	"time"

	"workweave/router/internal/router"
	"workweave/router/internal/router/sessionpin"

	"github.com/google/uuid"
)

func TestLoadPin_CanonicalizesLegacyModel(t *testing.T) {
	const canonical = "zai-org/glm-5.3"
	const legacy = "z-ai/glm-5.2"

	var testKey [sessionpin.SessionKeyLen]byte
	for i := range testKey {
		testKey[i] = byte(i + 1)
	}

	pin := sessionpin.Pin{
		SessionKey:                testKey,
		Role:                      sessionpin.DefaultRole,
		InstallationID:            uuid.New(),
		Provider:                  "aiand",
		Model:                     legacy, // legacy alias
		PairedModel:               "",
		Reason:                    "test",
		PinnedUntil:               time.Now().Add(time.Hour),
		FirstPinnedAt:             time.Now(),
		LastSeenAt:                time.Now(),
		LastTurnEndedAt:           time.Now(),
		ConsecutiveUpstreamErrors: 0,
		LastServedModel:           legacy, // legacy alias
		ConsecutiveOverloadErrors: 0,
		DisabledProviders:         nil,
	}

	st := &sessionPinStoreFake{onGet: func(ctx context.Context, key [sessionpin.SessionKeyLen]byte, role string) (sessionpin.Pin, bool, error) {
		return pin, true, nil
	}}

	s := &Service{pinStore: st}
	loaded, ok := s.loadPin(context.Background(), testKey, sessionpin.DefaultRole)
	if !ok {
		t.Fatal("expected active pin load to succeed")
	}
	if got := loaded.Model; got != canonical {
		t.Fatalf("Model canonicalized to %q, want %q", got, canonical)
	}
	if got := loaded.LastServedModel; got != canonical {
		t.Fatalf("LastServedModel canonicalized to %q, want %q", got, canonical)
	}
}

func TestLoadPin_PassesThroughCanonical(t *testing.T) {
	const canonical = "zai-org/glm-5.3"

	var testKey [sessionpin.SessionKeyLen]byte
	for i := range testKey {
		testKey[i] = byte(i + 2)
	}

	pin := sessionpin.Pin{
		SessionKey:                testKey,
		Role:                      sessionpin.DefaultRole,
		InstallationID:            uuid.New(),
		Provider:                  "aiand",
		Model:                     canonical,
		PairedModel:               "",
		Reason:                    "test",
		PinnedUntil:               time.Now().Add(time.Hour),
		FirstPinnedAt:             time.Now(),
		LastSeenAt:                time.Now(),
		LastTurnEndedAt:           time.Now(),
		ConsecutiveUpstreamErrors: 0,
		LastServedModel:           canonical,
		ConsecutiveOverloadErrors: 0,
		DisabledProviders:         nil,
	}

	st := &sessionPinStoreFake{onGet: func(ctx context.Context, key [sessionpin.SessionKeyLen]byte, role string) (sessionpin.Pin, bool, error) {
		return pin, true, nil
	}}

	s := &Service{pinStore: st}
	loaded, ok := s.loadPin(context.Background(), testKey, sessionpin.DefaultRole)
	if !ok {
		t.Fatal("expected active pin load to succeed")
	}
	if got := loaded.Model; got != canonical {
		t.Fatalf("Model changed to %q, want %q unchanged", got, canonical)
	}
}

func TestLoadPin_LegacyPairedModelCanonicalized(t *testing.T) {
	const canonical = "zai-org/glm-5.3"
	const legacy = "z-ai/glm-5.2"

	var testKey [sessionpin.SessionKeyLen]byte
	for i := range testKey {
		testKey[i] = byte(i + 3)
	}

	pin := sessionpin.Pin{
		SessionKey:                testKey,
		Role:                      sessionpin.DefaultRole,
		InstallationID:            uuid.New(),
		Provider:                  "aiand",
		Model:                     canonical,
		PairedModel:               legacy, // legacy alias
		Reason:                    "test",
		PinnedUntil:               time.Now().Add(time.Hour),
		FirstPinnedAt:             time.Now(),
		LastSeenAt:                time.Now(),
		LastTurnEndedAt:           time.Now(),
		ConsecutiveUpstreamErrors: 0,
		LastServedModel:           "",
		ConsecutiveOverloadErrors: 0,
		DisabledProviders:         nil,
	}

	st := &sessionPinStoreFake{onGet: func(ctx context.Context, key [sessionpin.SessionKeyLen]byte, role string) (sessionpin.Pin, bool, error) {
		return pin, true, nil
	}}

	s := &Service{pinStore: st}
	loaded, ok := s.loadPin(context.Background(), testKey, sessionpin.DefaultRole)
	if !ok {
		t.Fatal("expected active pin load to succeed")
	}
	if got := loaded.Model; got != canonical {
		t.Fatalf("Model canonicalized to %q, want %q", got, canonical)
	}
	if got := loaded.PairedModel; got != canonical {
		t.Fatalf("PairedModel canonicalized to %q, want %q", got, canonical)
	}
}

func TestLoadHMMHistory_CanonicalizesLegacyModel(t *testing.T) {
	const canonical = "zai-org/glm-5.3"
	const legacy = "z-ai/glm-5.2"

	var testKey [sessionpin.SessionKeyLen]byte
	for i := range testKey {
		testKey[i] = byte(i + 4)
	}

	hmmRole := hmmHistoryRole(sessionpin.DefaultRole)
	pin := sessionpin.Pin{
		SessionKey:                testKey,
		Role:                      hmmRole,
		InstallationID:            uuid.New(),
		Provider:                  "aiand",
		Model:                     legacy, // legacy alias
		PairedModel:               "",
		Reason:                    "test",
		PinnedUntil:               time.Now().Add(time.Hour),
		FirstPinnedAt:             time.Now(),
		LastSeenAt:                time.Now(),
		LastTurnEndedAt:           time.Now(),
		ConsecutiveUpstreamErrors: 0,
		LastServedModel:           legacy, // legacy alias
		ConsecutiveOverloadErrors: 0,
		DisabledProviders:         nil,
	}

	st := &sessionPinStoreFake{onGet: func(ctx context.Context, key [sessionpin.SessionKeyLen]byte, role string) (sessionpin.Pin, bool, error) {
		return pin, true, nil
	}}

	s := &Service{pinStore: st}
	hmm := s.loadHMMHistory(context.Background(), testKey, sessionpin.DefaultRole)
	if got := hmm.Model; got != canonical {
		t.Fatalf("Model canonicalized to %q, want %q", got, canonical)
	}
	if got := hmm.LastServedModel; got != canonical {
		t.Fatalf("LastServedModel canonicalized to %q, want %q", got, canonical)
	}
}

// sessionPinStoreFake implements the sessionpin.Store Get method only, for tests.
type sessionPinStoreFake struct {
	onGet func(ctx context.Context, key [sessionpin.SessionKeyLen]byte, role string) (sessionpin.Pin, bool, error)
}

func (st *sessionPinStoreFake) Get(ctx context.Context, key [sessionpin.SessionKeyLen]byte, role string) (sessionpin.Pin, bool, error) {
	return st.onGet(ctx, key, role)
}

func (st *sessionPinStoreFake) Consume(ctx context.Context, key [sessionpin.SessionKeyLen]byte, role string, expected router.Strategy) (sessionpin.Pin, bool, error) {
	return st.onGet(ctx, key, role)
}

func (st *sessionPinStoreFake) Upsert(ctx context.Context, p sessionpin.Pin) error {
	return nil
}

func (st *sessionPinStoreFake) UpdateUsage(ctx context.Context, key [sessionpin.SessionKeyLen]byte, role string, usage sessionpin.Usage) error {
	return nil
}

func (st *sessionPinStoreFake) IncrementUpstreamErrors(ctx context.Context, key [sessionpin.SessionKeyLen]byte, role string, expected router.Strategy) (int, error) {
	return 0, nil
}

func (st *sessionPinStoreFake) ResetUpstreamErrors(ctx context.Context, key [sessionpin.SessionKeyLen]byte, role string, expected router.Strategy) error {
	return nil
}

func (st *sessionPinStoreFake) IncrementOverloadErrors(ctx context.Context, key [sessionpin.SessionKeyLen]byte, role string, expected router.Strategy) (int, error) {
	return 0, nil
}

func (st *sessionPinStoreFake) ResetOverloadErrors(ctx context.Context, key [sessionpin.SessionKeyLen]byte, role string, expected router.Strategy) error {
	return nil
}

func (st *sessionPinStoreFake) DisableProvider(ctx context.Context, key [sessionpin.SessionKeyLen]byte, role, provider string, expected router.Strategy) error {
	return nil
}

func (st *sessionPinStoreFake) SweepExpired(ctx context.Context) error {
	return nil
}
