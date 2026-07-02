// Command covergate runs `go test` with coverage over the gated (pure) packages and fails if line
// coverage is below -min. Go has no native function/branch coverage, so this is a line gate — the
// mains/bins/adapters are excluded by scoping -coverpkg to the pure layers. Run from the repo root
// (e.g. `go run ./tools/covergate -min 90`).
package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// gatedPackages are the pure layers held to the coverage bar (the analogue of the old
// --test-coverage-exclude for bins/*.cli/adapters: those simply aren't listed here).
var gatedPackages = []string{
	"./internal/ghactions/...",
	"./internal/actions/...",
	"./internal/noinlinescripts/...",
	"./internal/shadow/core/...",
	"./internal/rollout/...",
}

func main() {
	min := flag.Float64("min", 90, "minimum line coverage percentage")
	flag.Parse()

	// `bazel run` executes in the runfiles tree; BUILD_WORKSPACE_DIRECTORY points back at the repo
	// root so the `go test ./internal/...` below resolves.
	if ws := os.Getenv("BUILD_WORKSPACE_DIRECTORY"); ws != "" {
		if err := os.Chdir(ws); err != nil {
			fail(err)
		}
	}

	profile, err := os.CreateTemp("", "cover-*.out")
	if err != nil {
		fail(err)
	}
	defer os.Remove(profile.Name())
	profile.Close()

	// No -coverpkg: each package is measured by its own tests. Spanning -coverpkg across the
	// gated packages structurally under-counts (cross-binary zero-count blocks), so we list the
	// pure layers as the test targets and let each measure itself; bins/adapters aren't listed.
	args := append([]string{"test", "-covermode=count",
		"-coverprofile=" + profile.Name()}, gatedPackages...)
	cmd := exec.Command("go", args...)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		fail(fmt.Errorf("go test failed: %w", err))
	}

	pct, err := coverage(profile.Name())
	if err != nil {
		fail(err)
	}
	fmt.Printf("line coverage: %.1f%% (min %.1f%%)\n", pct, *min)
	if pct < *min {
		fmt.Fprintf(os.Stderr, "✗ coverage %.1f%% is below the %.1f%% gate\n", pct, *min)
		os.Exit(1)
	}
	fmt.Println("✓ coverage gate met")
}

type block struct {
	numStmt int
	covered bool
}

// coverage computes overall line coverage from a Go coverprofile: covered statements / total. With
// multi-package -coverpkg the merged profile repeats each block once per test binary, so blocks are
// deduped by their `file:start,end` key (covered if hit in any run) — matching `go tool cover`.
func coverage(path string) (float64, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	blocks := map[string]block{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if line == "" || strings.HasPrefix(line, "mode:") {
			continue
		}
		// format: name.go:line.col,line.col numStmt count
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		numStmt, err1 := strconv.Atoi(fields[len(fields)-2])
		count, err2 := strconv.Atoi(fields[len(fields)-1])
		if err1 != nil || err2 != nil {
			continue
		}
		key := fields[0]
		b := blocks[key]
		b.numStmt = numStmt
		b.covered = b.covered || count > 0
		blocks[key] = b
	}
	if err := sc.Err(); err != nil {
		return 0, err
	}

	var total, covered int
	for _, b := range blocks {
		total += b.numStmt
		if b.covered {
			covered += b.numStmt
		}
	}
	if total == 0 {
		return 0, fmt.Errorf("no coverage data in %s", path)
	}
	return float64(covered) / float64(total) * 100, nil
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "covergate:", err)
	os.Exit(1)
}
