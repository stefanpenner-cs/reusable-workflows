package rollout

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const validManifest = `{
  "channels": {
    "stable": {
      "ref": "v1.2.0",
      "previous": "v1.1.0",
      "ramp": { "ref": "v1.3.0-rc.1", "percent": 25 },
      "overrides": { "acme/legacy": "v1.0.5" }
    },
    "canary": { "ref": "main" }
  }
}`

func TestParseManifestValid(t *testing.T) {
	m, err := ParseManifest([]byte(validManifest))
	require.NoError(t, err)

	stable := m.Channels["stable"]
	assert.Equal(t, "v1.2.0", stable.Ref)
	assert.Equal(t, "v1.1.0", stable.Previous)
	require.NotNil(t, stable.Ramp)
	assert.Equal(t, "v1.3.0-rc.1", stable.Ramp.Ref)
	assert.Equal(t, 25, stable.Ramp.Percent)
	assert.Equal(t, "v1.0.5", stable.Overrides["acme/legacy"])

	canary := m.Channels["canary"]
	assert.Equal(t, "main", canary.Ref)
	assert.Nil(t, canary.Ramp)
}

func TestParseManifestRejectsBadInput(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"not json", `nope`, "invalid rollout manifest"},
		{"no channels", `{}`, "no channels"},
		{"empty channels", `{"channels":{}}`, "no channels"},
		{"empty ref", `{"channels":{"stable":{"ref":""}}}`, `channel "stable": ref must be non-empty`},
		{"unknown field (typo)", `{"channels":{"stable":{"ref":"v1","rammp":{}}}}`, "unknown field"},
		{"percent too high", `{"channels":{"stable":{"ref":"v1","ramp":{"ref":"v2","percent":101}}}}`,
			`channel "stable": ramp percent must be 0..100`},
		{"percent negative", `{"channels":{"stable":{"ref":"v1","ramp":{"ref":"v2","percent":-1}}}}`,
			`channel "stable": ramp percent must be 0..100`},
		{"ramp without ref", `{"channels":{"stable":{"ref":"v1","ramp":{"percent":10}}}}`,
			`channel "stable": ramp ref must be non-empty`},
		{"empty override ref", `{"channels":{"stable":{"ref":"v1","overrides":{"a/b":""}}}}`,
			`channel "stable": override "a/b": ref must be non-empty`},
		{"bad override repo", `{"channels":{"stable":{"ref":"v1","overrides":{"not-a-repo":"v1"}}}}`,
			`channel "stable": override "not-a-repo": expected "owner/name"`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseManifest([]byte(tc.in))
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.want)
		})
	}
}

func TestRenderRoundTrips(t *testing.T) {
	m, err := ParseManifest([]byte(validManifest))
	require.NoError(t, err)

	out, err := Render(m)
	require.NoError(t, err)

	back, err := ParseManifest(out)
	require.NoError(t, err)
	assert.Equal(t, m, back)

	// Deterministic: rendering twice yields identical bytes, ends with newline.
	out2, err := Render(m)
	require.NoError(t, err)
	assert.Equal(t, out, out2)
	assert.True(t, len(out) > 0 && out[len(out)-1] == '\n')
}
