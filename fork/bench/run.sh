#!/bin/sh
# Copyright 2026 Alberto Cavalcante. All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.
set -eu

# Build once; alternate paired execution order to expose temporal host noise.
output=${1:?new absolute output directory required}
case "$output" in /*) ;; *)
  echo 'output must be absolute' >&2
  exit 2
  ;;
esac
cd "$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)"
[ -z "$(git status --porcelain -- .)" ] || {
  echo 'commit benchmark inputs before measuring' >&2
  exit 1
}
mkdir "$output"
export GOTOOLCHAIN=go1.27.3 GOWORK=off GOMAXPROCS=1 GOFLAGS=-p=2 GOGC=100 GOMEMLIMIT=off GODEBUG=''
unset GOEXPERIMENT
[ "$(go env GOVERSION)" = go1.27.3 ]
{
  git rev-parse HEAD
  git status --porcelain
  uname -a
  date -u
  uptime
  go version
  go env -json GOOS GOARCH GOFLAGS GOEXPERIMENT CGO_ENABLED GOTOOLCHAIN GOAMD64 GOARM64
  printf 'GOGC=100; GOMEMLIMIT=off; GODEBUG=; GOMAXPROCS=1; samples=10; benchtime=200ms; alternating order\n'
  shasum -a 256 bench_test.go hooks_fork_test.go hooks_upstream_test.go go.mod go.sum fork.go.mod fork.go.sum tools/go.mod tools/go.sum run.sh
} >"$output/metadata.txt"

go list -mod=readonly -m -json all >"$output/upstream-modules.json"
go list -mod=readonly -modfile=fork.go.mod -m -json all >"$output/fork-modules.json"
fork_dir=$(go list -mod=readonly -modfile=fork.go.mod -m -f '{{.Dir}}' go.starlark.net)
cp "$fork_dir/fork/manifest.json" "$output/fork-manifest.json"
jq -e '.upstream.commit == "4f4267fb140d1e8b91c9bce55cee2f809e8840dd"' "$output/fork-manifest.json" >/dev/null

go test -mod=readonly -c -o "$output/upstream.test"
go test -mod=readonly -modfile=fork.go.mod -tags=forkhooks -c -o "$output/fork.test"
: >"$output/upstream.txt"
: >"$output/fork.txt"
sample=1
while [ "$sample" -le 10 ]; do
  if [ "$((sample % 2))" -eq 1 ]; then order='upstream fork'; else order='fork upstream'; fi
  for implementation in $order; do
    "$output/$implementation.test" -test.run='^$' -test.bench='.' -test.benchmem -test.benchtime=200ms -test.count=1 -test.cpu=1 >>"$output/$implementation.txt"
  done
  sample=$((sample + 1))
done
# Require every fixed workload/mode rather than accepting a partial result file.
for implementation in upstream fork; do
  awk -v implementation="$implementation" '
    /^Benchmark/ { name=$1; sub(/-[0-9]+$/, "", name); count[name]++ }
    END {
      if (count["BenchmarkParse"] != 10 || count["BenchmarkLoop/nil"] != 10 || count["BenchmarkCalls/nil"] != 10) exit 1
      if (implementation == "fork" && (count["BenchmarkLoop/active"] != 10 || count["BenchmarkCalls/active"] != 10)) exit 1
    }' "$output/$implementation.txt"
done
date -u >>"$output/metadata.txt"
uptime >>"$output/metadata.txt"
go tool -modfile=tools/go.mod benchstat "$output/upstream.txt" "$output/fork.txt" >"$output/benchstat.txt"
rm "$output/upstream.test" "$output/fork.test"
printf 'Results saved in %s; inspect host noise before interpreting differences.\n' "$output"
