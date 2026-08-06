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
      --git-author-name "You" --git-author-email you@example.com

## Versioning and releases

Semantic versioning, pre-1.0 (`v0.x`): a minor bump means new features or
breaking changes, a patch bump means fixes only. There is no compatibility
promise until v1.0.0.

A release is just an annotated tag on `main` — no CI, no changelog file
(the commit history is the changelog):

    git tag -a vX.Y.Z -m "vX.Y.Z"
    git push origin vX.Y.Z
