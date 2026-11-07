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
