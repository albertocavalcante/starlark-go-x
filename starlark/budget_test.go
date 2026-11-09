// Copyright 2026 Alberto Cavalcante. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package starlark_test

import (
	"context"
	"errors"
	"math"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"time"

	"go.starlark.net/starlark"
	"go.starlark.net/syntax"
)

func TestBudgetRepeat(t *testing.T) {
	for _, tc := range []struct {
		name, sequence, resource string
		limit                    starlark.BudgetUsage
	}{
		{"string", `"ab"`, "bytes", starlark.BudgetUsage{Bytes: 6}},
		{"bytes", `b"ab"`, "bytes", starlark.BudgetUsage{Bytes: 6}},
		{"list", `[1, 2]`, "elements", starlark.BudgetUsage{Elements: 6}},
		{"tuple", `(1, 2)`, "elements", starlark.BudgetUsage{Elements: 6}},
	} {
		for _, reverse := range []bool{false, true} {
			t.Run(tc.name+map[bool]string{false: "/right", true: "/left"}[reverse], func(t *testing.T) {
				sequence, err := starlark.Eval(&starlark.Thread{}, "sequence.star", tc.sequence, nil)
				if err != nil {
					t.Fatal(err)
				}
				source := "def repeat(n):\n    return sequence * n\n"
				if reverse {
					source = "def repeat(n):\n    return n * sequence\n"
				}
				thread := &starlark.Thread{Budget: starlark.NewBudget(tc.limit)}
				globals, err := starlark.ExecFile(thread, "repeat.star", source, starlark.StringDict{"sequence": sequence})
				if err != nil {
					t.Fatal(err)
				}
				call := func(n int) (starlark.Value, error) {
					return starlark.Call(thread, globals["repeat"], starlark.Tuple{starlark.MakeInt(n)}, nil)
				}
				value, err := call(3)
				if err != nil || starlark.Len(value) != 6 {
					t.Fatalf("exact limit: value %v, error %v", value, err)
				}
				before := thread.Budget.Usage()
				_, err = call(1)
				var budgetErr *starlark.BudgetError
				if !errors.As(err, &budgetErr) || budgetErr.Resource != tc.resource || budgetErr.Limit != 6 || budgetErr.Used != 6 || budgetErr.Requested != 2 {
					t.Fatalf("cumulative limit: %v (%+v)", err, budgetErr)
				}
				if thread.Budget.Usage() != before || thread.CallStackDepth() != 0 {
					t.Fatal("failed charge changed usage or retained frames")
				}
				var evalErr *starlark.EvalError
				if !errors.As(err, &evalErr) || !strings.Contains(evalErr.Backtrace(), "repeat.star:2") {
					t.Fatalf("missing source diagnostic: %v", err)
				}
				for _, n := range []int{-1, 0} {
					value, err := call(n)
					if err != nil || starlark.Len(value) != 0 || thread.Budget.Usage() != before {
						t.Fatalf("empty repeat %d: %v, %v", n, value, err)
					}
				}
				thread.Budget = starlark.NewBudget(tc.limit)
				if _, err := call(3); err != nil {
					t.Fatalf("new budget: %v", err)
				}
			})
		}
	}
}

func TestBudgetRejectsLargeRepeat(t *testing.T) {
	// Isolate process-wide allocation accounting from other tests. The finite
	// 16 MiB mutation case must fail this check even if it returns BudgetError.
	if os.Getenv("STARLARK_BUDGET_ALLOCATION_CHILD") != "1" {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestBudgetRejectsLargeRepeat$")
		cmd.Env = append(os.Environ(), "STARLARK_BUDGET_ALLOCATION_CHILD=1")
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("allocation child: %v\n%s", err, output)
		}
		return
	}
	thread := &starlark.Thread{Budget: starlark.NewBudget(starlark.BudgetUsage{Bytes: 1024})}
	globals, err := starlark.ExecFile(thread, "large.star", "def repeat(n):\n    return \"x\" * n\n", nil)
	if err != nil {
		t.Fatal(err)
	}
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	_, err = starlark.Call(thread, globals["repeat"], starlark.Tuple{starlark.MakeInt(16777216)}, nil)
	runtime.ReadMemStats(&after)
	var budgetErr *starlark.BudgetError
	if !errors.As(err, &budgetErr) || budgetErr.Requested != 16777216 || thread.Budget.Usage().Bytes != 0 {
		t.Fatalf("large repeat was not refused: %v", err)
	}
	if allocated := after.TotalAlloc - before.TotalAlloc; allocated >= 4<<20 {
		t.Fatalf("refused repetition allocated %d bytes; limit check must precede the 16 MiB allocation", allocated)
	}
}

