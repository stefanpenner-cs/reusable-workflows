package ghactions

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRenderOutputs(t *testing.T) {
	got, err := RenderOutputs([]Pair{{"node_version", "20"}})
	require.NoError(t, err)
	assert.Equal(t, "node_version=20\n", got)

	got, err = RenderOutputs([]Pair{{"a", "1"}, {"b", "2"}})
	require.NoError(t, err)
	assert.Equal(t, "a=1\nb=2\n", got)
}

func TestRenderOutputsMultilineUsesHeredoc(t *testing.T) {
	// A legitimately multiline value must use GitHub's heredoc form, and the delimiter must not
	// appear as a line in the value (else it would terminate early).
	got, err := RenderOutputs([]Pair{{"body", "line1\nline2"}})
	require.NoError(t, err)
	require.Contains(t, got, "line1\nline2\n")
	lines := strings.SplitN(got, "\n", 2)
	require.True(t, strings.HasPrefix(lines[0], "body<<"), "want heredoc header, got %q", lines[0])
	delim := strings.TrimPrefix(lines[0], "body<<")
	assert.NotContains(t, "line1\nline2", delim)
	assert.Equal(t, 2, strings.Count(got, delim), "delimiter opens and closes exactly once")
}

func TestRenderOutputsRejectsInjectionViaKey(t *testing.T) {
	// A newline or '=' in the KEY is how output injection is smuggled; reject loudly.
	for _, bad := range []string{"a\ninjected", "a=b", "a<<EOF", "a\rb"} {
		_, err := RenderOutputs([]Pair{{bad, "v"}})
		assert.Error(t, err, "key %q must be rejected", bad)
	}
}

func TestAppendOutputRejectsBadKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out")
	assert.Error(t, AppendOutput(path, []Pair{{"k\nevil", "v"}}))
}

func TestAppendOutputWritesPairs(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out")
	require.NoError(t, AppendOutput(path, []Pair{{"pr", "7"}, {"sha", "abc"}}))
	b, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "pr=7\nsha=abc\n", string(b))
}

func TestAppendOutputErrorsWhenUnset(t *testing.T) {
	assert.Error(t, AppendOutput("", []Pair{{"x", "1"}}))
}

func TestAppendFileAppends(t *testing.T) {
	path := filepath.Join(t.TempDir(), "summary")
	require.NoError(t, AppendFile(path, "## one\n"))
	require.NoError(t, AppendFile(path, "## two\n"))
	b, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "## one\n## two\n", string(b))
}

func TestAppendFileErrorsWhenEmptyPath(t *testing.T) {
	assert.Error(t, AppendFile("", "x"))
}
