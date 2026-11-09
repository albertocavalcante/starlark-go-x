//go:build forkhooks

// Copyright 2026 Alberto Cavalcante. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bench

import (
	"fmt"

	"go.starlark.net/starlark"
)

func hookModes() []bool { return []bool{false, true} }
func configureHooks(thread *starlark.Thread, active bool, f fixture) func(uint64) error {
	var execs, branches, enters, exits, iterations uint64
	if active {
		thread.OnExec = func(*starlark.Function, uint32) { execs++ }
		thread.OnBranch = func(*starlark.Function, uint32, bool) { branches++ }
		thread.OnFunctionEnter = func(*starlark.Function) { enters++ }
		thread.OnFunctionExit = func(*starlark.Function, starlark.Value) { exits++ }
		thread.OnIteration = func(*starlark.Function, uint32, bool) { iterations++ }
	}
	return func(calls uint64) error {
		if active && (execs == 0 || enters != f.calls*calls || exits != enters || iterations != f.iterations*calls || branches != f.branches*calls) {
			return fmt.Errorf("hook fixture %s: exec=%d branch=%d enter=%d exit=%d iteration=%d after %d calls", f.name, execs, branches, enters, exits, iterations, calls)
		}
		return nil
	}
}
