# ai-fleet

Run Claude Code headlessly in Docker on a copy of your repository — get the
work back as a mergeable git bundle, optionally published as a pull request.

Your working tree is never touched. Each run clones the repository's git
data into a disposable workspace, executes a Claude Code session inside a
container built for your project, and collects whatever was committed. You
review and merge on your own terms.

## How a run works

1. **Preflight** — toolchain, credentials, project registration, and the
   baseline branch are checked before anything is created. A doomed run
   fails in seconds with exit code 2.
2. **Image** — the project image is built (cached by Dockerfile content,
   so an unchanged Dockerfile costs nothing).
3. **Snapshot** — a bare-bones clone (git data only: no untracked files,
   no `.env`, no hooks) is prepared under `.ai-fleet/runs/<run-id>/`.
4. **Session** — the container runs `claude` against your prompt on a
   fresh branch. Its stream-json output is written verbatim to
   `out/log.jsonl`.
5. **Collect** — commits come back as `out/run.bundle`, verified and
   counted. The console prints the exact `git fetch` command to import
   the branch into your repository.
6. **Publish** (optional) — with a repository and credentials configured,
   the branch is pushed and a pull request is opened. The session itself
   writes the PR title and body (`out/pull-request.md`).

## Requirements

- `git`, `docker`, and the `claude` CLI on `PATH` (a running Docker daemon)
- `CLAUDE_CODE_OAUTH_TOKEN` exported — create one with `claude setup-token`

## Install

    go install github.com/altafard/ai-fleet@latest

Check what you have: `ai-fleet --version`.

## Getting started

Initialize your project once — from anywhere inside its git repository:

    cd your-project
    ai-fleet init

This checks your toolchain, asks Claude Code to analyze the project,
generates `.ai-fleet/<project>.ai-fleet.dockerfile` (commit it — and edit
it freely; init adopts an existing one on a fresh clone without
re-analyzing), and prebuilds the container image.

Then deploy an agent:

    ai-fleet deploy unit -p "your task" \
      --model opus --effort high \
      --git-author-name "You" --git-author-email you@example.com

Longer tasks read better from a file: `--prompt-file task.md`. The run
works on `feature/<run-id>` unless you pass `--branch`.

## Configuration

Flags you'd otherwise pass every time can be stored instead:

    ai-fleet config set agent.model opus
    ai-fleet config set agent.effort high
    ai-fleet config get agent.model
    ai-fleet config list

`set` with no value removes the key. Add `--global` to any subcommand to
operate on the global file, `$AI_FLEET_HOME/ai-fleet.ini` (`AI_FLEET_HOME`
defaults to `~/.ai-fleet`), instead of the project's
`.ai-fleet/ai-fleet.ini` (which requires `ai-fleet init` first). Precedence
when `ai-fleet deploy unit` runs is:

    flag > environment > local > global

The schema is closed — unknown keys and invalid values are rejected:

| Key | Notes |
| --- | --- |
| `agent.model` | claude model, alias or full ID |
| `agent.effort` | `low\|medium\|high\|xhigh\|max` |
| `git.provider` | PR mode hosting provider (`github`) |
| `git.repository` | PR mode target repository (`owner/name` or URL) |
| `git.type` | `user\|bot`, default `user` |
| `git.author.name` | git author name for run commits |
| `git.author.email` | git author email for run commits |
| `git.token` | auth token (`user` type only; prefer `AI_FLEET_GIT_TOKEN`) |
| `git.app.id` | GitHub App ID (`bot` type) |
| `git.app.private-key` | path to the App's RSA PEM; relative paths resolve against the repo root (local scope) or `$AI_FLEET_HOME` (global scope) |
| `git.app.installation-id` | GitHub App installation ID; optional, discovered if unset |

The local file is uncommittable by construction (`.ai-fleet/.gitignore` is
an allowlist), so a token stored there never reaches the remote.

## Opening pull requests

PR mode switches on when a provider and repository are present — from
flags or config:

    ai-fleet config set git.provider github
    ai-fleet config set git.repository owner/repo

**As yourself** (`git.type = user`, the default): supply a token via
`AI_FLEET_GIT_TOKEN`, `--git-token`, or `git.token`. **As a bot**
(`git.type = bot`): register a GitHub App with `contents: write` and
`pull_requests: write` permissions, install it on the repository, and
configure:

    ai-fleet config set git.type bot
    ai-fleet config set git.app.id 123456
    ai-fleet config set git.app.private-key .ai-fleet/bot-private-key.pem

A short-lived installation token is minted at publish time — the App's
key is checked offline during preflight, and the installation ID is
discovered automatically (set `git.app.installation-id` to skip the
lookup). Commits then land under the App's identity; set
`git.author.name` / `git.author.email` to match it.

If a pull request for the branch already exists, the push updates it and
the run reports the existing URL — that is success, not a failure.

Credentials never appear in process arguments, console output, error
messages, or `status.json`.

## After a run

Everything lives under `.ai-fleet/runs/<run-id>/`:

| File | Meaning |
| --- | --- |
| `status.json` | run id, baseline, branch, model/effort, image, exit code, commit count, PR URL, timestamps |
| `out/log.jsonl` | the session's stream-json output, verbatim |
| `out/run.bundle` | the commits, as a git bundle |
| `out/pull-request.md` | the session-written PR title and body |
| `prompt.md`, `entrypoint.sh` | exactly what the container was given |

Without PR mode, import the work with the printed command:

    git fetch .ai-fleet/runs/<run-id>/out/run.bundle <branch>:<branch>

A run that produced no commits exits 3 and writes no bundle — that is a
clean outcome, not an error. A failed or interrupted session still
salvages whatever was committed (plus a `wip:` safety commit for any
uncommitted changes) into the bundle, so partial work is never lost; the
run is reported as failed and no PR is opened for it.

## Interrupting

The first `Ctrl-C` (or SIGTERM/SIGHUP) stops the run gracefully: the
container gets a grace period to salvage its work into the bundle. A
second interrupt aborts — the container is removed immediately and the
salvage is abandoned.

## Exit codes

| Code | Meaning |
| --- | --- |
| 0 | success |
| 1 | run or publish failure (salvage may still have produced a bundle) |
| 2 | preflight or usage error — nothing was created |
| 3 | success, but the session made no commits |
| 130 | interrupted |

## Environment variables

| Variable | Meaning |
| --- | --- |
| `CLAUDE_CODE_OAUTH_TOKEN` | Claude Code credential, required (`claude setup-token`) |
| `AI_FLEET_GIT_TOKEN` | PR-mode token for `git.type = user`; beats config, loses to `--git-token` |
| `AI_FLEET_HOME` | global ai-fleet directory, default `~/.ai-fleet` |

## Versioning and releases

Semantic versioning, pre-1.0 (`v0.x`): a minor bump means new features or
breaking changes, a patch bump means fixes only. There is no compatibility
promise until v1.0.0.

A release is just an annotated tag on `main` — no CI, no changelog file
(the commit history is the changelog):

    git tag -a vX.Y.Z -m "vX.Y.Z"
    git push origin vX.Y.Z
