package core

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

const workflowsRepo = "stefanpenner-cs/reusable-workflows"
const sha = "0123456789abcdef0123456789abcdef01234567"

func parseYAML(t *testing.T, s string) map[string]any {
	t.Helper()
	var m map[string]any
	require.NoError(t, yaml.Unmarshal([]byte(s), &m))
	return m
}

// nav walks nested map[string]any by keys.
func nav(m map[string]any, keys ...string) any {
	var cur any = m
	for _, k := range keys {
		cur = cur.(map[string]any)[k]
	}
	return cur
}

func patch(t *testing.T, in string) string {
	t.Helper()
	out, err := PatchConsumerWorkflow(in, workflowsRepo, sha)
	require.NoError(t, err)
	return out
}

func TestPatchRepointAndInjectRef(t *testing.T) {
	in := "name: Use Shared Workflow\non: { push: { branches: [main] } }\njobs:\n  ci:\n    uses: stefanpenner-cs/reusable-workflows/.github/workflows/shared.yaml@main\n"
	out := parseYAML(t, patch(t, in))
	assert.Equal(t, workflowsRepo+"/.github/workflows/shared.yaml@"+sha, nav(out, "jobs", "ci", "uses"))
	assert.Equal(t, sha, nav(out, "jobs", "ci", "with", "ref"))
}

func TestPatchFixesYmlTypo(t *testing.T) {
	in := "jobs:\n  ci:\n    uses: stefanpenner-cs/reusable-workflows/.github/workflows/shared.yml@main\n"
	out := parseYAML(t, patch(t, in))
	assert.Equal(t, workflowsRepo+"/.github/workflows/shared.yaml@"+sha, nav(out, "jobs", "ci", "uses"))
}

func TestPatchPreservesWithBlock(t *testing.T) {
	in := "jobs:\n  ci:\n    uses: stefanpenner-cs/reusable-workflows/.github/workflows/shared.yaml@main\n    with:\n      project-name: my-app\n"
	out := parseYAML(t, patch(t, in))
	with := nav(out, "jobs", "ci", "with").(map[string]any)
	assert.Equal(t, "my-app", with["project-name"])
	assert.Equal(t, sha, with["ref"])
}

func TestPatchOverwritesExistingRef(t *testing.T) {
	in := "jobs:\n  ci:\n    uses: stefanpenner-cs/reusable-workflows/.github/workflows/shared.yaml@v1\n    with: { ref: v1 }\n"
	out := parseYAML(t, patch(t, in))
	assert.Equal(t, sha, nav(out, "jobs", "ci", "with", "ref"))
}

func TestPatchLeavesOtherReposUntouched(t *testing.T) {
	in := "jobs:\n  other:\n    uses: someorg/other-repo/.github/workflows/build.yaml@main\n"
	out := parseYAML(t, patch(t, in))
	assert.Equal(t, "someorg/other-repo/.github/workflows/build.yaml@main", nav(out, "jobs", "other", "uses"))
	_, hasWith := nav(out, "jobs", "other").(map[string]any)["with"]
	assert.False(t, hasWith)
}

func TestPatchLeavesStepLevelUsesUntouched(t *testing.T) {
	in := "jobs:\n  build:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: actions/checkout@v4\n      - uses: stefanpenner-cs/reusable-workflows/actions/setup@main\n"
	out := parseYAML(t, patch(t, in))
	steps := nav(out, "jobs", "build", "steps").([]any)
	assert.Equal(t, "actions/checkout@v4", steps[0].(map[string]any)["uses"])
	assert.Equal(t, "stefanpenner-cs/reusable-workflows/actions/setup@main", steps[1].(map[string]any)["uses"])
}

