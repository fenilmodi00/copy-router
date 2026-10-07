// Package policy's product-eligibility coverage is inert on the AIand-only
// catalog: every surviving row is SourceOpenSource, so
// eligibility.MaxOpenSourceOnly refuses nothing and the product-ineligible
// exclusion (and its precedence over the desugared wire exclusion) is
// unreachable through real catalog data. The fixtures that named
// closed/unknown-source rows are gone rather than left asserting a no-op.
//
// If a non-open-source row returns to internal/router/catalog/catalog.go,
// restore coverage for: ExclusionProductIneligible ahead of provider
// resolution, its hardness when it empties the pool, and its precedence over
// the request exclusion it desugars into.
package policy_test
