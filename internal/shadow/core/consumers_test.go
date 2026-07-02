package core

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseConsumers(t *testing.T) {
	got, err := ParseConsumers(`[{"repo":"stefanpenner-cs/reusable-workflows-consumer","ref":"main"}]`)
	require.NoError(t, err)
	assert.Equal(t, []Consumer{{Repo: "stefanpenner-cs/reusable-workflows-consumer", Ref: "main"}}, got)
}

func TestParseConsumersDefaultsRef(t *testing.T) {
	got, err := ParseConsumers(`[{"repo":"o/r"}]`)
	require.NoError(t, err)
	assert.Equal(t, []Consumer{{Repo: "o/r", Ref: "main"}}, got)
}

func TestParseConsumersAllowsEmpty(t *testing.T) {
	got, err := ParseConsumers(`[]`)
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestParseConsumersRejectsBadInput(t *testing.T) {
	_, err := ParseConsumers("not json")
	assert.Error(t, err)
	_, err = ParseConsumers(`[{"ref":"main"}]`) // repo missing
	assert.Error(t, err)
	_, err = ParseConsumers(`[{"repo":"nope"}]`) // not owner/name
	assert.Error(t, err)
	_, err = ParseConsumers(`{"repo":"o/r"}`) // not an array
	assert.Error(t, err)
}

func TestCheckMatrixLimit(t *testing.T) {
	assert.NoError(t, CheckMatrixLimit(0))
	assert.NoError(t, CheckMatrixLimit(MaxShadowConsumers))
	err := CheckMatrixLimit(MaxShadowConsumers + 1)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "matrix limit")
}

func TestParseConsumersRejectsShellMetachars(t *testing.T) {
	// consumer.repo flows into a bazelisk run: arg — a metacharacter would be an injection sink.
	for _, bad := range []string{`a/b;touch x`, "a/b|c", "a/`whoami`", "a/b$IFS", "a b/c", "a/b&d"} {
		_, err := ParseConsumers(`[{"repo":"` + bad + `"}]`)
		assert.Error(t, err, "repo %q must be rejected", bad)
	}
}
