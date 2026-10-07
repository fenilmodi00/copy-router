package catalog

// RouterTokenScalars estimates how many tokens a baseline model would have
// needed per token the served model actually used, one ratio per token kind.
// It exists because pricing one model's observed token counts at another
// model's rates assumes both are equally verbose, and they are not.
//
// Each value is tokens(baseline) / tokens(served), so it multiplies the served
// model's counts to yield the baseline's estimated counts. Above 1.0 means the
// baseline is the more verbose of the pair, which makes the counterfactual
// dearer and the reported savings larger, since consumers report comparison
// cost minus actual cost. Inverting this direction breaks no test: reciprocal
// cells stay self-consistent.
type RouterTokenScalars struct {
	Input      float64
	Output     float64
	CacheWrite float64
	CacheRead  float64
}

// routerTokenScalarTable maps served model -> baseline model -> ratios. A pair
// absent from the table means unit scalars, so models outside the measured
// window keep the equal-token-count behaviour rather than dropping out of the
// comparison.
//
// Empty after the AIand-only cut: every measured pair involved a claude-*/gpt-*
// baseline or served row, and all of those catalog rows were retired. An empty
// table is exactly the unit-scalar fallback the consumer already applies, so
// the dashboard's numbers are unchanged until roster pairs are measured.
//
// Nothing in the router reads this; the only consumer is WorkWeave's router
// dashboard, which scripts/sync_router_pricing.py copies this file into
// verbatim, rewriting just the package clause. Keep the file free of imports
// and functions so that stays true. Keys are raw IDs to match Models itself,
// which the copy cannot borrow a key type from.
var routerTokenScalarTable = map[string]map[string]RouterTokenScalars{}
