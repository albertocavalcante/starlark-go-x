// Copyright 2026 Alberto Cavalcante. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package starlark

import (
	"errors"
	"runtime"
	"strings"
	"testing"

	"go.starlark.net/internal/compile"
)

func TestBudgetSequenceComprehensionAppendAtomic(t *testing.T) {
	// Exercise both growth and reused-capacity appends. A refused charge must
	// preserve the complete prefix and backing slice, not just its length.
	for _, capacity := range []int{1, 4} {
		elems := make([]Value, 1, capacity)
		elems[0] = None
		list := NewList(elems)
		budget := NewBudget(BudgetUsage{Elements: 1})
		if err := budget.Charge(BudgetUsage{Elements: 1}); err != nil {
			t.Fatal(err)
		}
		before := list.elems
		err := appendToComprehension(budget, list, True)
		var budgetErr *BudgetError
		if !errors.As(err, &budgetErr) || len(list.elems) != 1 || cap(list.elems) != capacity || &list.elems[0] != &before[0] || list.elems[0] != None || budget.Usage().Elements != 1 {
			t.Fatalf("refused append mutated list: %v, %v", list, err)
		}
		if capacity > 1 && list.elems[:capacity][1] != nil {
			t.Fatal("refused append wrote spare capacity")
		}
	}
}

func TestBudgetSequenceLiteralBeforeAllocation(t *testing.T) {
	// Frame/operand allocation and parsing are intentionally outside this guard.
	// Start measuring at the MAKELIST opcode, after those allocations. A mutation
	// moving charge below make would allocate a 2 MiB result and fail the test.
	const n = 1 << 17
	source := "def f():\n    return [" + strings.Repeat("None,", n) + "]\n"
	thread := &Thread{Budget: NewBudget(BudgetUsage{Elements: 1})}
	globals, err := ExecFile(thread, "large-list.star", source, nil)
	if err != nil {
		t.Fatal(err)
	}
	var before, after runtime.MemStats
	seen := false
	thread.OnExec = func(fn *Function, pc uint32) {
		if compile.Opcode(fn.funcode.Code[pc]) == compile.MAKELIST {
			seen = true
			runtime.ReadMemStats(&before)
		}
	}
	_, err = Call(thread, globals["f"], nil, nil)
	runtime.ReadMemStats(&after)
	var budgetErr *BudgetError
	if !seen || !errors.As(err, &budgetErr) || budgetErr.Requested != n || thread.Budget.Usage().Elements != 0 {
		t.Fatalf("literal was not refused at construction: %v", err)
	}
	t.Logf("refused literal allocated %d bytes after MAKELIST", after.TotalAlloc-before.TotalAlloc)
	if allocated := after.TotalAlloc - before.TotalAlloc; allocated >= 512<<10 {
		t.Fatalf("refused literal allocated %d bytes after MAKELIST; charge must precede backing allocation", allocated)
	}
}
