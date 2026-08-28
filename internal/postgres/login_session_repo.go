package postgres

import (
	"context"
	"net"
	"net/netip"

	"workweave/router/internal/auth"
	"workweave/router/internal/sqlc"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// loginSessionRepo implements auth.LoginSessionRepository over SQLC. Session
// ids are DB-generated UUIDs surfaced as uuid.UUID; account_id is the acct_
// VARCHAR string.
type loginSessionRepo struct {
	tx sqlc.DBTX
}

// inetToNetIP converts the column's *netip.Addr to the domain *net.IP. Returns
// nil for a NULL row so the domain preserves "no IP captured".
func inetToNetIP(addr *netip.Addr) *net.IP {
	if addr == nil {
		return nil
	}
	b := addr.AsSlice()
	ip := net.IP(b)
	return &ip
}

// netIPToInet converts the domain *net.IP to the *netip.Addr the INSERT query
// expects. A nil domain IP becomes a nil arg so the column lands as NULL.
func netIPToInet(ip *net.IP) *netip.Addr {
	if ip == nil {
		return nil
	}
	// netip.AddrFromSlice returns a 16-byte (IPv6) or 4-byte (IPv4) addr after
	// trimming net.IP's 16-byte canonical v6-in-v4 encoding down to the
	// natural width; both round-trip through the column as INET.
	addr, ok := netip.AddrFromSlice(*ip)
	if !ok {
		return nil
	}
	return &addr
}

// loginSessionRow converts a sqlc.RouterAccountSession row to the domain
// LoginSession. issued_at / expires_at / last_seen_at are NOT NULL timestamptz;
// revoked_at is nullable (nil while the session is still active).
func loginSessionRow(row sqlc.RouterAccountSession) *auth.LoginSession {
	s := &auth.LoginSession{
		ID:          row.ID.String(),
		AccountID:   row.AccountID,
		TokenHash:   row.TokenHash,
		TokenPrefix: row.TokenPrefix,
		TokenSuffix: row.TokenSuffix,
		IssuedAt:    timestamptzOrZero(row.IssuedAt),
		ExpiresAt:   timestamptzOrZero(row.ExpiresAt),
		RevokedAt:   timestamptzPtr(row.RevokedAt),
		LastSeenAt:  timestamptzOrZero(row.LastSeenAt),
		IPAtIssue:   inetToNetIP(row.IpAtIssue),
	}
	if row.InstallationID != nil {
		s.InstallationID = *row.InstallationID
	}
	return s
}

func (r *loginSessionRepo) Insert(ctx context.Context, s auth.LoginSession) (*auth.LoginSession, error) {
	q := sqlc.New(r.tx)
	params := sqlc.InsertAccountSessionParams{
		AccountID:   s.AccountID,
		TokenHash:   s.TokenHash,
		TokenPrefix: s.TokenPrefix,
		TokenSuffix: s.TokenSuffix,
		ExpiresAt:   pgtype.Timestamptz{Time: s.ExpiresAt, Valid: true},
		IpAtIssue:   netIPToInet(s.IPAtIssue),
	}
	if s.InstallationID != "" {
		params.InstallationID = &s.InstallationID
	}
	row, err := q.InsertAccountSession(ctx, params)
	if err != nil {
		return nil, err
	}
	return loginSessionRow(row), nil
}

func (r *loginSessionRepo) GetActiveByTokenHash(ctx context.Context, tokenHash string) (*auth.LoginSession, error) {
	q := sqlc.New(r.tx)
	row, err := q.GetActiveAccountSessionByHash(ctx, tokenHash)
	if err != nil {
		return nil, err
	}
	return loginSessionRow(row), nil
}

func (r *loginSessionRepo) TouchLastSeen(ctx context.Context, accountID, sessionID string) error {
	id, err := uuid.Parse(sessionID)
	if err != nil {
		return err
	}
	q := sqlc.New(r.tx)
	_, err = q.TouchAccountSessionLastSeen(ctx, sqlc.TouchAccountSessionLastSeenParams{
		AccountID: accountID,
		ID:        id,
	})
	return err
}

func (r *loginSessionRepo) RevokeByID(ctx context.Context, accountID, sessionID string) error {
	id, err := uuid.Parse(sessionID)
	if err != nil {
		return err
	}
	q := sqlc.New(r.tx)
	_, err = q.RevokeAccountSessionByID(ctx, sqlc.RevokeAccountSessionByIDParams{
		AccountID: accountID,
		ID:        id,
	})
	return err
}

func (r *loginSessionRepo) RevokeAllForAccount(ctx context.Context, accountID string) error {
	q := sqlc.New(r.tx)
	_, err := q.RevokeAllAccountSessionsForAccount(ctx, accountID)
	return err
}

func (r *loginSessionRepo) ListForAccount(ctx context.Context, accountID string) ([]auth.LoginSession, error) {
	q := sqlc.New(r.tx)
	rows, err := q.ListAccountSessionsForAccount(ctx, accountID)
	if err != nil {
		return nil, err
	}
	out := make([]auth.LoginSession, 0, len(rows))
	for _, row := range rows {
		out = append(out, *loginSessionRow(row))
	}
	return out, nil
}
