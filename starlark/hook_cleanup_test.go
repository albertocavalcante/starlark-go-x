// Copyright 2026 Alberto Cavalcante. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package starlark_test

import (
	"testing"

	"go.starlark.net/starlark"
)

func TestExitHookPanicCleanup(t *testing.T) {
	for _, test := range []struct {
		name string
		body string
	}{
		{"return", "return x"},
		{"error", `fail("body error")`},
		{"panic", "boom()"},
		{"cancel", "cancel()"},
	} {
		t.Run(test.name, func(t *testing.T) {
			xs := starlark.NewList([]starlark.Value{starlark.MakeInt(1)})
			thread := new(starlark.Thread)
			exitPanic := new(int)
			var exits int
			thread.OnFunctionExit = func(fn *starlark.Function, result starlark.Value) {
				if fn.Name() == "f" {
					exits++
					panic(exitPanic)
				}
			}
			predeclared := starlark.StringDict{
				"xs": xs,
				"boom": starlark.NewBuiltin("boom", func(*starlark.Thread, *starlark.Builtin, starlark.Tuple, []starlark.Tuple) (starlark.Value, error) {
					panic("body panic")
				}),
				"cancel": starlark.NewBuiltin("cancel", func(thread *starlark.Thread, _ *starlark.Builtin, _ starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
					thread.Cancel("body cancellation")
					return starlark.None, nil
				}),
			}
			var recovered interface{}
			func() {
				defer func() { recovered = recover() }()
				src := "def f(xs):\n    for x in xs:\n        " + test.body + "\nf(xs)\n"
				_, err := starlark.ExecFile(thread, "hook.star", src, predeclared)
				t.Errorf("ExecFile returned %v, want exit hook panic", err)
			}()
			if recovered != exitPanic {
				t.Fatalf("recovered %v, want exit hook panic", recovered)
			}
			if exits != 1 {
				t.Errorf("exit hook called %d times, want 1", exits)
			}
			if depth := thread.CallStackDepth(); depth != 0 {
				t.Errorf("call stack depth = %d, want 0", depth)
			}
			if err := xs.Append(starlark.MakeInt(2)); err != nil {
				t.Errorf("append after recovered hook panic: %v", err)
			}
			thread.OnFunctionExit = nil
			thread.Uncancel()
			if _, err := starlark.ExecFile(thread, "reuse.star", "result = 1\n", nil); err != nil {
				t.Errorf("reuse thread: %v", err)
			}
		})
	}
}
