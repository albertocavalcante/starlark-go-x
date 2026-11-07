// Copyright 2026 Alberto Cavalcante. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package starlark_test

import (
	"testing"

	"go.starlark.net/starlark"
)

func TestHookPanicReleasesNestedIterators(t *testing.T) {
	for _, hook := range []string{"instruction", "branch", "iteration", "entry"} {
		t.Run(hook, func(t *testing.T) {
			xs := starlark.NewList([]starlark.Value{starlark.MakeInt(1)})
			ys := starlark.NewList([]starlark.Value{starlark.MakeInt(2)})
			thread := new(starlark.Thread)
			marker := new(int)
			armed, callbacks := false, 0
			stop := func(fn *starlark.Function) {
				if armed && ((hook == "entry" && fn.Name() == "leaf") || (hook != "entry" && fn.Name() == "outer")) {
					callbacks++
					panic(marker)
				}
			}
			switch hook {
			case "instruction":
				thread.OnExec = func(fn *starlark.Function, _ uint32) { stop(fn) }
			case "branch":
				thread.OnBranch = func(fn *starlark.Function, _ uint32, _ bool) { stop(fn) }
			case "iteration":
				thread.OnIteration = func(fn *starlark.Function, _ uint32, _ bool) { stop(fn) }
			case "entry":
				thread.OnFunctionEnter = stop
			}
			predeclared := starlark.StringDict{
				"xs": xs, "ys": ys,
				"arm": starlark.NewBuiltin("arm", func(*starlark.Thread, *starlark.Builtin, starlark.Tuple, []starlark.Tuple) (starlark.Value, error) {
					armed = true
					return starlark.None, nil
				}),
			}
			const src = `def leaf():
    return 1

def outer(xs, ys):
    for x in xs:
        for y in ys:
            arm()
            if x:
                leaf()
outer(xs, ys)
`
			var recovered any
			func() {
				defer func() { recovered = recover() }()
				_, err := starlark.ExecFile(thread, "panic.star", src, predeclared)
				t.Errorf("ExecFile returned %v, want callback panic", err)
			}()
			if recovered != marker || callbacks != 1 {
				t.Fatalf("recovered %v after %d callbacks, want marker after one", recovered, callbacks)
			}
			if depth := thread.CallStackDepth(); depth != 0 {
				t.Errorf("call stack depth = %d, want 0", depth)
			}
			for name, list := range map[string]*starlark.List{"outer": xs, "inner": ys} {
				if err := list.Append(starlark.MakeInt(3)); err != nil {
					t.Errorf("%s iterator retained after panic: %v", name, err)
				}
			}
			thread.OnExec, thread.OnBranch, thread.OnIteration, thread.OnFunctionEnter = nil, nil, nil, nil
			globals, err := starlark.ExecFile(thread, "reuse.star", "answer = 20 + 22", nil)
			if err != nil {
				t.Fatalf("reuse thread: %v", err)
			}
			if got := globals["answer"]; got != starlark.MakeInt(42) {
				t.Errorf("reused thread answer = %v, want 42", got)
			}
		})
	}
}
