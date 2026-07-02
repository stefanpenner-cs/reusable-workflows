package rollout

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBucketProperties(t *testing.T) {
	// Deterministic.
	assert.Equal(t, Bucket("acme/app"), Bucket("acme/app"))

	// Case-insensitive: GitHub repo names are case-insensitive.
	assert.Equal(t, Bucket("Acme/App"), Bucket("acme/app"))

	// In range, and roughly uniform over many repos (expected 10 per bucket for 1000 repos).
	counts := map[int]int{}
	for i := 0; i < 1000; i++ {
		b := Bucket(fmt.Sprintf("org%d/repo%d", i%37, i))
		require.GreaterOrEqual(t, b, 0)
		require.Less(t, b, 100)
		counts[b]++
	}
	for b, n := range counts {
		assert.LessOrEqual(t, n, 40, "bucket %d is pathologically hot", b)
	}
	assert.Greater(t, len(counts), 80, "hash should spread across most buckets")
}

func mustParse(t *testing.T, s string) Manifest {
	t.Helper()
	m, err := ParseManifest([]byte(s))
	require.NoError(t, err)
	return m
}

func TestResolveUnknownChannel(t *testing.T) {
	m := mustParse(t, `{"channels":{"stable":{"ref":"v1"}}}`)
	_, err := Resolve(m, "prod", "acme/app")
	require.Error(t, err)
	assert.Contains(t, err.Error(), `unknown channel "prod"`)
	assert.Contains(t, err.Error(), "stable") // names the channels that do exist
}

func TestResolveChannelRef(t *testing.T) {
	m := mustParse(t, `{"channels":{"stable":{"ref":"v1.2.0"}}}`)
	d, err := Resolve(m, "stable", "acme/app")
	require.NoError(t, err)
	assert.Equal(t, "v1.2.0", d.Ref)
	assert.Equal(t, "channel", d.Rule)
}

func TestResolveOverrideWinsOverRamp(t *testing.T) {
	m := mustParse(t, `{"channels":{"stable":{
		"ref":"v1",
		"ramp":{"ref":"v2","percent":100},
		"overrides":{"acme/pinned":"v0.9"}}}}`)

	d, err := Resolve(m, "stable", "acme/pinned")
	require.NoError(t, err)
	assert.Equal(t, "v0.9", d.Ref)
	assert.Equal(t, "override", d.Rule)

	// Override lookup is case-insensitive like repo names.
	d, err = Resolve(m, "stable", "Acme/Pinned")
	require.NoError(t, err)
	assert.Equal(t, "v0.9", d.Ref)
}

func TestResolveRampBoundaries(t *testing.T) {
	manifest := `{"channels":{"stable":{"ref":"old","ramp":{"ref":"new","percent":%d}}}}`

	// percent=0: nobody ramps; percent=100: everybody ramps.
	m0 := mustParse(t, fmt.Sprintf(manifest, 0))
	m100 := mustParse(t, fmt.Sprintf(manifest, 100))
	for _, repo := range []string{"a/a", "b/b", "c/c", "acme/app"} {
		d, err := Resolve(m0, "stable", repo)
		require.NoError(t, err)
		assert.Equal(t, "old", d.Ref, "percent=0 must never ramp %s", repo)

		d, err = Resolve(m100, "stable", repo)
		require.NoError(t, err)
		assert.Equal(t, "new", d.Ref, "percent=100 must always ramp %s", repo)
		assert.Equal(t, "ramp", d.Rule)
	}

	// The cutoff is exactly bucket < percent.
	repo := "acme/app"
	b := Bucket(repo)
	atCutoff := mustParse(t, fmt.Sprintf(manifest, b))
	d, err := Resolve(atCutoff, "stable", repo)
	require.NoError(t, err)
	assert.Equal(t, "old", d.Ref, "bucket == percent is NOT in the ramp")

	pastCutoff := mustParse(t, fmt.Sprintf(manifest, b+1))
	d, err = Resolve(pastCutoff, "stable", repo)
	require.NoError(t, err)
	assert.Equal(t, "new", d.Ref, "bucket < percent is in the ramp")
	assert.Equal(t, b, d.Bucket)
}

func TestResolveRampIsMonotonic(t *testing.T) {
	// A repo in the ramp at p stays in the ramp for every p' > p — cohorts only grow.
	for i := 0; i < 50; i++ {
		repo := fmt.Sprintf("org/app-%d", i)
		in := false
		for p := 0; p <= 100; p += 5 {
			m := mustParse(t, fmt.Sprintf(
				`{"channels":{"stable":{"ref":"old","ramp":{"ref":"new","percent":%d}}}}`, p))
			d, err := Resolve(m, "stable", repo)
			require.NoError(t, err)
			nowIn := d.Ref == "new"
			assert.False(t, in && !nowIn, "%s left the ramp when percent rose to %d", repo, p)
			in = nowIn
		}
		assert.True(t, in, "at 100%% every repo ramps")
	}
}

func TestDecideExplicitRefWins(t *testing.T) {
	m := mustParse(t, `{"channels":{"stable":{"ref":"v1"}}}`)

	d, err := Decide("deadbeef", m, "stable", "acme/app")
	require.NoError(t, err)
	assert.Equal(t, "deadbeef", d.Ref)
	assert.Equal(t, "explicit", d.Rule)

	// No explicit ref → falls through to channel resolution.
	d, err = Decide("", m, "stable", "acme/app")
	require.NoError(t, err)
	assert.Equal(t, "v1", d.Ref)
	assert.Equal(t, "channel", d.Rule)
}
