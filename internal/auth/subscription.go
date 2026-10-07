package auth

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// SubscriptionProvider identifies a provider-specific account pool.
type SubscriptionProvider string

const (
	// SubscriptionProviderClaude is a Claude subscription account.
	SubscriptionProviderClaude SubscriptionProvider = "claude"
	// SubscriptionProviderCodex is a Codex subscription account.
	SubscriptionProviderCodex SubscriptionProvider = "codex"
)

// SubscriptionAccountState is the credential-free routing health of a linked account.
type SubscriptionAccountState string

const (
	SubscriptionAccountStateActive            SubscriptionAccountState = "active"
	SubscriptionAccountStateExhausted         SubscriptionAccountState = "exhausted"
	SubscriptionAccountStateCooldown          SubscriptionAccountState = "cooldown"
	SubscriptionAccountStateReconnectRequired SubscriptionAccountState = "reconnect_required"
	SubscriptionAccountStateDisabled          SubscriptionAccountState = "disabled"
	SubscriptionAccountStateUnknown           SubscriptionAccountState = "unknown"
)

// Routable reports whether an account may be attempted.
func (s SubscriptionAccountState) Routable() bool {
	return s == SubscriptionAccountStateActive || s == SubscriptionAccountStateUnknown
}

// SubscriptionOwner addresses the linked accounts one authenticated caller may
// serve and manage. SubscriberID is the Router credential subject and survives
// API-key rotation, so it is the runtime pool identity. APIKeyID is enrollment
// attribution, and the only ownership legacy rows have until they are migrated
// or reconnected — a subscriber therefore still reaches the unattributed rows
// of whichever key it is presenting.
type SubscriptionOwner struct {
	SubscriberID string
	APIKeyID     string
}

// Valid reports whether this owner can address any account.
func (o SubscriptionOwner) Valid() bool {
	return o.SubscriberID != "" || o.APIKeyID != ""
}

// PoolKey is the runtime pool identity. Two keys of one subscriber share a
// pool; a key with no subscriber keeps its own legacy pool.
func (o SubscriptionOwner) PoolKey() string {
	switch {
	case o.SubscriberID != "":
		return "subscriber:" + o.SubscriberID
	case o.APIKeyID != "":
		return "api_key:" + o.APIKeyID
	default:
		return ""
	}
}

// LegacyPoolKey is the pool identity of the rows this owner reaches only
// through its api key. It stays separate from PoolKey so a second key of one
// subscriber never serves another key's unattributed accounts, whose
// subscriber is by definition unknown.
func (o SubscriptionOwner) LegacyPoolKey() string {
	if o.APIKeyID == "" {
		return ""
	}
	return "api_key:" + o.APIKeyID
}

// SyncKey identifies the full set of pools this owner reaches, for callers
// caching a pool refresh.
func (o SubscriptionOwner) SyncKey() string {
	return o.PoolKey() + "|" + o.LegacyPoolKey()
}

// LogKey identifies the pool in logs without emitting the api key id: legacy
// pools are distinguished by the account id logged alongside it.
func (o SubscriptionOwner) LogKey() string {
	switch {
	case o.SubscriberID != "":
		return "subscriber:" + o.SubscriberID
	case o.APIKeyID != "":
		return "api_key"
	default:
		return ""
	}
}

// SubscriptionOwnerForKey derives linked-account ownership from an
// authenticated key: its credential subject where one exists, plus the key
// itself for rows enrolled before ownership moved to the subscriber.
func SubscriptionOwnerForKey(key *APIKey) SubscriptionOwner {
	if key == nil {
		return SubscriptionOwner{}
	}
	return SubscriptionOwner{SubscriberID: key.CredentialSubjectID, APIKeyID: key.ID}
}

// SubscriptionAccount is the server-side representation of an enrolled
// account. RefreshTokenCiphertext is encrypted storage and must not cross the
// auth/service boundary into an API response.
type SubscriptionAccount struct {
	ID                 string
	SubscriberID       string
	EnrolledByAPIKeyID string
	Provider           SubscriptionProvider
	ExternalAccountID  string
	// DisplayName is provider-supplied metadata for humans; it is not identity.
	DisplayName            string
	RefreshTokenCiphertext []byte
	Enabled                bool
	State                  SubscriptionAccountState
	CooldownUntil          *time.Time
	CreatedAt              time.Time
}

// CreateSubscriptionAccountParams describes an encrypted account enrollment.
type CreateSubscriptionAccountParams struct {
	Owner             SubscriptionOwner
	Provider          SubscriptionProvider
	ExternalAccountID string
	DisplayName       string
	RefreshToken      []byte
	// InstallationExternalID identifies the authenticated installation for onboarding.
	InstallationExternalID string
}

