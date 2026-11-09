#!/usr/bin/env bash
# Copyright 2026 Alberto Cavalcante. See LICENSE for licensing terms.
set -euo pipefail
script_dir=$(cd -- "$(dirname -- "$0")" && pwd)
checker=${CHECK_SAMPLES:-$script_dir/check-samples.awk}
fixture_dir=$(mktemp -d)
trap 'rm -rf "$fixture_dir"' EXIT
printf '%s\n' BenchmarkParse BenchmarkLoop/hooks=nil/budget=nil >"$fixture_dir/expected"
for ((i = 0; i < 12; i++)); do
  printf '%s\n' 'BenchmarkParse-1 10 100 ns/op 2.60 MB/s 5200 B/op 119 allocs/op'
  printf '%s\n' 'BenchmarkLoop/hooks=nil/budget=nil-1 20 400 ns/op 192 B/op 5 allocs/op'
done >"$fixture_dir/valid"
awk -f "$checker" "$fixture_dir/expected" "$fixture_dir/valid"
reject() {
  if awk -f "$checker" "$fixture_dir/expected" "$fixture_dir/invalid" >"$fixture_dir/output" 2>&1; then
    echo "invalid samples accepted: $1" >&2
    exit 1
  fi
}
sed '$s/ .*//' "$fixture_dir/valid" >"$fixture_dir/invalid"
reject truncated-twelfth-row
for field in iterations duration unit duplicate missing; do
  awk -v change="$field" '
    NR == 2 {
      if (change == "iterations") $2 = 0
      if (change == "duration") $3 = "NaN"
      if (change == "unit") $4 = "us/op"
      if (change == "duplicate") $6 = "ns/op"
      if (change == "missing") NF = 4
    }
    { print }
  ' "$fixture_dir/valid" >"$fixture_dir/invalid"
  reject "$field"
done
sed '$d' "$fixture_dir/valid" >"$fixture_dir/invalid"
reject missing-sample
cat "$fixture_dir/valid" "$fixture_dir/valid" >"$fixture_dir/invalid"
reject extra-samples
sed 's/BenchmarkParse/BenchmarkUnknown/' "$fixture_dir/valid" >"$fixture_dir/invalid"
reject unknown-benchmark
: >"$fixture_dir/empty"
if awk -f "$checker" "$fixture_dir/empty" "$fixture_dir/valid" >/dev/null 2>&1; then
  echo 'empty expected sample set accepted' >&2
  exit 1
fi
echo 'Benchmark sample validation fixtures pass.'
