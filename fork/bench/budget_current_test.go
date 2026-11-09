//go:build forkbudget

// Copyright 2026 Alberto Cavalcante. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bench

import (
	"errors"
	"fmt"
	"testing"

	"go.starlark.net/starlark"
)

func budgetModes() []bool { return []bool{false, true} }
func configureBudget(thread *starlark.Thread, enabled bool, perCall usage) func(uint64) error {
	if enabled {
		thread.Budget = starlark.NewBudget(starlark.BudgetUsage{})
	}
	return func(calls uint64) error {
		want := starlark.BudgetUsage{}
		if enabled {
			want.Bytes = perCall.bytes * calls
			want.Elements = perCall.elements * calls
		}
		if got := thread.Budget.Usage(); got != want {
			return fmt.Errorf("budget usage = %+v; want %+v after %d calls", got, want, calls)
		}
		return nil
	}
}

// Finite limits verify the accounting path independently of timed unlimited runs.
func TestBudgetLimits(t *testing.T) {
	for _, f := range fixtures() {
		if f.charge == (usage{}) {
			continue
		}
		t.Run(f.name, func(t *testing.T) {
			r := prepare(t, f, false, false)
			limit := starlark.BudgetUsage{Bytes: f.charge.bytes, Elements: f.charge.elements}
			r.thread.Budget = starlark.NewBudget(limit)
			if _, err := starlark.Call(r.thread, r.function, f.args, nil); err != nil {
				t.Fatal(err)
			}
			if got := r.thread.Budget.Usage(); got != limit {
				t.Fatalf("exact limit usage = %+v; want %+v", got, limit)
			}
			_, err := starlark.Call(r.thread, r.function, f.args, nil)
			var refused *starlark.BudgetError
			if !errors.As(err, &refused) || r.thread.Budget.Usage() != limit || r.thread.CallStackDepth() != 0 {
				t.Fatalf("finite limit did not refuse safely: error %v, usage %+v", err, r.thread.Budget.Usage())
			}
		})
	}
}
