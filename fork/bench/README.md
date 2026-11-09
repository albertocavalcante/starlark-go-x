# Reproduce runtime hook and budget measurements

This nested development module compares three immutable runtime revisions on the
same Google upstream base, `4f4267fb140d1e8b91c9bce55cee2f809e8840dd`:

| Runtime | Module input | Evaluation modes |
|---|---|---|
| Google upstream | `go.mod` | nil hooks, nil budget |
| Original fork `29330a31ded85bbc547ac653ffcff934c839b1c2` | `fork.go.mod` | nil/active hooks, nil budget |
| Current fork `909c1c80351e6e4c361f35be2b1b27a804fe3a75` | `current.go.mod` | nil/active hooks crossed with nil/enabled budget |

The checksum files pin sources and the separate tools module pins benchstat.
These development dependencies remain outside consumer dependencies; root
`go test ./...` does not traverse the nested module.

From a committed harness, on a quiet machine with Go, Git, jq and shasum:

```sh
fork/bench/run.sh /absolute/path/to/new-results-directory
```

Use external scratch/cache locations and an appropriate capacity guard. The
runner fixes Go 1.27.3, GOMAXPROCS=1, GOGC=100, GOMEMLIMIT=off and empty GODEBUG.
It records architecture feature levels, exact source/module/build identities,
workload hashes, runtime controls, start/end timestamps and host load. Cold-cache
source downloads and fixture checks finish before timing. Each binary is built
once; all six execution orders run twice, giving twelve samples per case and
balanced positions for all three runtimes. `order.txt` preserves the schedule.

## Workloads and correctness

The original 260-byte parse source is unchanged. `Loop` executes 1,000 branch
iterations and returns 499500; `Calls` executes 1,000 calls and returns 1000.
Both have zero logical budget charges. New fixed synthetic fixtures use a dynamic
`"abcdefgh"` argument to avoid compiler folding:

| Fixture | Result | Enabled budget charged per call |
|---|---|---|
| Concat | `"abcdefghabcdefgh"` | 16 bytes |
| Literal | Four copies of the argument | 4 elements |
| Comprehension | Sixteen copies of the argument | 16 elements |

Enabled budgets use zero limits, meaning unlimited cumulative accounting. A
fresh budget is installed after module initialization and before timing. The
runner checks exact post-timing resource usage, including zero Work, and exact
function/branch/iteration callback counts; instruction callbacks must also fire.
All five hooks are installed in active mode, with zero branch/iteration events
expected for fixtures that contain none. Independent fixture tests validate full
results and finite-limit refusal before timed samples begin.

Parse includes its AST-count check. Evaluation times ordinary warm Call setup
and execution; it excludes module compilation/initialization, configuration,
result comparison and counter validation. The last result and cumulative counters
are checked after timing, and each fixture/mode is checked separately beforehand.
This changes the old harness's timed result-string validation: historical raw
timings must not be mixed with this measurement definition. Active mode measures
all five callbacks together with local counters; it excludes source mapping and
coverage-report processing and does not isolate individual hook costs.

## Matched reports and limits

Raw samples remain in `upstream.txt`, `old.txt` and `current.txt`. Every expected
row must have twelve samples; unknown modes and partial runs fail. Derived mode
files normalize only the selected mode onto matching workload names, and are
validated again before benchstat runs:

- `nil-runtime-comparison.txt`: matching nil-hook/nil-budget cases across all
  three runtimes, including parsing.
- `old-hooks-fixed-nil-comparison.txt`: original fork hook cost with nil budgets.
- `current-hooks-fixed-{nil,enabled}-comparison.txt`: current hook cost at each
  fixed budget mode.
- `current-budget-fixed-{nil,active}-comparison.txt`: current budget cost at each
  fixed hook mode.

Sample validation rejects truncated rows, invalid iteration/metric values and
missing, duplicate or unknown metric units before benchstat. Run
`bash fork/bench/test-samples.sh` for the malformed-input regression fixtures.

Parse appears only in the runtime comparison because it has no Thread modes.
Do not interpret mixed-mode geomeans as application speed. Old-to-current changes
include other runtime work as well as budgets; use within-current toggles when
discussing accounting cost. Busy-host results establish no stable overhead
budget, regression threshold or end-to-end Sky/Bazel claim. Inspect variance and
repeat on a quiet comparable host before interpreting percentages/significance.

Successful runs remove only their generated benchmark binaries; interrupted
outputs remain for inspection. No performance assertion gates ordinary tests.
Raw vet remains a separate obligation under [the VET assessment](../VET.md);
fixture success does not suppress or clear its remaining pointer diagnostic.