func TestPatchEveryJob(t *testing.T) {
	in := "jobs:\n  a:\n    uses: stefanpenner-cs/reusable-workflows/.github/workflows/shared.yaml@main\n  b:\n    uses: stefanpenner-cs/reusable-workflows/.github/workflows/shared.yml@main\n"
	out := parseYAML(t, patch(t, in))
	assert.True(t, strings.HasSuffix(nav(out, "jobs", "a", "uses").(string), "@"+sha))
	assert.Equal(t, workflowsRepo+"/.github/workflows/shared.yaml@"+sha, nav(out, "jobs", "b", "uses"))
	assert.Equal(t, sha, nav(out, "jobs", "a", "with", "ref"))
	assert.Equal(t, sha, nav(out, "jobs", "b", "with", "ref"))
}

func TestPatchIsIdempotent(t *testing.T) {
	in := "jobs:\n  ci:\n    uses: stefanpenner-cs/reusable-workflows/.github/workflows/shared.yml@main\n    with: { project-name: my-app }\n"
	once := patch(t, in)
	twice, err := PatchConsumerWorkflow(once, workflowsRepo, sha)
	require.NoError(t, err)
	assert.Equal(t, once, twice)
}

func TestPatchPreservesComments(t *testing.T) {
	in := "# top-level comment\nname: Use Shared Workflow\njobs:\n  ci:\n    # call the shared workflow\n    uses: stefanpenner-cs/reusable-workflows/.github/workflows/shared.yaml@main\n"
	out := patch(t, in)
	assert.Contains(t, out, "# top-level comment")
	assert.Contains(t, out, "# call the shared workflow")
}

func TestPatchMatchesRepoCaseInsensitively(t *testing.T) {
	// GitHub owner/repo names are case-insensitive; a consumer that writes a different case must
	// still be repointed, else the shadow test silently validates the OLD ref (false pass).
	in := "jobs:\n  ci:\n    uses: StefanPenner-CS/Reusable-Workflows/.github/workflows/shared.yaml@v1\n"
	out := parseYAML(t, patch(t, in))
	assert.Equal(t, "StefanPenner-CS/Reusable-Workflows/.github/workflows/shared.yaml@"+sha, nav(out, "jobs", "ci", "uses"))
	assert.Equal(t, sha, nav(out, "jobs", "ci", "with", "ref"))
}

func TestReferencesWorkflowsRepoCaseInsensitive(t *testing.T) {
	mixed := "jobs:\n  ci:\n    uses: StefanPenner-CS/Reusable-Workflows/.github/workflows/shared.yaml@main\n"
	ok, err := ReferencesWorkflowsRepo(mixed, workflowsRepo)
	require.NoError(t, err)
	assert.True(t, ok)
}

func TestPatchPreservesAnchoredWithInputs(t *testing.T) {
	// `with: *anchor` is an alias node; the ref override must NOT wipe the anchored inputs.
	in := "x-common: &common\n  project-name: my-app\njobs:\n  ci:\n    uses: stefanpenner-cs/reusable-workflows/.github/workflows/shared.yaml@main\n    with: *common\n"
	out := parseYAML(t, patch(t, in))
	with := nav(out, "jobs", "ci", "with").(map[string]any)
	assert.Equal(t, "my-app", with["project-name"], "anchored inputs must survive")
	assert.Equal(t, sha, with["ref"])
}

func TestReferencesWorkflowsRepo(t *testing.T) {
	yes := "jobs:\n  ci:\n    uses: stefanpenner-cs/reusable-workflows/.github/workflows/shared.yaml@main\n"
	ok, err := ReferencesWorkflowsRepo(yes, workflowsRepo)
	require.NoError(t, err)
	assert.True(t, ok)

	noJob := "jobs:\n  build:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: actions/checkout@v4\n"
	ok, err = ReferencesWorkflowsRepo(noJob, workflowsRepo)
	require.NoError(t, err)
	assert.False(t, ok)

	other := "jobs:\n  ci:\n    uses: someorg/other/.github/workflows/x.yaml@main\n"
	ok, err = ReferencesWorkflowsRepo(other, workflowsRepo)
	require.NoError(t, err)
	assert.False(t, ok)
}
