# TODO

Backlog for locking this down, refining functionality, and best-in-class DX. Roughly priority-ordered
within each section; 🔴 = security/correctness, 🟡 = important, 🟢 = nice-to-have.

## Rollout control plane (shipped)

Fleet release/ramp/rollback now lives in [`.github/rollout.json`](.github/rollout.json), operated by
`rolloutctl` (ramp/promote/rollback/validate/plan) and read at runtime by `shared.yaml`'s two-phase
checkout via `//rollout/cmd/resolve`. Runbook: [`ROLLOUT.md`](ROLLOUT.md). Follow-ups:

- 🟡 CI gate: `rolloutctl validate` on `rollout.json` in `test.yaml` (reject invalid/non-canonical).
- 🟡 Auto-drive `plan` off the real fleet list (not just `shadow-consumers.json`).
- 🟢 Time-boxed auto-ramp (advance % on a schedule if the cohort's CI is green).
- 🟢 Per-channel required-reviewer gate on manifest commits (CODEOWNERS on `rollout.json`).

## Fixed in the adversarial-review pass

- 🔴 **Composite-action arg injection** — inputs now travel through `env:` and are referenced as
  `"$VAR"`, never interpolated into the `run:` text (`actions/{lint,test,setup}`).
- 🔴 **`no-inline-scripts` bypass** — `bash -c`/`sh -c` and `go generate` are now rejected; the
  shell-operator check is quote-aware (no more false positives on `--x='<div>'`).
- 🔴 **`$GITHUB_OUTPUT` injection** — `RenderOutputs` rejects keys with `\n`/`=`/`<` and emits
  heredoc (collision-proof delimiter) for multiline values.
- 🔴 **Shadow watcher false PASS** — `watchCommitRun` now awaits *every* run for the commit, not
  `runs[0]`; a case-mismatched `uses:` repo is repointed (`EqualFold`).
- 🔴 **Shadow false FAIL at scale** — `awaitRun`/`watchCommitRun` retry rate-limit/5xx/transport
  errors with reset-aware backoff; every API call has a 30s timeout; permanent errors (401) surface
  instead of masquerading as timeouts.
- 🔴 **Cleanup silently leaked** — `closePRAndDeleteBranch` no longer swallows auth/rate errors
  (only 404/422 already-gone).
- 🟡 **`ensurePR` TOCTOU** — 422 already-exists re-fetches the winner's PR instead of false-failing.
- 🟡 **`MirrorTree`** — preserves exec bits and recreates symlinks (was 0644 + deref-crash).
- 🟡 **Anchored `with:`** — `with: *anchor` keeps its inputs on repoint (was wiped).
- 🟡 **`repoRe`** tightened to `[A-Za-z0-9._-]` (was `[^/\s]`, a metachar sink).
- 🟡 **Matrix cap** — `list-consumers` fails loudly past GitHub's 256-job limit.
- 🟡 **`shared.yaml` fail-open** removed — no more `${{ inputs.ref || 'main' }}`; resolve fails loud.
- 🟡 **`permissions: contents: read`** added to `shared.yaml`/`ci.yaml`/`test.yaml`.

## Remaining from the adversarial review

- 🟡 **Shadow watcher timeout budgets are inverted** — outer `WatchRun` (900s) < inner
  find+await worst case (~1100s + clones); a slow-but-passing consumer false-fails. Extract the
  budgets and assert outer > inner + overhead (`internal/shadow/adapters/github.go`).
- 🟡 **Cleanup only tears down branches derivable from the *current* consumers file** — a consumer
  removed from the file on the same PR leaks its shadow PR/branch. List runner branches by prefix
  `shadow/pr-<n>-` (paginated) and delete those.
- 🟡 **`strconv.Atoi` on `workflows-pr` ignored** → `shadow/pr-0-*` branches on the cleanup /
  dispatch paths (only `resolve-ctx` guards it). Validate numeric in core.
- 🟢 **Token in clone URL** leaks into git/exec error strings (`git.go`/`exec.go`) — masked in
  Actions logs, not elsewhere.
- 🟢 **`CloneShallow` can't take a SHA** though `ParseConsumers` accepts one as `ref`.
- 🟢 **`cancel-in-progress` can orphan** a runner-side shadow PR (self-heals on close; consider a
  cancel trap or `cancel-in-progress: false` for the privileged matrix).
- 🟢 **`uses:`/`on:` alias nodes** in a consumer workflow are skipped (rare); resolve or error.
- 🟢 **Deterministic-SHA rerun** re-reports a stale failed run for an unchanged head (document the
  flake-recovery path).
- 🟢 **Manual `workflow_dispatch` of shadow** has no "I reviewed this head" gate the label implies —
  note in `shadow/SECURITY.md`.

## Secrets → OIDC only 🔴

Eliminate the long-lived `SHADOW_PAT` — per [`shadow/SECURITY.md`](shadow/SECURITY.md) it's the
entire risk surface (a standing cross-repo write token). Move all cross-repo auth to short-lived,
OIDC-minted credentials:

- Exchange the Actions **OIDC token** for an ephemeral, repo-scoped credential (GitHub App
  installation token, or OIDC→token exchange): `actions:write` on the runner + `contents`/
  `pull-requests` write, time-boxed, per-run.
- Cover every privileged op: receiver `workflow_dispatch`, shadow PR create/close, branch
  push/delete (`internal/shadow/adapters/{git,github}.go`).
- Remove `SHADOW_PAT` from all workflows + secrets once OIDC is live; update `shadow/SECURITY.md`.
- **Goal: no standing secret anywhere — every shadow run uses an ephemeral credential.**

## Security / supply chain

- 🔴 **Static-analyze workflows in CI** — [`zizmor`](https://github.com/zizmorcore/zizmor) (`--persona pedantic` on the shadow workflows) + [`actionlint`](https://github.com/rhysd/actionlint). Catches template-injection / `pull_request_target` / `workflow_run` sinks a provider ships to every consumer.
- 🔴 **Audit the shadow execution pattern** — never run attacker-controlled refs with secrets in scope (`pull_request_target` + checkout-of-head is the canonical RCE); keep the privileged step from executing mirrored consumer code. [GH SecurityLab](https://securitylab.github.com/resources/github-actions-new-patterns-and-mitigations/)
- 🔴 **[harden-runner](https://github.com/step-security/harden-runner)** (egress `audit`→`block`) on the runner jobs that execute untrusted consumer CI — only control that detects secret/source exfiltration + C2 egress from that step.
- 🔴 **Gate the privileged shadow dispatch behind a GitHub Environment** with required reviewers + "prevent self-review"; scope the token as an **environment** secret (unreachable until approved) — a hard backstop during OIDC migration.
- 🔴 **Cache-poisoning isolation** — ensure untrusted shadow-PR runs can't write cache keys (Bazel/`actions/cache`) that trusted main/release jobs later read; per-PR namespacing or read-only restore on untrusted jobs. [writeup](https://adnanthekhan.com/2024/05/06/the-monsters-in-your-build-cache-github-actions-cache-poisoning/)
- 🔴 **No `${{ }}` of untrusted metadata in `run:`/bazelisk args** — route PR title/body/branch + consumer-repo strings through intermediate `env:` vars.
- 🟡 **Least-privilege `permissions:`** on every workflow (`test.yaml`/`ci.yaml` set none → broad default; most need only `contents: read`). Plus org default `GITHUB_TOKEN` read-only and disable "Actions can create/approve PRs" except where shadow needs it.
- 🟡 **SHA-pin all third-party actions** + enable `sha_pinning_required` (currently `false`); add `persist-credentials: false` on checkouts that don't push.
- 🟡 **Artifact attestations / SLSA provenance** for released action binaries ([`attest-build-provenance`](https://docs.github.com/actions/security-guides/using-artifact-attestations-and-reusable-workflows-to-achieve-slsa-v1-build-level-3)) + `gh attestation verify` in the `checkout-anywhere` bootstrap before exec.
- 🟡 **Validate untrusted receiver inputs** at the Go entrypoint (allow-list provider + consumer repos; require 40-hex `workflows_ref`) — flagged in `receiver.yaml`/`SECURITY.md`, not enforced.
- 🟡 **`govulncheck`** + **Dependabot/Renovate** (`go.mod`, `MODULE.bazel`, `.bazelversion`, action SHAs).
- 🟢 **CODEOWNERS** for `shared.yaml`/`actions/**`/`shadow/**`; scheduled **OpenSSF [Scorecard](https://github.com/ossf/scorecard)**; **branch protection** on `main` (require `test`+`shared / ci` + review); **signed tags/commits** (gitsign/GPG).

## Release / versioning

- 🟡 Tag a moving **`v1`** + semver; automate with **[release-please](https://docs.github.com/en/actions/how-tos/create-and-publish-actions/release-and-maintain-actions)** (re-point `v1` on release) + a `CHANGELOG.md` from Conventional Commits. Consumers float on `@main` today.
- 🟡 **Deprecation / upgrade guidance** (esp. the OIDC change → a breaking `v2`) + a repo `SECURITY.md` policy.
- 🟢 **Decision (record it): do NOT publish to Marketplace** — actions live in `actions/<x>/` subdirs and need Bazel + the `checkout-anywhere` bootstrap, which violates Marketplace rules. They're internal-only composite actions.
- 🟢 **`branding:`** (icon/color) on each `action.yaml`; **status badges** on the README.

## CI / build (Bazel)

- 🟡 **Kill the cold-Bazel tax** (~3 min/consumer job): remote Bazel cache (BuildBuddy/GCS) and/or **prebuilt released static binaries** the actions exec instead of building. Interim: `bazelisk build` once + exec the artifacts rather than N `bazelisk run` (each re-runs analysis).
- 🟡 **CI drift gates**: `gazelle -mode=diff`, `bazel mod tidy` + `git diff --exit-code`, and `MODULE.bazel.lock` unchanged — fail fast on BUILD/MODULE/lock drift.
- 🟡 **`.bazelrc` hardening**: `common --noenable_workspace`, `--incompatible_strict_action_env` (cache-hit + hermeticity), `--sandbox_default_allow_network=false`; add `--remote_download_minimal` once a cache exists. Consider [Aspect bazelrc presets](https://blog.aspect.build/bazelrc-presets).
- 🟡 **buildifier** (Starlark lint/format) in CI — we lint Go/YAML but not BUILD/`.bzl`/`MODULE.bazel`.
- 🟢 **Version-stamp** released binaries (`--stamp` + `workspace_status_command`, `x_defs`) so a consumer's running commit is traceable.
- 🟢 **Concurrency groups** (`cancel-in-progress`) on `test.yaml`/`ci.yaml`; a **flaky-detection** lane (`--runs_per_test=N --runs_per_test_detects_flakes`); periodic `.bazelversion`/rules_go/gazelle/go-github bumps.
- 🟢 **bazelisk-less fallback** — since it's pure Go, document + CI-smoke a `go build`/`go test ./...` path for a fast inner loop and a Bazel-outage safety net.

## Go quality / reliability / observability

- 🔴 **Plumb `context.Context`** through the go-github watch loops; replace fixed `time.Sleep` polls with a context-aware ticker + cancellation/timeouts.
- 🔴 **Rate-limit handling** — branch on `*github.RateLimitError` / `*github.AbuseRateLimitError`, honor `Rate.Reset`/`RetryAfter`; add backoff+jitter (e.g. [go-github-ratelimit](https://github.com/gofri/go-github-ratelimit) RoundTripper). Flat polling will trip secondary limits.
- 🔴 **Audit pagination** — iterate `Response.NextPage`/cursor on all list calls; reading only page 1 is a latent correctness bug (missed PRs/runs).
- 🟡 **`log/slog`** with a GHA-aware handler mapping levels → `::notice/warning/error::` (URL-encoded), JSON/text off-CI — replaces ad-hoc `fmt.Printf` log commands.
- 🟡 **Curate golangci-lint** for this style: + `gosec`, `bodyclose`, `noctx`, `contextcheck` (enforces the ctx work), `errorlint`, `wrapcheck` (the `%w` convention); pin the version + `depguard` (import allowlist).
- 🟡 **`go test -race`** on the shadow/watch packages; **native fuzz** targets for the YAML transforms/parsers with the crash corpus wired into `go test`.
- 🟡 **`--dry-run`** for the privileged shadow commands (preview mutations) + consistent exit codes via one `RunE`→exit mapper.
- 🟢 **Reproducible builds** (`-trimpath`, `-ldflags -buildid=` + version `-X`) surfaced via cobra `--version`; **SBOM** ([syft](https://anchore.com/sbom/)) + **cosign** signing of released binaries.
- 🟢 **OpenTelemetry** spans/metrics on the watch loops (poll latency, API-call count, rate-limit headroom).
- 🟢 Per-package coverage floors (line-only gate at 90 today; Go has no fn/branch coverage).

## Functionality / polish

- 🟡 `lint`/`test` actions are scaffolds (echo inputs) — wire real logic into `internal/actions/{lint,test}`.
- 🟡 **Multi-OS/arch reality check** — only `linux/amd64` proven; verify hermetic-cc `linux/arm64`, macOS/Windows, or document "linux only."

## Test

- 🟡 Run the **live shadow flow end-to-end** in CI (label a PR `shadow-test`) — the one path not yet exercised against the Go/Bazel binaries.
- 🟡 Keep `//shadow/cmd/check-dispatch-auth` + `dispatch-auth-test.yaml` green against the runner.
- 🟢 **Consumer canary matrix** — drive `shadow-consumers.json` as a documented, easily-extended matrix (per-consumer pinned ref); golden-file fixtures for the YAML transforms vs real consumer workflows.

## DX / docs

- 🟡 **`./dev` (or Makefile) entrypoint** wrapping the five `bazelisk`/lint commands (shells to Bazel targets — keeps the no-inline rule); referenced by CONTRIBUTING + CI.
- 🟡 **`CONTRIBUTING.md`** — adding an action, the TDD loop, the commands, the no-inline rule.
- 🟢 **Auto-generate inputs/outputs docs** from each `action.yml` + `shared.yaml` ([action-docs](https://github.com/npalm/action-docs)/auto-doc) with a CI sync check (`setup`'s real inputs/output are undocumented in the README).
- 🟢 **`examples/`** dir with copy-pasteable `uses: …/shared.yaml@v1` (branch/tag/SHA + the empty-`job_workflow_sha` caveat); **`act`** local-test doc + curated `.actrc` (note its limits with `bazelisk run`); issue/PR templates (PR template nudges the `shadow-test` label).

## Cleanup

- Reconcile consumer naming: `shadow-consumers.json` → `stefanpenner-cs/reusable-workflows-consumer`, but the local `consumer/` remote is `stefanpenner/shared-workflow-consumer`.
- Drop now-moot `node_modules` entries from `.bazelignore` / gazelle excludes; sweep docs for Node-era references; tidy stale branches once open PRs land.
