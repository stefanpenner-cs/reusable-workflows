package core

import (
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// reusableUses matches a job-level reusable-workflow uses: owner/repo/.github/workflows/<file>@<ref>.
var reusableUses = regexp.MustCompile(`^([^/]+/[^/]+)/(\.github/workflows/[^@]+)@.+$`)

// ReferencesWorkflowsRepo reports whether any job calls workflowsRepo as a reusable workflow — used
// to decide which of a consumer's workflows to mirror-transform.
func ReferencesWorkflowsRepo(yamlText, workflowsRepo string) (bool, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(yamlText), &doc); err != nil {
		return false, err
	}
	jobs := mapGet(docRoot(&doc), "jobs")
	if jobs == nil || jobs.Kind != yaml.MappingNode {
		return false, nil
	}
	for i := 1; i < len(jobs.Content); i += 2 {
		uses := mapGet(jobs.Content[i], "uses")
		if uses == nil || uses.Kind != yaml.ScalarNode {
			continue
		}
		if m := reusableUses.FindStringSubmatch(uses.Value); m != nil && strings.EqualFold(m[1], workflowsRepo) {
			return true, nil
		}
	}
	return false, nil
}

// PatchConsumerWorkflow repoints a consumer's reusable-workflow call at workflowsRef and injects
// with.ref, preserving comments/formatting. Only job-level uses targeting workflowsRepo are touched
// (step-level action uses and other repos' workflows are left alone). Idempotent.
func PatchConsumerWorkflow(yamlText, workflowsRepo, workflowsRef string) (string, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(yamlText), &doc); err != nil {
		return "", err
	}
	jobs := mapGet(docRoot(&doc), "jobs")
	if jobs == nil || jobs.Kind != yaml.MappingNode {
		return marshalNode(&doc)
	}
	for i := 1; i < len(jobs.Content); i += 2 {
		job := jobs.Content[i]
		if job.Kind != yaml.MappingNode {
			continue
		}
		uses := mapGet(job, "uses")
		if uses == nil || uses.Kind != yaml.ScalarNode {
			continue
		}
		m := reusableUses.FindStringSubmatch(uses.Value)
		if m == nil || !strings.EqualFold(m[1], workflowsRepo) {
			continue
		}
		// Preserve the consumer's own owner/repo casing (GitHub is case-insensitive) — only the ref
		// after @ is rewritten.
		repo, path := m[1], m[2]
		if strings.HasSuffix(path, ".yml") { // fix the .yml -> .yaml typo
			path = strings.TrimSuffix(path, ".yml") + ".yaml"
		}
		*uses = yaml.Node{Kind: yaml.ScalarNode, Value: repo + "/" + path + "@" + workflowsRef}

		switch with := mapGet(job, "with"); {
		case with != nil && with.Kind == yaml.MappingNode:
			mapSetScalar(with, "ref", workflowsRef)
		case with != nil && with.Kind == yaml.AliasNode && with.Alias != nil && with.Alias.Kind == yaml.MappingNode:
			// `with: *anchor` — copy the anchored inputs into a fresh mapping (never mutate the
			// shared anchor) and override ref, so the consumer's inputs survive the repoint.
			merged := &yaml.Node{Kind: yaml.MappingNode, Content: append([]*yaml.Node{}, with.Alias.Content...)}
			mapSetScalar(merged, "ref", workflowsRef)
			mapSetNode(job, "with", merged)
		default:
			mapSetNode(job, "with", &yaml.Node{
				Kind:    yaml.MappingNode,
				Content: []*yaml.Node{scalarNode("ref"), scalarNode(workflowsRef)},
			})
		}
	}
	return marshalNode(&doc)
}
