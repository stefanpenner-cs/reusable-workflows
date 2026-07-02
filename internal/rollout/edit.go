package rollout

import "fmt"

// SetRamp returns a copy of the manifest with a ramp staged on the channel (or the existing
// ramp's percent moved). The input manifest is never mutated.
func SetRamp(m Manifest, channel, ref string, percent int) (Manifest, error) {
	ch, ok := m.Channels[channel]
	if !ok {
		return Manifest{}, fmt.Errorf("unknown channel %q (have: %s)", channel, channelNames(m))
	}
	ch.Ramp = &Ramp{Ref: ref, Percent: percent}
	if err := validateChannel(channel, ch); err != nil {
		return Manifest{}, err
	}
	return withChannel(m, channel, ch), nil
}

// Promote returns a copy of the manifest with the channel's ramp ref promoted to the channel ref;
// the old ref is kept as previous so Rollback can undo the promotion.
func Promote(m Manifest, channel string) (Manifest, error) {
	ch, ok := m.Channels[channel]
	if !ok {
		return Manifest{}, fmt.Errorf("unknown channel %q (have: %s)", channel, channelNames(m))
	}
	if ch.Ramp == nil {
		return Manifest{}, fmt.Errorf("channel %q: no ramp in progress to promote", channel)
	}
	ch.Previous = ch.Ref
	ch.Ref = ch.Ramp.Ref
	ch.Ramp = nil
	return withChannel(m, channel, ch), nil
}

// Rollback returns a copy of the manifest one safe step back: an in-progress ramp is aborted
// (the channel ref is untouched); with no ramp, ref and previous swap, so rolling back twice
// rolls forward again rather than losing state.
func Rollback(m Manifest, channel string) (Manifest, error) {
	ch, ok := m.Channels[channel]
	if !ok {
		return Manifest{}, fmt.Errorf("unknown channel %q (have: %s)", channel, channelNames(m))
	}
	switch {
	case ch.Ramp != nil:
		ch.Ramp = nil
	case ch.Previous != "":
		ch.Ref, ch.Previous = ch.Previous, ch.Ref
	default:
		return Manifest{}, fmt.Errorf("channel %q: nothing to roll back (no ramp, no previous)", channel)
	}
	return withChannel(m, channel, ch), nil
}

// withChannel copies the manifest with one channel replaced — edits stay pure.
func withChannel(m Manifest, name string, ch Channel) Manifest {
	out := Manifest{Channels: make(map[string]Channel, len(m.Channels))}
	for k, v := range m.Channels {
		out.Channels[k] = v
	}
	out.Channels[name] = ch
	return out
}