func TestBudgetUnchargedBoundaries(t *testing.T) {
	// Host values, ordinary builtins, tuple literals and mutable += remain outside
	// this accounting contract, including arbitrary iterable-driven extension.
	items := starlark.NewList(nil)
	thread := &starlark.Thread{Budget: starlark.NewBudget(starlark.BudgetUsage{Bytes: 1, Elements: 1})}
	_, err := starlark.ExecFile(thread, "other.star", `def build(items):
    answer = "".join(("ab", "cd"))
    items += (1, 2, 3)
    items.append(4)
    items.extend((5, 6))
    return list((answer, items))
build(items)
`, starlark.StringDict{"items": items})
	if err != nil || items.Len() != 6 || thread.Budget.Usage() != (starlark.BudgetUsage{}) {
		t.Fatalf("uncharged contract changed: %v, %+v", err, thread.Budget.Usage())
	}
	// The threadless public API preserves its existing unbudgeted behavior.
	for _, op := range []syntax.Token{syntax.STAR, syntax.PLUS} {
		y := starlark.Value(starlark.String("cd"))
		if op == syntax.STAR {
			y = starlark.MakeInt(2)
		}
		if _, err := starlark.Binary(op, starlark.String("ab"), y); err != nil {
			t.Fatalf("Binary: %v", err)
		}
	}
}

func TestBudgetCharge(t *testing.T) {
	var absent *starlark.Budget
	if absent.Charge(starlark.BudgetUsage{Bytes: math.MaxUint64}) != nil || absent.Usage() != (starlark.BudgetUsage{}) {
		t.Fatal("nil budget should be disabled")
	}
	budget := starlark.NewBudget(starlark.BudgetUsage{Work: 3})
	if err := budget.Charge(starlark.BudgetUsage{Bytes: 10, Elements: 20, Work: 3}); err != nil {
		t.Fatal(err)
	}
	before := budget.Usage()
	if err := budget.Charge(starlark.BudgetUsage{Bytes: 1, Elements: 1, Work: 1}); err == nil || budget.Usage() != before {
		t.Fatal("multi-resource refusal must leave every counter unchanged")
	}
	for _, resource := range []string{"bytes", "elements", "work"} {
		t.Run(resource, func(t *testing.T) {
			usage := starlark.BudgetUsage{}
			switch resource {
			case "bytes":
				usage.Bytes = math.MaxUint64
			case "elements":
				usage.Elements = math.MaxUint64
			case "work":
				usage.Work = math.MaxUint64
			}
			b := starlark.NewBudget(starlark.BudgetUsage{})
			if err := b.Charge(usage); err != nil {
				t.Fatal(err)
			}
			var budgetErr *starlark.BudgetError
			if err := b.Charge(usage); !errors.As(err, &budgetErr) || budgetErr.Resource != resource || b.Usage() != usage {
				t.Fatalf("overflow: %v, %+v", err, b.Usage())
			}
		})
	}
}

func TestBudgetHostWork(t *testing.T) {
	workDone := 0
	thread := &starlark.Thread{Budget: starlark.NewBudget(starlark.BudgetUsage{Work: 3})}
	predeclared := starlark.StringDict{
		"host_work": starlark.NewBuiltin("host_work", func(thread *starlark.Thread, _ *starlark.Builtin, _ starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
			for range 10 {
				if err := thread.Budget.Charge(starlark.BudgetUsage{Work: 1}); err != nil {
					return nil, err
				}
				workDone++
			}
			return starlark.None, nil
		}),
	}
	_, err := starlark.ExecFile(thread, "host.star", "host_work()", predeclared)
	var budgetErr *starlark.BudgetError
	if !errors.As(err, &budgetErr) || budgetErr.Resource != "work" || workDone != 3 {
		t.Fatalf("host work after refusal: %d, %v", workDone, err)
	}
}

func FuzzBudgetCharge(f *testing.F) {
	f.Add(uint64(4), uint64(6), uint64(2), uint64(1), uint64(3), uint64(1))
	f.Add(uint64(0), uint64(0), uint64(0), uint64(math.MaxUint64), uint64(1), uint64(0))
	f.Fuzz(func(t *testing.T, bytes, elements, work, addBytes, addElements, addWork uint64) {
		limits := starlark.BudgetUsage{Bytes: bytes, Elements: elements, Work: work}
		amount := starlark.BudgetUsage{Bytes: addBytes, Elements: addElements, Work: addWork}
		b := starlark.NewBudget(limits)
		for range 3 {
			before := b.Usage()
			if err := b.Charge(amount); err != nil {
				if b.Usage() != before {
					t.Fatal("refused charge changed counters")
				}
				continue
			}
			after := b.Usage()
			for _, r := range [][3]uint64{{before.Bytes, after.Bytes, limits.Bytes}, {before.Elements, after.Elements, limits.Elements}, {before.Work, after.Work, limits.Work}} {
				if r[1] < r[0] || (r[2] != 0 && r[1] > r[2]) {
					t.Fatalf("charge overflowed or exceeded a limit: %v", r)
				}
			}
		}
	})
}
