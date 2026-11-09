#!/bin/sh
# Copyright 2026 Alberto Cavalcante. All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.
set -eu

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
  printf 'GOGC=100; GOMEMLIMIT=off; GODEBUG=; GOMAXPROCS=1; samples=12; benchtime=200ms; six execution orders repeated twice\n'
  shasum -a 256 ./*_test.go go.mod go.sum fork.go.mod fork.go.sum current.go.mod current.go.sum tools/go.mod tools/go.sum run.sh
} >"$output/metadata.txt"

# All source pins are downloaded before inspecting .Dir, including cold caches.
for implementation in upstream old current; do
  case "$implementation" in
  upstream)
    modfile=go.mod
    tags=''
    ;;
  old)
    modfile=fork.go.mod
    tags=forkhooks
    ;;
  current)
    modfile=current.go.mod
    tags=forkhooks,forkbudget
    ;;
  esac
  go mod download -modfile="$modfile" -json all >"$output/$implementation-download.json"
  go list -mod=readonly -modfile="$modfile" -m -json all >"$output/$implementation-modules.json"
  if [ "$implementation" != upstream ]; then
    source_dir=$(go list -mod=readonly -modfile="$modfile" -m -f '{{.Dir}}' go.starlark.net)
    [ -n "$source_dir" ] || {
      echo "pinned $implementation source was not downloaded" >&2
      exit 1
    }
    cp "$source_dir/fork/manifest.json" "$output/$implementation-manifest.json"
    jq -e '.upstream.commit == "4f4267fb140d1e8b91c9bce55cee2f809e8840dd"' "$output/$implementation-manifest.json" >/dev/null
  fi
  go test -mod=readonly -modfile="$modfile" -tags="$tags" -c -o "$output/$implementation.test"
  go version -m "$output/$implementation.test" >"$output/$implementation-build.txt"
  "$output/$implementation.test" -test.run='^Test(Fixtures|BudgetLimits)$' -test.bench='^$' -test.v >"$output/$implementation-fixtures.txt"
  : >"$output/$implementation.txt"
done

# All six permutations run twice, balancing each implementation's position.
sample=0
while [ "$sample" -lt 12 ]; do
  case "$((sample % 6))" in
  0) order='upstream old current' ;;
  1) order='current old upstream' ;;
  2) order='old upstream current' ;;
  3) order='current upstream old' ;;
  4) order='old current upstream' ;;
  5) order='upstream current old' ;;
  esac
  printf '%s: %s\n' "$((sample + 1))" "$order" >>"$output/order.txt"
  for implementation in $order; do
    "$output/$implementation.test" -test.run='^$' -test.bench='.' -test.benchmem -test.benchtime=200ms -test.count=1 -test.cpu=1 >>"$output/$implementation.txt"
  done
  sample=$((sample + 1))
done

# Require all expected rows, reject unknown modes, and reject partial samples.
check_samples() {
  awk 'FNR == NR { expected[$0] = 1; next }
    /^Benchmark/ {
      name = $1; sub(/-[0-9]+$/, "", name)
      if (!(name in expected)) { print "unexpected benchmark: " name > "/dev/stderr"; bad = 1 }
      count[name]++
    }
    END {
      for (name in expected) if (count[name] != 12) { print "incomplete benchmark: " name > "/dev/stderr"; bad = 1 }
      exit bad
    }' "$1" "$2"
}
for implementation in upstream old current; do
  hooks='nil active'
  budgets='nil'
  [ "$implementation" != upstream ] || hooks='nil'
  [ "$implementation" != current ] || budgets='nil enabled'
  {
    printf 'BenchmarkParse\n'
    for workload in Loop Calls Concat Literal Comprehension; do
      for hook in $hooks; do
        for budget in $budgets; do
          printf 'Benchmark%s/hooks=%s/budget=%s\n' "$workload" "$hook" "$budget"
        done
      done
    done
  } >"$output/$implementation-expected.txt"
  check_samples "$output/$implementation-expected.txt" "$output/$implementation.txt"
done

# Project one exact mode onto common workload names for matched comparisons.
# Parse has no thread modes and appears only in the three-runtime comparison.
select_mode() {
  awk -v hooks="$2" -v budget="$3" -v parse="$4" '
    /^Benchmark/ {
      if (parse && $1 ~ /^BenchmarkParse(-[0-9]+)?$/) { print; next }
      if ($1 ~ "/hooks=" hooks "/budget=" budget "(-[0-9]+)?$") {
        sub("/hooks=" hooks "/budget=" budget, "", $1); print
      }
      next
    }
    /^(goos|goarch|pkg|cpu):/ { print }' "$1"
}
for workload in Loop Calls Concat Literal Comprehension; do
  printf 'Benchmark%s\n' "$workload"
done >"$output/mode-expected.txt"
{
  printf 'BenchmarkParse\n'
  cat "$output/mode-expected.txt"
} >"$output/common-expected.txt"
for implementation in upstream old current; do
  select_mode "$output/$implementation.txt" nil nil 1 >"$output/$implementation-common.txt"
  check_samples "$output/common-expected.txt" "$output/$implementation-common.txt"
done
go tool -modfile=tools/go.mod benchstat "$output/upstream-common.txt" "$output/old-common.txt" "$output/current-common.txt" >"$output/nil-runtime-comparison.txt"

compare_modes() {
  implementation=$1
  axis=$2
  fixed=$3
  case "$axis" in
  hooks)
    left=nil
    right=active
    lh=nil
    rh=active
    lb=$fixed
    rb=$fixed
    ;;
  budget)
    left=nil
    right=enabled
    lh=$fixed
    rh=$fixed
    lb=nil
    rb=enabled
    ;;
  esac
  prefix="$output/$implementation-$axis-fixed-$fixed"
  select_mode "$output/$implementation.txt" "$lh" "$lb" 0 >"$prefix-$left.txt"
  select_mode "$output/$implementation.txt" "$rh" "$rb" 0 >"$prefix-$right.txt"
  check_samples "$output/mode-expected.txt" "$prefix-$left.txt"
  check_samples "$output/mode-expected.txt" "$prefix-$right.txt"
  go tool -modfile=tools/go.mod benchstat "$prefix-$left.txt" "$prefix-$right.txt" >"$prefix-comparison.txt"
}
compare_modes old hooks nil
compare_modes current hooks nil
compare_modes current hooks enabled
compare_modes current budget nil
compare_modes current budget active
date -u >>"$output/metadata.txt"
uptime >>"$output/metadata.txt"
rm "$output/upstream.test" "$output/old.test" "$output/current.test"
printf 'Results saved in %s; inspect host noise before interpreting differences.\n' "$output"
