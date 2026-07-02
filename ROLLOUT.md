# Fleet rollout

How to ship a change to this reusable workflow across thousands of consumer repos
with confidence: **release → ramp → watch → promote or roll back.**

The control plane is one file — [`.github/rollout.json`](.github/rollout.json) —
edited only through `rolloutctl`, validated in CI, and read at runtime by every
consumer. No consumer repo changes to move the fleet.

---

## The model

- **Channel** — a named release track (`stable`, `canary`). A consumer picks a
  channel once (`channel: stable`) and never touches it again.
- **ref** — the provider version a consumer actually runs.
- **Ramp** — a candidate ref staged to a *percentage* of a channel's repos.
- **Bucket** — each repo hashes to a stable `0..99`. A repo's bucket never
  changes, so raising the ramp percentage only ever *adds* repos — cohorts grow,
  they never reshuffle.
- **Override** — pin one repo to a specific ref (escape hatch).

Resolution order for a repo on a channel:

```
explicit ref (consumer-set)  →  override  →  ramp cohort  →  channel ref
```

### How a consumer opts in

```yaml
jobs:
  ci:
    uses: stefanpenner-cs/reusable-workflows/.github/workflows/shared.yaml@v1
    with:
      channel: stable          # follow the fleet
      project-name: my-app
```

`shared.yaml` does a **two-phase checkout**:

1. check out the control plane at `bootstrap-ref` (default `main`) →
   run the resolver against `rollout.json` → get this repo's ref.
2. check out the actions at the **resolved** ref → run them.

So the manifest on `main` steers the fleet; the ref it resolves to is the pinned
thing that runs.

---

## Runbook

All commands edit `.github/rollout.json` in place. Commit the change — that commit
*is* the fleet action. Consumers pick it up on their next run.

### 1. Release a new version

Tag it, shadow-test it (`shadow-test` label on the PR — see
[`shadow/README.md`](shadow/README.md)), merge.

### 2. Start a ramp

```sh
rolloutctl ramp --channel stable --ref v1.4.0 --percent 5
```

Preview who moves **before** committing:

```sh
rolloutctl plan --channel stable --repos-file consumers.json
#   v1.3.0   950  (95.0%)
#   v1.4.0    50  ( 5.0%)
```

`consumers.json` is a JSON array of `"owner/name"` strings — your fleet (or a
sample).

### 3. Watch, then widen

Watch the 5% cohort's CI. Healthy? Raise it:

```sh
rolloutctl ramp --channel stable --ref v1.4.0 --percent 25
rolloutctl ramp --channel stable --ref v1.4.0 --percent 50
```

Because buckets are stable, the 5% cohort stays inside the 25% cohort stays inside
the 50%. No repo flip-flops.

### 4. Promote (100%)

```sh
rolloutctl promote --channel stable
```

The ramp ref becomes the channel ref; the old ref is kept as `previous` for
rollback.

### 5. Roll back — the kill switch

```sh
rolloutctl rollback --channel stable
```

- **During a ramp** → aborts it. Everyone snaps back to the channel ref. Instant
  kill switch for a bad candidate.
- **After a promote** → swaps `ref` ↔ `previous`. Rolling back twice rolls
  forward again (no lost state).

### 6. Pin one repo

Edit `rollout.json`'s `overrides`, or a consumer sets `ref:` directly to bypass
the channel entirely.

---

## Why this is safe to iterate on

- **One control point.** Fleet state is one validated file, not N repo edits.
- **Deterministic cohorts.** Same manifest + same repo → same ref, always
  (`hash/fnv`, no randomness). A ramp is reproducible and monotonic.
- **Loud failure.** Unknown channel, missing manifest, bad percent → the resolve
  step *fails* the consumer run. It never silently drifts to `main`.
- **Shadow-tested before merge.** Changes run against real consumers under a real
  `pull_request` event first.
- **Manifest is CI-gated.** `rolloutctl validate` rejects a manifest that isn't
  canonically formatted or is structurally invalid.

---

## Fleet-scale notes

- **Shadow matrix is a sample, not the fleet.** GitHub caps a matrix at 256 jobs;
  `list-consumers` fails loudly past that. Keep `shadow-consumers.json` a
  representative set (a few real repos per language/shape), not every repo.
- **Rate limits.** The shadow watch loops back off on `RateLimitError` /
  secondary limits and 5xx, and every API call has a 30s timeout — a few dozen
  concurrent shadow runs won't false-fail on a 403. Onboarding *thousands* of
  live consumers is fine (each runs its own CI on its own runner); it's the
  *shadow* path that's sampled.
- **Onboarding thousands of repos.** Add `channel: stable` to each repo's caller
  once (a scripted PR). After that, every rollout is a manifest commit — zero
  per-repo churn.
- **Standing secret.** The shadow path still uses `SHADOW_PAT`; migrating it to
  OIDC is the top open item in [`TODO.md`](TODO.md). The rollout control plane
  itself needs no secret — it's a file read on the consumer's own runner.
