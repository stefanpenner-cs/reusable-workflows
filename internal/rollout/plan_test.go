package rollout

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPlanSummarizesCohort(t *testing.T) {
	m := mustParse(t, `{"channels":{"stable":{
		"ref":"v1","ramp":{"ref":"v2","percent":50},
		"overrides":{"acme/pinned":"v0.9"}}}}`)

	repos := []string{"acme/pinned"}
	for i := 0; i < 200; i++ {
		repos = append(repos, fmt.Sprintf("org/app-%d", i))
	}

	plan, err := Plan(m, "stable", repos)
	require.NoError(t, err)

	assert.Equal(t, len(repos), plan.Total)
	assert.Equal(t, 1, plan.ByRule["override"], "the one pinned repo")
	// ~50% of the 200 org repos ramp; allow slack for hash variance.
	assert.Greater(t, plan.ByRule["ramp"], 60)
	assert.Less(t, plan.ByRule["ramp"], 140)
	// Every repo is accounted for exactly once.
	sum := 0
	for _, n := range plan.ByRule {
		sum += n
	}
	assert.Equal(t, plan.Total, sum)
	// Ref tallies also sum to total.
	refSum := 0
	for _, n := range plan.ByRef {
		refSum += n
	}
	assert.Equal(t, plan.Total, refSum)
	assert.Equal(t, 1, plan.ByRef["v0.9"])
}

func TestPlanUnknownChannel(t *testing.T) {
	m := mustParse(t, `{"channels":{"stable":{"ref":"v1"}}}`)
	_, err := Plan(m, "nope", []string{"a/b"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), `unknown channel "nope"`)
}
