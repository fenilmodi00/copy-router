package hmm

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

type fakeDecider struct {
	query Query
	res   Result
	err   error
	calls int
}

func (f *fakeDecider) Decide(_ context.Context, q Query) (Result, error) {
	f.calls++
	f.query = q
	return f.res, f.err
}

func candidateRosterIDs(candidates []Candidate) []string {
	ids := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		ids = append(ids, candidate.RosterID)
	}
	return ids
}

func TestIsToolExecutionResultRecognizesBothTaxonomies(t *testing.T) {
	// "explore" is the retired five-class label (roster_v2, still the pinned
	// prod package); "low" is its four-class successor (roster_v4) that the
	// retired explore cluster was merged into. Both must route as tool
	// execution during the migration window where either package can be
	// deployed.
	assert.True(t, isToolExecutionResult(Result{PolicyGroup: "explore"}))
	assert.True(t, isToolExecutionResult(Result{PolicyGroup: "low"}))
	assert.True(t, isToolExecutionResult(Result{PolicyGroup: "EXPLORE"}))
	assert.True(t, isToolExecutionResult(Result{PolicyGroup: " Low "}))
	assert.True(t, isToolExecutionResult(Result{PolicyLabel: "spawn_explore"}))
	assert.True(t, isToolExecutionResult(Result{PolicyLabel: "prefix_tool_call_suffix"}))
	assert.False(t, isToolExecutionResult(Result{PolicyGroup: "fast"}))
	assert.False(t, isToolExecutionResult(Result{PolicyGroup: "balanced"}))
	assert.False(t, isToolExecutionResult(Result{PolicyGroup: "medium"}))
	assert.False(t, isToolExecutionResult(Result{}))
}

func TestReasonForTagsToolExecutionPrefixForBothTaxonomies(t *testing.T) {
	assert.Equal(t, "hmm_policy:tool_execution(label=explore)", reasonFor(Result{PolicyGroup: "explore", PolicyLabel: "explore"}))
	assert.Equal(t, "hmm_policy:tool_execution(label=low)", reasonFor(Result{PolicyGroup: "low", PolicyLabel: "low"}))
	assert.Equal(t, "hmm_policy(label=balanced)", reasonFor(Result{PolicyGroup: "balanced", PolicyLabel: "balanced"}))
}
