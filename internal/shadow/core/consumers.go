package core

import (
	"encoding/json"
	"fmt"
	"regexp"
)

// Consumer is a downstream repo we mirror to verify the workflows draft doesn't break it.
type Consumer struct {
	Repo string `json:"repo"`
	Ref  string `json:"ref"`
}

// repoRe is deliberately strict: GitHub owner/repo names are [A-Za-z0-9._-] only. The old
// `[^/\s]+` allowed shell metacharacters (`;`, `|`, `$`, backtick), and consumer.repo is
// interpolated into a bazelisk `run:` arg in shadow.yaml — a loose value would be a command-
// injection sink on the provider runner.
var repoRe = regexp.MustCompile(`^[A-Za-z0-9._-]+/[A-Za-z0-9._-]+$`)

// MaxShadowConsumers is GitHub's hard cap on jobs in one matrix. Past it the whole shadow workflow
// fails at expansion ("Matrix must not have more than 256 jobs"), so we reject earlier with a
// clear message. Shadow testing is a representative sample of the fleet, not the whole fleet — if
// you need more coverage, shard the consumers file across workflows.
const MaxShadowConsumers = 256

// CheckMatrixLimit fails if there are too many consumers to expand into a single GitHub matrix.
func CheckMatrixLimit(n int) error {
	if n > MaxShadowConsumers {
		return fmt.Errorf("%d consumers exceeds GitHub's %d-job matrix limit; shard shadow-consumers.json", n, MaxShadowConsumers)
	}
	return nil
}

// ParseConsumers parses + validates the workflows' shadow-consumers.json. ref defaults to "main".
func ParseConsumers(jsonStr string) ([]Consumer, error) {
	var data any
	if err := json.Unmarshal([]byte(jsonStr), &data); err != nil {
		return nil, fmt.Errorf("invalid consumers JSON: %w", err)
	}
	arr, ok := data.([]any)
	if !ok {
		return nil, fmt.Errorf("expected an array of consumers, got %T", data)
	}
	out := make([]Consumer, 0, len(arr))
	for i, entry := range arr {
		c, err := parseConsumer(entry, i)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, nil
}

func parseConsumer(entry any, index int) (Consumer, error) {
	obj, ok := entry.(map[string]any)
	if !ok {
		return Consumer{}, fmt.Errorf("consumer[%d]: expected an object", index)
	}
	repo, ok := obj["repo"].(string)
	if !ok || !repoRe.MatchString(repo) {
		return Consumer{}, fmt.Errorf("consumer[%d].repo: expected \"owner/name\", got %v", index, obj["repo"])
	}
	ref := "main"
	if raw, present := obj["ref"]; present {
		s, ok := raw.(string)
		if !ok || s == "" {
			return Consumer{}, fmt.Errorf("consumer[%d].ref: expected a non-empty string, got %v", index, raw)
		}
		ref = s
	}
	return Consumer{Repo: repo, Ref: ref}, nil
}
