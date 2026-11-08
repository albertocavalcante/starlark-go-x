#!/bin/sh
# Copyright 2026 Alberto Cavalcante. All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.
set -eu

# Run from a reviewed checkout. Only this checkout's verifier is executed;
# historical source is inspected as data, with no access to Google.
source_repo=${1:-https://github.com/albertocavalcante/starlark-go-x.git}
requested_revision=${2-}
historical=b2230cee27bcb2233ca2a159be38d14080f0248b
archive_source=https://github.com/albertocavalcante/starlark-go-x.git
case "$source_repo" in
  https://github.com/albertocavalcante/starlark-go-x.git | /*) ;;
  *)
    echo 'source must be the canonical fork or an absolute local path' >&2
    exit 2
    ;;
esac
# Do not inherit object stores, routing, credentials or URL rewrites.
unset GIT_DIR GIT_WORK_TREE GIT_COMMON_DIR GIT_OBJECT_DIRECTORY GIT_ALTERNATE_OBJECT_DIRECTORIES GIT_CONFIG_COUNT GIT_CONFIG_PARAMETERS GIT_CONFIG GIT_CONFIG_SYSTEM GIT_TEMPLATE_DIR GIT_INDEX_FILE GIT_SHALLOW_FILE
export GIT_CONFIG_NOSYSTEM=1 GIT_CONFIG_GLOBAL=/dev/null GIT_TERMINAL_PROMPT=0
export GIT_ALLOW_PROTOCOL=file:https GIT_NO_LAZY_FETCH=1 LC_ALL=C
current=${requested_revision:-$(git rev-parse HEAD)}
case "$current" in
  *[!0-9a-f]* | '')
    echo 'current revision must be a full commit SHA' >&2
    exit 2
    ;;
esac
[ "${#current}" -eq 40 ] || exit 2

recovery_dir=$(mktemp -d "${TMPDIR:-/tmp}/starlark-recovery.XXXXXX")
trap 'rm -rf "$recovery_dir"' EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

# Temporary data may live on a noexec mount. Build only the reviewed verifier
# beside this executable checkout; historical checkouts remain data in TMPDIR.
verifier_dir=$(mktemp -d "$PWD/.starlark-recovery-exec.XXXXXX")
trap 'rm -rf "$recovery_dir" "$verifier_dir"' EXIT
go build -o "$verifier_dir/verify" ./fork
for revision in "$current" "$historical"; do
  checkout="$recovery_dir/$revision"
  git -c core.hooksPath=/dev/null clone --no-local --no-tags --no-checkout -- "$source_repo" "$checkout"
  # CI checkouts may be shallow; fetch the historical published snapshot explicitly.
  if ! git -C "$checkout" cat-file -e "$revision^{commit}"; then
    git -C "$checkout" fetch --no-tags -- "$archive_source" "$revision"
  fi
  git -C "$checkout" -c core.hooksPath=/dev/null checkout --detach "$revision"
  [ ! -e "$checkout/.git/objects/info/alternates" ]
  [ ! -e "$checkout/.git/objects/info/http-alternates" ]
  upstream=$(git -C "$checkout" show "$revision:fork/manifest.json" | jq -er '.upstream.commit | select(test("^[0-9a-f]{40}$"))')
  archive="refs/tags/upstream/$upstream"
  git -C "$checkout" fetch --no-tags -- "$archive_source" "$archive:$archive"
  [ "$(git -C "$checkout" rev-parse "$archive^{commit}")" = "$upstream" ]
  "$verifier_dir/verify" -repo "$checkout" -revision "$revision"

  git -C "$checkout" update-ref -d "$archive"
  if "$verifier_dir/verify" -repo "$checkout" -revision "$revision" >"$recovery_dir/negative.log" 2>&1; then
    echo 'missing archive unexpectedly verified' >&2
    exit 1
  fi
  grep -q 'fetch the recorded archive ref first' "$recovery_dir/negative.log"
  git -C "$checkout" update-ref "$archive" "$revision"
  if "$verifier_dir/verify" -repo "$checkout" -revision "$revision" >"$recovery_dir/negative.log" 2>&1; then
    echo 'moved archive unexpectedly verified' >&2
    exit 1
  fi
  grep -q 'upstream archive identity mismatch' "$recovery_dir/negative.log"
  git -C "$checkout" update-ref "$archive" "$upstream"
  "$verifier_dir/verify" -repo "$checkout" -revision "$revision"
  printf 'Recovered %s; missing/moved archive negatives rejected.\n' "$revision"
  rm -rf "$checkout"
done