// SubscriptionUpsertKind reports whether an upsert inserted, adopted a legacy row, or refreshed an existing identity.
type SubscriptionUpsertKind string

const (
	SubscriptionUpsertUpdated  SubscriptionUpsertKind = "updated"
	SubscriptionUpsertInserted SubscriptionUpsertKind = "inserted"
	SubscriptionUpsertAdopted  SubscriptionUpsertKind = "adopted"
)

// FirstConnected reports a genuine first registration, including legacy-row adoption.
func (k SubscriptionUpsertKind) FirstConnected() bool {
	return k == SubscriptionUpsertInserted || k == SubscriptionUpsertAdopted
}

// SubscriptionCredentialRecord is the encrypted credential state for one
// enrolled account. It never crosses the auth service boundary in this form.
type SubscriptionCredentialRecord struct {
	ExternalAccountID      string
	Provider               SubscriptionProvider
	RefreshTokenCiphertext []byte
	AccessTokenCiphertext  []byte
	AccessTokenExpiresAt   *time.Time
	TokenRefreshVersion    int64
	TokenRefreshLeaseID    string
	Enabled                bool
	State                  SubscriptionAccountState
	CooldownUntil          *time.Time
}

// SubscriptionCredentials is the decrypted credential state used by the
// subscription runtime. The access token is retained only in process memory
// after this method returns.
type SubscriptionCredentials struct {
	RefreshToken         []byte
	AccessToken          []byte
	AccessTokenExpiresAt *time.Time
	TokenRefreshVersion  int64
	TokenRefreshLeaseID  string
	Enabled              bool
	State                SubscriptionAccountState
	CooldownUntil        *time.Time
}

// RefreshLeaseAcquisition is the outcome of one refresh-lease attempt.
// TookOver means an expired lease from a holder that never released was
// replaced. That holder may already have spent the refresh token, so the new
// holder must not treat a terminal provider error as proof the account is dead.
type RefreshLeaseAcquisition struct {
	Acquired bool
	TookOver bool
}

// SubscriptionRefreshRepository coordinates refresh leases and encrypted
// credential persistence across router replicas.
type SubscriptionRefreshRepository interface {
	TryAcquireSubscriptionRefreshLease(context.Context, string, SubscriptionOwner, string, time.Duration) (RefreshLeaseAcquisition, error)
	ExtendSubscriptionRefreshLease(context.Context, string, SubscriptionOwner, string, time.Duration) (int64, error)
	ReleaseSubscriptionRefreshLease(context.Context, string, SubscriptionOwner, string) error
	DisableSubscriptionAccountIfRefreshHolder(context.Context, string, SubscriptionOwner, string, int64) error
	CooldownSubscriptionAccountIfRefreshHolder(context.Context, string, SubscriptionOwner, string, int64, time.Time) error
	GetSubscriptionCredentialRecord(context.Context, string, SubscriptionOwner) (*SubscriptionCredentialRecord, error)
	PersistSubscriptionTokens(context.Context, string, SubscriptionOwner, string, int64, []byte, []byte, time.Time) error
}

// SubscriptionAccountRepository persists encrypted subscription account state
// and coordinates cross-replica refresh leases.
type SubscriptionAccountRepository interface {
	UpsertSubscriptionAccount(context.Context, CreateSubscriptionAccountParams) (*SubscriptionAccount, SubscriptionUpsertKind, error)
	ListSubscriptionAccounts(context.Context, SubscriptionOwner) ([]*SubscriptionAccount, error)
	UpdateSubscriptionAccountState(context.Context, string, SubscriptionOwner, bool, *time.Time) error
	UpdateSubscriptionAccountCooldown(context.Context, string, SubscriptionOwner, time.Time) error
	UpdateSubscriptionRefreshToken(context.Context, string, SubscriptionOwner, []byte) error
	DeleteSubscriptionAccount(context.Context, string, SubscriptionOwner) error
	SubscriptionRefreshRepository
}

// ErrSubscriptionAccountNotFound indicates a state mutation did not match the
// authenticated owner.
var ErrSubscriptionAccountNotFound = errors.New("subscription account not found")

// ErrSubscriptionRefreshConflict means the lease or credential version no longer
// permits this refresher to publish tokens or record a failure.
var ErrSubscriptionRefreshConflict = errors.New("subscription refresh lost race")

const subscriptionAccessPurposeSuffix = ":access"

func normalizeSubscriptionAccountDisplayName(value string) string {
	value = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, value)
	value = strings.Join(strings.Fields(value), " ")
	if utf8.RuneCountInString(value) > 512 {
		value = string([]rune(value)[:512])
	}
	return value
}

func subscriptionAccessPurpose(provider SubscriptionProvider) string {
	return string(provider) + subscriptionAccessPurposeSuffix
}
