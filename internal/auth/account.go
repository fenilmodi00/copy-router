package auth

import (
	"context"
	"net"
	"time"
)

// Account is the self-service login identity: an aiand user bound to the router
// installation that owns all tenant data. The account's ID doubles as the
// installation's external_id (1:1, re-hydrated via ListForExternalID). aiand_user_id
// is an opaque external string, never an FK. A soft-deleted account means the
// user revoked their key (their data wipe is intentional).
type Account struct {
	ID                  string
	AiandUserID         string
	AiandOrganizationID string
	DisplayName         *string
	CreatedAt           time.Time
	LastLoginAt         time.Time
	DeletedAt           *time.Time
}

// AccountUpsertParams carries the caller-generated account id (which doubles as
// the installation external_id) plus the aiand identity.
type AccountUpsertParams struct {
	ID                  string
	AiandUserID         string
	AiandOrganizationID string
	DisplayName         *string
}

// AccountRepository persists self-service login accounts. Implemented by
// internal/postgres.
type AccountRepository interface {
	UpsertByAiandUser(ctx context.Context, p AccountUpsertParams) (*Account, error)
	GetByAiandUserID(ctx context.Context, aiandUserID string) (*Account, error)
	GetByID(ctx context.Context, id string) (*Account, error)
	SoftDelete(ctx context.Context, id string) error
}

// LoginSession is a revocable, opaque, SHA-256-hashed dashboard session.
type LoginSession struct {
	ID          string
	AccountID   string
	TokenHash   string
	TokenPrefix string
	TokenSuffix string
	IssuedAt    time.Time
	ExpiresAt   time.Time
	RevokedAt   *time.Time
	LastSeenAt  time.Time
	IPAtIssue   *net.IP
}

// LoginSessionRepository persists dashboard sessions for self-service accounts.
// Implemented by internal/postgres.
type LoginSessionRepository interface {
	Insert(ctx context.Context, s LoginSession) (*LoginSession, error)
	GetActiveByTokenHash(ctx context.Context, tokenHash string) (*LoginSession, error)
	TouchLastSeen(ctx context.Context, accountID, sessionID string) error
	RevokeByID(ctx context.Context, accountID, sessionID string) error
	RevokeAllForAccount(ctx context.Context, accountID string) error
	ListForAccount(ctx context.Context, accountID string) ([]LoginSession, error)
}
