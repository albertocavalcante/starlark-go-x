// Copyright 2026 Alberto Cavalcante. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package starlark_test

import (
	"fmt"
	"testing"

	"go.starlark.net/starlark"
	"go.starlark.net/syntax"
)

// CJMP combines an upstream cancellation check with the fork's branch hook.
// A finite while loop catches either side being lost without hanging the test.
func TestBranchHookBackedgeCancellation(t *testing.T) {
	for _, hooks := range []bool{false, true} {
		t.Run(fmt.Sprint(hooks), func(t *testing.T) {
			thread := new(starlark.Thread)
			thread.SetMaxExecutionSteps(100)
			var branches int
			if hooks {
				thread.OnBranch = func(fn *starlark.Function, _ uint32, _ bool) {
					if fn.Name() == "run" {
						branches++
					}
				}
			}
			_, err := starlark.ExecFileOptions(&syntax.FileOptions{While: true}, thread, "backedge.star", `
def run():
    i = 0
    while True:
        i += 1
        if i < 100:
            continue
        break
    return i
run()
`, nil)
			if fmt.Sprint(err) != "Starlark computation cancelled: too many steps" {
				t.Fatalf("execution error = %v, want step-limit cancellation", err)
			}
			if hooks && branches == 0 {
				t.Fatal("branch hook did not observe the loop")
			}
			if depth := thread.CallStackDepth(); depth != 0 {
				t.Fatalf("stack depth after cancellation = %d", depth)
			}
		})
	}
}
