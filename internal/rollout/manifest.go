// Package rollout is the pure control-plane logic for releasing this repo's workflows to a fleet
// of consumers: a channel manifest (ref per channel, optional percentage ramp, per-repo
// overrides), deterministic repo bucketing, and the resolve/edit operations the rollout CLIs use.
package rollout

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// Ramp stages a candidate ref to a percentage of the channel's consumers.
type Ramp struct {
	Ref     string `json:"ref"`
	Percent int    `json:"percent"`
}

// Channel maps a named release track to the ref its consumers run.
type Channel struct {
	Ref       string            `json:"ref"`
	Previous  string            `json:"previous,omitempty"`
	Ramp      *Ramp             `json:"ramp,omitempty"`
	Overrides map[string]string `json:"overrides,omitempty"`
}

// Manifest is the rollout control plane: every channel and its current state.
type Manifest struct {
	Channels map[string]Channel `json:"channels"`
}

// overrideRepoRe: GitHub owner/repo names are [A-Za-z0-9._-] only — strict so an override key can't
// smuggle shell metacharacters into anything that later interpolates it.
var overrideRepoRe = regexp.MustCompile(`^[A-Za-z0-9._-]+/[A-Za-z0-9._-]+$`)

// ParseManifest parses and strictly validates a rollout manifest. Unknown fields are errors so a
// typo'd edit fails loudly instead of silently changing nothing.
func ParseManifest(data []byte) (Manifest, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var m Manifest
	if err := dec.Decode(&m); err != nil {
		return Manifest{}, fmt.Errorf("invalid rollout manifest: %w", err)
	}
	if len(m.Channels) == 0 {
		return Manifest{}, fmt.Errorf("invalid rollout manifest: no channels defined")
	}
	for name, ch := range m.Channels {
		if err := validateChannel(name, ch); err != nil {
			return Manifest{}, err
		}
	}
	return m, nil
}

func validateChannel(name string, ch Channel) error {
	if ch.Ref == "" {
		return fmt.Errorf("channel %q: ref must be non-empty", name)
	}
	if ch.Ramp != nil {
		if ch.Ramp.Ref == "" {
			return fmt.Errorf("channel %q: ramp ref must be non-empty", name)
		}
		if ch.Ramp.Percent < 0 || ch.Ramp.Percent > 100 {
			return fmt.Errorf("channel %q: ramp percent must be 0..100, got %d", name, ch.Ramp.Percent)
		}
	}
	for repo, ref := range ch.Overrides {
		if !overrideRepoRe.MatchString(repo) {
			return fmt.Errorf("channel %q: override %q: expected \"owner/name\"", name, repo)
		}
		if ref == "" {
			return fmt.Errorf("channel %q: override %q: ref must be non-empty", name, repo)
		}
	}
	return nil
}

// Render serializes a manifest deterministically (encoding/json sorts map keys) with a trailing
// newline, so re-rendering an unchanged manifest is a byte-for-byte no-op.
func Render(m Manifest) ([]byte, error) {
	out, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("render rollout manifest: %w", err)
	}
	return append(out, '\n'), nil
}

// channelNames returns the sorted channel names, for actionable unknown-channel errors.
func channelNames(m Manifest) string {
	names := make([]string, 0, len(m.Channels))
	for name := range m.Channels {
		names = append(names, name)
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}
