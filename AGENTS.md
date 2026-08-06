# AGENTS.md

Guidance for AI coding agents working in this repository.


## What this is

`ai-fleet` runs a headless Claude Code session in a Docker container on a disposable clone of a git project and brings the resulting commits back as a git bundle, optionally opening a pull request. Go CLI, cobra, no CI — releases are annotated tags on `main` (`git tag -a vX.Y.Z -m "vX.Y.Z"`), semver pre-1.0.

## Commands

```sh
go build .                                   # build the binary
go test ./...                                # full default suite (no docker/claude needed)
go test ./internal/initx/ -run TestName -v   # single test
go test -tags integration ./internal/dockerx/ -v   # docker integration tests (needs a live daemon)
gofmt -l .                                   # must print nothing before committing
go vet ./...
```

## Architecture

`internal/cli` only wires the cobra tree; all behavior lives in the feature packages. Two commands:

- **`ai-fleet init`** → `internal/initx.Execute`. Stages, strictly ordered so nothing touches disk before analysis succeeds: toolchain checks (git/claude/docker versions) → repo-root resolution → already-initialized check (`.ai-fleet/ai-fleet.ini` is the marker, not the folder — deploy runs create `.ai-fleet/runs/` on their own) → inventory analysis (host-side `claude -p` with the embedded `inventory-prompt.md`, strict JSON parsing, fail fast, no fallback/retry/timeout) → write `.ai-fleet/` files (allowlist `.gitignore`, machine-local `ai-fleet.ini`, generated `<name>.ai-fleet.dockerfile` from `dockerfile.tmpl`) → prebuild image + prune stale tags.
- **`ai-fleet deploy unit`** → `internal/run.Execute`. Phases: preflight → image build → run snapshot (`.ai-fleet/runs/<run-id>/` with rendered prompt + entrypoint) → container run → collect → optional PR publish. The container's stdout (claude stream-json) is written verbatim to `out/log.jsonl`.

Cross-cutting packages:

- `internal/execx` — the single doorway to subprocesses. Non-zero exit is reported in `Result`, not as an error; error means the process could not run at all.
- `internal/gitx` / `internal/dockerx` — deliberately shell out to the host CLIs (no go-git, no Docker SDK) so credential helpers, Docker contexts, and user git behavior keep working. Keep it that way.
- `internal/logstream` — console rendering (✓/✗/! lines, in-place spinner) plus parsers for docker build steps and claude stream-json events.
- `internal/runner` — embedded container-side assets: `prompt.md.tmpl` (the headless task briefing), `guidelines.md`, `entrypoint.sh`.
- `internal/provider` — PR publication (`github` in v1).

## Contracts that must not break

- **Exit codes** (`run.Execute`): 0 success, 1 run/publish failure, 2 preflight/usage, 3 success with no commits, 130 interrupted. Entrypoint exit 86 means "commits exist but the bundle could not be written" — it must never be reported as a clean no-change run.
- **Image naming, two schemes**: the generated-Dockerfile path uses `ai-fleet/<name>-<hash>:<content-tag>` and prunes stale tags per project; an explicit `--dockerfile` is the legacy/exceptional path — `ai-fleet:<content-tag>`, never pruned (user intent unknown). `<name>` = sanitized repo-root basename, `<hash>` = first 4 hex of SHA-256 of the absolute root path (machine-local, which is why `ai-fleet.ini` is never committed), content tag = first 12 hex of SHA-256 of Dockerfile bytes.
- **Salvage semantics**: collect still runs after a failed or interrupted container so a bundle salvaged by the entrypoint's trap is verified and reported.
- **Inventory parsing** (`initx`): strict on purpose — unknown JSON fields, trailing content, missing `base_image`, and injection-shaped values are errors. Don't loosen it to "be helpful".

## Conventions

- Comments explain constraints and *why* (see any file header) — not what the next line does. Match this density and tone.
- Tests are table-driven; git-dependent tests use real git in `t.TempDir()` (that's the convention — no git mocking). External effects in `initx` go through package-level `var` seams overridden in tests. Docker-dependent tests live behind `//go:build integration`.
- No new Go module dependencies without a strong reason — the ini parser is hand-rolled deliberately.
