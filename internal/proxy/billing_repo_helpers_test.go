package proxy_test

import (
	"context"
	"sync"

	"weave-os/router/internal/billing"
)

// capturingBillingRepo records debits and answers balance/spend reads for
// proxy tests that need a wired billing service.
type capturingBillingRepo struct {
	userSpent int64
	userLimit *int64

	mu     sync.Mutex
	debits []billing.DebitParams
}

func (r *capturingBillingRepo) GetBalance(context.Context, string) (int64, error) { return 0, nil }
func (r *capturingBillingRepo) HasActiveOverride(context.Context, string) (bool, error) {
	return false, nil
}
func (r *capturingBillingRepo) DebitInference(_ context.Context, p billing.DebitParams) (int64, error) {
	r.mu.Lock()
	r.debits = append(r.debits, p)
	r.mu.Unlock()
	return 0, nil
}
func (r *capturingBillingRepo) GetAPIKeySpend(context.Context, string) (int64, *int64, bool, error) {
	return 0, nil, false, nil
}
func (r *capturingBillingRepo) GetUserMonthlySpendAndLimit(context.Context, string, string) (int64, *int64, error) {
	return r.userSpent, r.userLimit, nil
}
func (r *capturingBillingRepo) GetOrgMonthlySpendAndLimit(context.Context, string) (int64, *int64, error) {
	return 0, nil, nil
}
func (r *capturingBillingRepo) GetAutopayConfig(context.Context, billing.Owner) (bool, int64, error) {
	return false, 0, nil
}
func (r *capturingBillingRepo) BillingTablesExist(context.Context) (bool, error) { return true, nil }

func (r *capturingBillingRepo) recordedDebits() []billing.DebitParams {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]billing.DebitParams(nil), r.debits...)
}
