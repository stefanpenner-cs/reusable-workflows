// Command resolve (consumer runtime): given the rollout manifest, a channel, and the calling repo,
// decide which provider ref this repo should run and emit it on $GITHUB_OUTPUT as `ref`. This is
// the control-plane read that shared.yaml performs before checking the actions out — the fleet's
// ramp/rollback state lives in the manifest, not in each consumer.
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/stefanpenner-cs/reusable-workflows/internal/ghactions"
	"github.com/stefanpenner-cs/reusable-workflows/internal/rollout"
)

func main() {
	var manifestPath, channel, repo, explicitRef string

	cmd := &cobra.Command{
		Use:           "resolve",
		Short:         "Resolve the provider ref for a repo on a channel",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(_ *cobra.Command, _ []string) error {
			if err := ghactions.RequireFlags([]ghactions.Pair{
				{Key: "manifest", Value: manifestPath},
				{Key: "channel", Value: channel},
				{Key: "repo", Value: repo},
			}); err != nil {
				return err
			}
			data, err := os.ReadFile(manifestPath)
			if err != nil {
				return err
			}
			m, err := rollout.ParseManifest(data)
			if err != nil {
				return err
			}
			d, err := rollout.Decide(explicitRef, m, channel, repo)
			if err != nil {
				return err
			}
			if err := ghactions.AppendOutput(os.Getenv("GITHUB_OUTPUT"), []ghactions.Pair{
				{Key: "ref", Value: d.Ref},
				{Key: "rule", Value: d.Rule},
			}); err != nil {
				return err
			}
			fmt.Printf("✅ %s on channel %q → ref %s (%s, bucket %d)\n", repo, channel, d.Ref, d.Rule, d.Bucket)
			return nil
		},
	}
	f := cmd.Flags()
	f.StringVar(&manifestPath, "manifest", "", "path to the rollout manifest JSON")
	f.StringVar(&channel, "channel", "", "release channel name")
	f.StringVar(&repo, "repo", "", "the calling consumer repo (owner/name)")
	f.StringVar(&explicitRef, "ref", "", "explicit ref override (bypasses the channel)")

	if err := cmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}
