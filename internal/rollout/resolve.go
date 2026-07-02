package rollout

import (
	"fmt"
	"hash/fnv"
	"strings"
)

// Decision is a resolved ref plus why it was chosen — the reason is surfaced in consumer logs so
// an operator can tell at a glance which cohort a repo landed in.
type Decision struct {
	Ref    string
	Rule   string // "explicit" | "override" | "ramp" | "channel"
	Bucket int
}

// Bucket deterministically maps a repo to 0..99. Repo names are case-insensitive on GitHub, so
// the hash is taken over the lowercased name; a repo keeps its bucket forever, which makes ramp
// cohorts stable (raising percent only ever adds repos, it never swaps them).
func Bucket(repo string) int {
	h := fnv.New64a()
	_, _ = h.Write([]byte(strings.ToLower(repo)))
	return int(h.Sum64() % 100)
}

// Resolve picks the ref a consumer repo should run for a channel: per-repo override first, then
// the ramp cohort (bucket < percent), then the channel ref.
func Resolve(m Manifest, channel, repo string) (Decision, error) {
	ch, ok := m.Channels[channel]
	if !ok {
		return Decision{}, fmt.Errorf("unknown channel %q (have: %s)", channel, channelNames(m))
	}
	bucket := Bucket(repo)
	for overrideRepo, ref := range ch.Overrides {
		if strings.EqualFold(overrideRepo, repo) {
			return Decision{Ref: ref, Rule: "override", Bucket: bucket}, nil
		}
	}
	if ch.Ramp != nil && bucket < ch.Ramp.Percent {
		return Decision{Ref: ch.Ramp.Ref, Rule: "ramp", Bucket: bucket}, nil
	}
	return Decision{Ref: ch.Ref, Rule: "channel", Bucket: bucket}, nil
}

// Decide is the full resolution policy: an explicit ref (the consumer's escape hatch / pin)
// bypasses the manifest entirely; otherwise the channel resolves via Resolve.
func Decide(explicitRef string, m Manifest, channel, repo string) (Decision, error) {
	if explicitRef != "" {
		return Decision{Ref: explicitRef, Rule: "explicit", Bucket: Bucket(repo)}, nil
	}
	return Resolve(m, channel, repo)
}
