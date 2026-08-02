#!/usr/bin/env bash
set -euo pipefail
exec 2>&1

SRC="${FLEET_SOURCE_DIR:-/source}"
WS="${FLEET_WORKSPACE_DIR:-/workspace}"
OUT="${FLEET_OUT_DIR:-/out}"

# The entrypoint is deliberately silent: claude is the only process writing
# to stdout, and the host streams that verbatim into out/log.jsonl. The run
# outcome is carried by the exit code and the presence of run.bundle.
finish() {
  status=$?
  set +e
  if [ -d "$WS/.git" ]; then
    cd "$WS" || exit "$status"
    if [ -n "$(git status --porcelain)" ]; then
      git add -A >/dev/null 2>&1
      git commit -q -m "wip: uncommitted changes at run end" >/dev/null 2>&1
    fi
    count=$(git rev-list --count "$FLEET_BASELINE_SHA..$FLEET_BRANCH" 2>/dev/null || echo 0)
    if [ "$count" -gt 0 ]; then
      git bundle create "$OUT/run.bundle" "$FLEET_BASELINE_SHA..$FLEET_BRANCH" >/dev/null 2>&1
    fi
  fi
  exit "$status"
}
trap finish EXIT
# docker stop sends SIGTERM only to this process (PID 1). A plain foreground
# child would defer the trap until it exits, so claude is run as a background
# job (`&`) and waited on with the `wait` builtin: bash's documented special
# case for `wait` is that a trapped signal interrupts the wait immediately
# (returning >128) and runs the trap right away, instead of waiting for the
# child to finish. The trap then forwards the signal to claude and exits,
# which lets the EXIT trap (finish) salvage a bundle before Docker's grace
# period elapses and SIGKILLs everything.
trap 'if [ -n "${cpid:-}" ]; then kill -TERM "$cpid" 2>/dev/null; fi; exit 130' TERM INT

git clone -q "$SRC/clone" "$WS" >/dev/null 2>&1
cd "$WS"

git config user.name "$GIT_AUTHOR_NAME"
git config user.email "$GIT_AUTHOR_EMAIL"

git checkout -q -b "$FLEET_BRANCH"

set +e
claude -p "$(cat "$SRC/prompt.md")" --output-format stream-json --verbose --dangerously-skip-permissions &
cpid=$!
wait "$cpid"
cexit=$?
set -e
exit "$cexit"
