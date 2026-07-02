package rollout

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSetRamp(t *testing.T) {
	m := mustParse(t, `{"channels":{"stable":{"ref":"v1"}}}`)

	out, err := SetRamp(m, "stable", "v2", 10)
	require.NoError(t, err)
	require.NotNil(t, out.Channels["stable"].Ramp)
	assert.Equal(t, "v2", out.Channels["stable"].Ramp.Ref)
	assert.Equal(t, 10, out.Channels["stable"].Ramp.Percent)

	// Input manifest is not mutated.
	assert.Nil(t, m.Channels["stable"].Ramp)

	// Re-ramping the same ref just moves the percent.
	out2, err := SetRamp(out, "stable", "v2", 50)
	require.NoError(t, err)
	assert.Equal(t, 50, out2.Channels["stable"].Ramp.Percent)
}

func TestSetRampValidates(t *testing.T) {
	m := mustParse(t, `{"channels":{"stable":{"ref":"v1"}}}`)

	_, err := SetRamp(m, "nope", "v2", 10)
	require.Error(t, err)
	assert.Contains(t, err.Error(), `unknown channel "nope"`)

	_, err = SetRamp(m, "stable", "", 10)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ref must be non-empty")

	_, err = SetRamp(m, "stable", "v2", 101)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "percent must be 0..100")
}

func TestPromote(t *testing.T) {
	m := mustParse(t, `{"channels":{"stable":{"ref":"v1","previous":"v0","ramp":{"ref":"v2","percent":50}}}}`)

	out, err := Promote(m, "stable")
	require.NoError(t, err)
	ch := out.Channels["stable"]
	assert.Equal(t, "v2", ch.Ref, "ramp ref becomes the channel ref")
	assert.Equal(t, "v1", ch.Previous, "old ref is kept for rollback")
	assert.Nil(t, ch.Ramp, "ramp is cleared")

	// Original untouched.
	assert.Equal(t, "v1", m.Channels["stable"].Ref)
}

func TestPromoteRequiresRamp(t *testing.T) {
	m := mustParse(t, `{"channels":{"stable":{"ref":"v1"}}}`)
	_, err := Promote(m, "stable")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no ramp in progress")

	_, err = Promote(m, "nope")
	require.Error(t, err)
	assert.Contains(t, err.Error(), `unknown channel "nope"`)
}

func TestRollbackClearsRampFirst(t *testing.T) {
	m := mustParse(t, `{"channels":{"stable":{"ref":"v1","previous":"v0","ramp":{"ref":"v2","percent":50}}}}`)

	out, err := Rollback(m, "stable")
	require.NoError(t, err)
	ch := out.Channels["stable"]
	assert.Nil(t, ch.Ramp, "a rollback during a ramp aborts the ramp")
	assert.Equal(t, "v1", ch.Ref, "the channel ref is untouched")
	assert.Equal(t, "v0", ch.Previous)
}

func TestRollbackSwapsPrevious(t *testing.T) {
	m := mustParse(t, `{"channels":{"stable":{"ref":"v2","previous":"v1"}}}`)

	out, err := Rollback(m, "stable")
	require.NoError(t, err)
	ch := out.Channels["stable"]
	assert.Equal(t, "v1", ch.Ref, "rolls back to previous")
	assert.Equal(t, "v2", ch.Previous, "swap keeps roll-forward possible")
}

func TestRollbackWithNothingToRollBack(t *testing.T) {
	m := mustParse(t, `{"channels":{"stable":{"ref":"v1"}}}`)
	_, err := Rollback(m, "stable")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nothing to roll back")

	_, err = Rollback(m, "nope")
	require.Error(t, err)
	assert.Contains(t, err.Error(), `unknown channel "nope"`)
}
