# Using starlark-go-x

The fork keeps the `go.starlark.net` module path so existing imports continue to
work. Select it in your application's main module with an immutable replacement.
A library's replacement directive does not select the fork for its consumers;
the consuming main module must make that choice.

## Run a hook example in a fresh module

Use Go 1.25 or newer. This revision is the reviewed upstream import; later
maintenance-only commits do not change its runtime behavior.

```sh
mkdir hook-example
cd hook-example
go mod init example.com/hook-example
go mod edit -replace=go.starlark.net=github.com/albertocavalcante/starlark-go-x@v0.0.0-20261107042442-29330a31ded8
```

Save this as `main.go`:

```go
package main

import (
	"fmt"
	"log"

	"go.starlark.net/starlark"
)

func main() {
	calls := 0
	thread := &starlark.Thread{
		Name: "example",
		OnExec: func(fn *starlark.Function, pc uint32) {
			calls++
		},
	}
	globals, err := starlark.ExecFile(thread, "example.star", "answer = 40 + 2", nil)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("answer =", globals["answer"])
	fmt.Println("instruction hook observed =", calls > 0)
}
```

Fetch the dependency and run without local workspace overrides:

```sh
GOWORK=off go mod tidy
GOWORK=off go run .
```

Expected output:

```text
answer = 42
instruction hook observed = true
```

Select a newer reviewed revision by changing the replacement, reviewing module
changes, and rerunning your application's checks.

## Instrumentation boundary

| API | Event |
|---|---|
| `Thread.OnExec` | Before an executed bytecode instruction |
| `Thread.OnBranch` | After a completed conditional-jump decision; `taken` identifies its outcome |
| `Thread.OnFunctionEnter` | When a Starlark function reaches its entry hook |
| `Thread.OnFunctionExit` | When a function unwinds through its installed exit hook; result is nil on evaluation failure |
| `Thread.OnIteration` | After a for-loop iteration decision; `continued` includes the exhausted-loop outcome |
| `Function.PositionAt(pc)` | Source position associated with an instruction site |

Callbacks run synchronously on the executing goroutine. Keep them observational
and inexpensive. Callback reentrancy has not been validated as a supported API.
An argument or entry-cancellation failure can occur before the entry/exit hooks
are installed, so these hooks do not count every attempted call.

An exit-hook panic propagates to the embedding host while the interpreter's
iterator and frame cleanup still runs; this path has a dedicated regression.
Other callback lifecycle cases should be judged by their specific tests rather
than assuming a blanket panic/reentrancy guarantee.

Program counters identify sites within a compiled function, not stable IDs across
compiler revisions. Source positions do not enumerate all executable lines or
possible branches. Observed events alone do not establish a complete coverage
denominator. No zero-overhead claim is made for unset hooks.

## Logical resource budget

This API requires a revision whose manifest includes `resource-budget`. The
historical hook-example pin above predates it. For a candidate PR, select its
recorded immutable head revision in the consuming application's replacement;
rerun the consumer checks before adopting it. Published API availability is
established by the selected revision, not by reading newer documentation.

Attach an optional budget before evaluation:

```go
thread := &starlark.Thread{
    Budget: starlark.NewBudget(starlark.BudgetUsage{
        Bytes: 1 << 20, Elements: 10000, Work: 10000,
    }),
}
```

The interpreter charges the complete output length **before** string/bytes or
list/tuple repetition by an integer, in either operand order. Bytes and elements
are separate units. Charges are cumulative across calls/modules using the same
budget, and are not refunded when objects become unreachable. Even a repetition
that reuses storage can incur its logical output charge. Zero limits mean
unlimited; counter overflow is always refused. A nil budget disables accounting.

Cooperating Go builtins can call `thread.Budget.Charge` before work or allocation.
For example, charge `BudgetUsage{Work: 1}` at each bounded iteration. Propagate a
charge error before performing that work. This API cannot interrupt a blocked or
uncooperative Go function; timeouts/cancellation still require host cooperation.

Use `Usage()` for successfully charged counters. A refused multi-resource charge
changes no counters. Errors retain a typed `*starlark.BudgetError` through the
source-mapped `*starlark.EvalError`; inspect it with `errors.As`. A budget is not
safe for concurrent access and must not be shared by executing threads. Reuse
preserves accumulated charges; install a new budget to reset them.

**Initial coverage is deliberately narrow.** Other operators (including
concatenation), literals, comprehensions, collection growth, parsing/compilation,
ordinary builtins, custom values and the threadless `starlark.Binary` API are not
automatically charged. Work is host-defined; byte charges are not actual Go
allocations, retained heap or RSS. Budget success does not qualify an untrusted
program as memory-bounded. Keep input/output/concurrency bounds and isolated
worker controls appropriate to the host's trust model.

## Annotation boundary

The default `syntax.FileOptions` rejects annotation syntax. With
`Types: syntax.TypesParseOnly`, parsing and AST inspection accept annotations;
ordinary unannotated code remains usable. Annotated programs cannot be resolved,
compiled or evaluated. Those entry points return ordinary errors, including when
the caller supplies a pre-parsed AST.

`syntax.TypesEnabled` is reserved. Resolution and evaluation reject that mode;
there is no static typechecker or supported typed execution.

## Maintenance and ownership

[The runbook](README.md) explains source reconstruction, upstream replay, rollback
and branch cleanup. A successful reconstruction proves the committed source is
explainable; the relevant tests and consumer checks establish behavior.

Upstream notices and LICENSE apply to upstream work. Alberto Cavalcante's fork
additions retain his copyright under the same BSD-style license. See
[FORK_NOTICE](../FORK_NOTICE).
