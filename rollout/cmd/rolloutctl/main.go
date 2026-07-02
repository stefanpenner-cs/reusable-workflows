// Command rolloutctl is the operator CLI for the rollout control plane: stage a ramp, promote it,
// roll back, validate the manifest, or preview a ramp's cohort split. Every mutation is a pure
// edit in internal/rollout re-rendered deterministically, so committing the result is the whole
// fleet action — thousands of consumers pick it up on their next run via shared.yaml's resolve step.
package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/stefanpenner-cs/reusable-workflows/internal/rollout"
)

func main() {
	if err := root().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}

func root() *cobra.Command {
	var manifestPath string
	root := &cobra.Command{
		Use:           "rolloutctl",
		Short:         "Operate the reusable-workflows rollout control plane",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.PersistentFlags().StringVar(&manifestPath, "manifest", ".github/rollout.json", "path to the rollout manifest JSON")

	root.AddCommand(
		rampCmd(&manifestPath),
		promoteCmd(&manifestPath),
		rollbackCmd(&manifestPath),
		validateCmd(&manifestPath),
		planCmd(&manifestPath),
	)
	return root
}

// editManifest loads, transforms, and writes the manifest back deterministically. A no-op edit
// rewrites identical bytes, so re-running is safe.
func editManifest(path string, fn func(rollout.Manifest) (rollout.Manifest, error)) error {
	m, err := load(path)
	if err != nil {
		return err
	}
	next, err := fn(m)
	if err != nil {
		return err
	}
	out, err := rollout.Render(next)
	if err != nil {
		return err
	}
	return os.WriteFile(path, out, 0o644)
}

func load(path string) (rollout.Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return rollout.Manifest{}, err
	}
	return rollout.ParseManifest(data)
}

func rampCmd(path *string) *cobra.Command {
	var channel, ref string
	var percent int
	cmd := &cobra.Command{
		Use:   "ramp",
		Short: "Stage a candidate ref to a percentage of a channel",
		RunE: func(_ *cobra.Command, _ []string) error {
			if channel == "" || ref == "" {
				return fmt.Errorf("--channel and --ref are required")
			}
			if err := editManifest(*path, func(m rollout.Manifest) (rollout.Manifest, error) {
				return rollout.SetRamp(m, channel, ref, percent)
			}); err != nil {
				return err
			}
			fmt.Printf("✅ ramped %q → %s at %d%%\n", channel, ref, percent)
			return nil
		},
	}
	cmd.Flags().StringVar(&channel, "channel", "", "channel to ramp")
	cmd.Flags().StringVar(&ref, "ref", "", "candidate ref to ramp toward")
	cmd.Flags().IntVar(&percent, "percent", 0, "percentage of the channel's repos (0..100)")
	return cmd
}

func promoteCmd(path *string) *cobra.Command {
	var channel string
	cmd := &cobra.Command{
		Use:   "promote",
		Short: "Promote a channel's in-progress ramp to its full ref",
		RunE: func(_ *cobra.Command, _ []string) error {
			if channel == "" {
				return fmt.Errorf("--channel is required")
			}
			if err := editManifest(*path, func(m rollout.Manifest) (rollout.Manifest, error) {
				return rollout.Promote(m, channel)
			}); err != nil {
				return err
			}
			fmt.Printf("✅ promoted the ramp on %q\n", channel)
			return nil
		},
	}
	cmd.Flags().StringVar(&channel, "channel", "", "channel to promote")
	return cmd
}

func rollbackCmd(path *string) *cobra.Command {
	var channel string
	cmd := &cobra.Command{
		Use:   "rollback",
		Short: "Abort a ramp, or revert a channel to its previous ref",
		RunE: func(_ *cobra.Command, _ []string) error {
			if channel == "" {
				return fmt.Errorf("--channel is required")
			}
			if err := editManifest(*path, func(m rollout.Manifest) (rollout.Manifest, error) {
				return rollout.Rollback(m, channel)
			}); err != nil {
				return err
			}
			fmt.Printf("✅ rolled back %q\n", channel)
			return nil
		},
	}
	cmd.Flags().StringVar(&channel, "channel", "", "channel to roll back")
	return cmd
}

func validateCmd(path *string) *cobra.Command {
	return &cobra.Command{
		Use:   "validate",
		Short: "Validate the manifest (and confirm it is canonically formatted)",
		RunE: func(_ *cobra.Command, _ []string) error {
			data, err := os.ReadFile(*path)
			if err != nil {
				return err
			}
			m, err := rollout.ParseManifest(data)
			if err != nil {
				return err
			}
			rendered, err := rollout.Render(m)
			if err != nil {
				return err
			}
			if string(rendered) != string(data) {
				return fmt.Errorf("%s is not canonically formatted — run `rolloutctl` edits or reformat", *path)
			}
			fmt.Printf("✅ %s is valid (%d channel(s))\n", *path, len(m.Channels))
			return nil
		},
	}
}

func planCmd(path *string) *cobra.Command {
	var channel, reposFile string
	cmd := &cobra.Command{
		Use:   "plan",
		Short: "Preview the cohort split of a channel across a repo list",
		RunE: func(_ *cobra.Command, _ []string) error {
			if channel == "" || reposFile == "" {
				return fmt.Errorf("--channel and --repos-file are required")
			}
			m, err := load(*path)
			if err != nil {
				return err
			}
			repos, err := loadRepos(reposFile)
			if err != nil {
				return err
			}
			plan, err := rollout.Plan(m, channel, repos)
			if err != nil {
				return err
			}
			fmt.Printf("channel %q — %d repo(s)\n", plan.Channel, plan.Total)
			for _, ref := range plan.SortedRefs() {
				n := plan.ByRef[ref]
				fmt.Printf("  %-24s %5d  (%.1f%%)\n", ref, n, pct(n, plan.Total))
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&channel, "channel", "", "channel to preview")
	cmd.Flags().StringVar(&reposFile, "repos-file", "", "JSON array of \"owner/name\" strings")
	return cmd
}

func pct(n, total int) float64 {
	if total == 0 {
		return 0
	}
	return float64(n) / float64(total) * 100
}

// loadRepos reads a JSON array of "owner/name" strings.
func loadRepos(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var repos []string
	if err := json.Unmarshal(data, &repos); err != nil {
		return nil, fmt.Errorf("invalid repos file %s: %w", path, err)
	}
	return repos, nil
}
