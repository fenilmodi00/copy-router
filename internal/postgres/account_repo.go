package postgres

import (
	"context"

	"workweave/router/internal/auth"
	"workweave/router/internal/sqlc"
)

// accountRepo implements auth.AccountRepository over SQLC. Account ids are
// acct_ VARCHAR strings that double as installation external_ids, so no UUID
// conversion is needed — every column maps to a plain Go string.
type accountRepo struct {
	tx sqlc.DBTX
}

// accountRow converts a sqlc.RouterAccount row to the domain Account. The
// created_at / last_login_at columns are NOT NULL timestamptz; deleted_at is
// nullable and surfaces as nil when the account is still active.
func accountRow(row sqlc.RouterAccount) *auth.Account {
	return &auth.Account{
		ID:                  row.ID,
		AiandUserID:         row.AiandUserID,
		AiandOrganizationID: row.AiandOrganizationID,
		// display_name is VARCHAR(255) NULL; both the generated row and the
		// domain carry *string, so the value passes through unchanged.
		DisplayName: row.DisplayName,
		CreatedAt:   timestamptzOrZero(row.CreatedAt),
		LastLoginAt: timestamptzOrZero(row.LastLoginAt),
		DeletedAt:   timestamptzPtr(row.DeletedAt),
	}
}

// UpsertByAiandUser inserts a fresh account on first login or refreshes
// aiand_organization_id / display_name + bumps last_login_at on a return. The
// ON CONFLICT clause is keyed on the partial unique index
// accounts_aiand_user_id_active_idx, so two concurrent first-logins for the
// same aiand user agree on one account id.
func (r *accountRepo) UpsertByAiandUser(ctx context.Context, p auth.AccountUpsertParams) (*auth.Account, error) {
	q := sqlc.New(r.tx)
	row, err := q.UpsertAccount(ctx, sqlc.UpsertAccountParams{
		ID:                  p.ID,
		AiandUserID:         p.AiandUserID,
		AiandOrganizationID: p.AiandOrganizationID,
		// UpsertAccountParams.DisplayName is *string (NULL when the caller
		// omits one) — same shape as the domain params, so no helper needed.
		DisplayName: p.DisplayName,
	})
	if err != nil {
		return nil, err
	}
	return accountRow(row), nil
}

func (r *accountRepo) GetByAiandUserID(ctx context.Context, aiandUserID string) (*auth.Account, error) {
	q := sqlc.New(r.tx)
	row, err := q.GetAccountByAiandUserID(ctx, aiandUserID)
	if err != nil {
		return nil, err
	}
	return accountRow(row), nil
}

func (r *accountRepo) GetByID(ctx context.Context, id string) (*auth.Account, error) {
	q := sqlc.New(r.tx)
	row, err := q.GetAccountByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return accountRow(row), nil
}

func (r *accountRepo) SoftDelete(ctx context.Context, id string) error {
	q := sqlc.New(r.tx)
	return q.SoftDeleteAccount(ctx, id)
}
