# ai-fleet
Run Claude Code headlessly in Docker on a copy of your repository — get the work back as a mergeable git bundle.

## Install

    go install github.com/altafard/ai-fleet@latest

Check what you have: `ai-fleet --version`.

## Getting started

Initialize your project once — from anywhere inside its git repository:

    cd your-project
    ai-fleet init

This checks your toolchain (git, Claude Code, Docker), asks Claude Code to
analyze the project, generates `.ai-fleet/<project>.ai-fleet.dockerfile`
(commit it — and edit it freely), and prebuilds the container image.

Then deploy an agent:

    ai-fleet deploy unit -p "your task" \
      --model opus --effort high \
      --git-author-name "You" --git-author-email you@example.com

## Configuration

Flags you'd otherwise pass every time can be stored instead:

    ai-fleet config set agent.model opus
    ai-fleet config set agent.effort high
    ai-fleet config get agent.model
    ai-fleet config list

`set` with no value removes the key. Add `--global` to any subcommand to
operate on `~/.ai-fleet/ai-fleet.ini` instead of the project's
`.ai-fleet/ai-fleet.ini` (which requires `ai-fleet init` first). Precedence
when `ai-fleet deploy unit` runs is:

    flag > environment > local > global

| Key | Notes |
| --- | --- |
| `agent.model` | claude model, alias or full ID |
| `agent.effort` | `low\|medium\|high\|xhigh\|max` |
| `git.provider` | PR mode hosting provider (`github`) |
| `git.repository` | PR mode target repository |
| `git.type` | `user\|bot`, default `user` |
| `git.author.name` | git author name for run commits |
| `git.author.email` | git author email for run commits |
| `git.token` | auth token (`user` type only; prefer `AI_FLEET_GIT_TOKEN`) |
| `git.app.id` | GitHub App ID (`bot` type) |
| `git.app.private-key` | path to the App's RSA PEM; relative paths resolve against the repo root (local scope) or `$HOME` (global scope) |
| `git.app.installation-id` | GitHub App installation ID; optional, discovered if unset |

## Versioning and releases

Semantic versioning, pre-1.0 (`v0.x`): a minor bump means new features or
breaking changes, a patch bump means fixes only. There is no compatibility
promise until v1.0.0.

A release is just an annotated tag on `main` — no CI, no changelog file
(the commit history is the changelog):

    git tag -a vX.Y.Z -m "vX.Y.Z"
    git push origin vX.Y.Z
