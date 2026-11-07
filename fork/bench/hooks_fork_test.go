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
func configureHooks(thread *starlark.Thread, active, branching bool) func() error {
	var execs, branches, enters, exits, iterations uint64
	if active {
		thread.OnExec = func(*starlark.Function, uint32) { execs++ }
		thread.OnBranch = func(*starlark.Function, uint32, bool) { branches++ }
		thread.OnFunctionEnter = func(*starlark.Function) { enters++ }
		thread.OnFunctionExit = func(*starlark.Function, starlark.Value) { exits++ }
		thread.OnIteration = func(*starlark.Function, uint32, bool) { iterations++ }
	}
	return func() error {
		if active && (execs == 0 || enters == 0 || exits == 0 || iterations == 0 || (branching && branches == 0)) {
			return fmt.Errorf("inactive hook fixture: exec=%d branch=%d enter=%d exit=%d iteration=%d", execs, branches, enters, exits, iterations)
		}
		return nil
	}
}
