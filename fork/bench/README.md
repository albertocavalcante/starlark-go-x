# Reproduce hook measurements

This development module compares Google's `4f4267fb140d1e8b91c9bce55cee2f809e8840dd`
with the fork runtime snapshot `29330a31ded85bbc547ac653ffcff934c839b1c2`.
Both contain the same upstream base. The module and checksum files pin both
sources and benchstat. They add no consumer dependencies; root `go test ./...`
does not traverse this nested module.

From a committed harness, on a quiet machine with Go, Git, jq and shasum:

```sh
fork/bench/run.sh /absolute/path/to/new-results-directory
```

Use an appropriate external scratch/cache location and capacity guard. The runner
requires a new output directory, fixes Go1.27.3, one logical CPU, GOGC=100,
GOMEMLIMIT=off and an empty GODEBUG, records architecture feature levels, compiles each
benchmark binary once, then alternates execution order across ten samples.
It saves source/toolchain/environment identity, workload hashes, both module
graphs, the fork manifest, raw samples and pinned benchstat output. Binary outputs
are removed after a successful run; interrupted outputs remain for inspection.

The fixed synthetic workloads measure parsing a 260-byte source, repeated warm
calls executing a 1,000-element branch loop, and a 1,000-call loop. Check the exact
source length in `bench_test.go` when editing it. Parse includes AST validation;
evaluation includes result validation and ordinary call setup, but excludes module
initialization and compilation. Active mode installs all five callbacks together
and increments thread-local counters; it checks that the expected callbacks fired.
Nil and active cases are reported separately. This does not isolate individual
hook costs or include source mapping/coverage-report processing.

Busy-host results are provisional observations, not a stable overhead budget,
regression threshold or end-to-end Sky/Bazel claim. Inspect variance and repeat
on a quiet host before interpreting benchstat's percentages or significance.
No performance assertion gates the ordinary test suite.
