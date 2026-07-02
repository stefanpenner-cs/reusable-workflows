package rollout

import "sort"

// PlanResult is the cohort breakdown of resolving a channel across a set of repos — the preview an
// operator reads before raising a ramp percentage.
type PlanResult struct {
	Channel string
	Total   int
	ByRule  map[string]int // rule ("ramp"/"channel"/"override") → repo count
	ByRef   map[string]int // resolved ref → repo count
}

// Plan resolves channel for every repo and tallies the cohort split, so a ramp change can be
// previewed ("47 of 1,000 repos move to v2") before it's committed.
func Plan(m Manifest, channel string, repos []string) (PlanResult, error) {
	res := PlanResult{Channel: channel, ByRule: map[string]int{}, ByRef: map[string]int{}}
	for _, repo := range repos {
		d, err := Resolve(m, channel, repo)
		if err != nil {
			return PlanResult{}, err
		}
		res.Total++
		res.ByRule[d.Rule]++
		res.ByRef[d.Ref]++
	}
	return res, nil
}

// SortedRefs returns the refs in the plan, most-used first (stable tie-break by ref), for
// deterministic rendering.
func (p PlanResult) SortedRefs() []string {
	refs := make([]string, 0, len(p.ByRef))
	for ref := range p.ByRef {
		refs = append(refs, ref)
	}
	sort.Slice(refs, func(i, j int) bool {
		if p.ByRef[refs[i]] != p.ByRef[refs[j]] {
			return p.ByRef[refs[i]] > p.ByRef[refs[j]]
		}
		return refs[i] < refs[j]
	})
	return refs
}
