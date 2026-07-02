// Command list-consumers (workflows setup): validate shadow-consumers.json, emit it as a matrix on
// $GITHUB_OUTPUT, and write the index of all shadow tests to the job summary.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/stefanpenner-cs/reusable-workflows/internal/ghactions"
	"github.com/stefanpenner-cs/reusable-workflows/internal/shadow/core"
)

func main() {
	var consumersFile, workflowsPR, runnerRepo string

	cmd := &cobra.Command{
		Use:           "list-consumers",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(_ *cobra.Command, _ []string) error {
			if err := ghactions.RequireFlags([]ghactions.Pair{
				{Key: "consumers-file", Value: consumersFile},
				{Key: "workflows-pr", Value: workflowsPR},
				{Key: "runner-repo", Value: runnerRepo},
			}); err != nil {
				return err
			}
			data, err := os.ReadFile(consumersFile)
			if err != nil {
				return err
			}
			consumers, err := core.ParseConsumers(string(data))
			if err != nil {
				return err
			}
			if err := core.CheckMatrixLimit(len(consumers)); err != nil {
				return err
			}
			matrix, err := json.Marshal(consumers)
			if err != nil {
				return err
			}
			if err := ghactions.AppendOutput(os.Getenv("GITHUB_OUTPUT"), []ghactions.Pair{
				{Key: "consumers", Value: string(matrix)},
			}); err != nil {
				return err
			}
			if summary := os.Getenv("GITHUB_STEP_SUMMARY"); summary != "" {
				pr, _ := strconv.Atoi(workflowsPR)
				if err := ghactions.AppendFile(summary, core.RenderShadowList(consumers, pr, runnerRepo)); err != nil {
					return err
				}
			}
			fmt.Printf("✅ %d consumer(s)\n", len(consumers))
			return nil
		},
	}
	f := cmd.Flags()
	f.StringVar(&consumersFile, "consumers-file", "", "path to shadow-consumers.json")
	f.StringVar(&workflowsPR, "workflows-pr", "", "workflows PR number")
	f.StringVar(&runnerRepo, "runner-repo", "", "owner/repo of the runner")

	if err := cmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}
