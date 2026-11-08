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
	"sync"
	"testing"
	"time"

	"go.starlark.net/starlark"
)

func TestBudgetSequenceConcat(t *testing.T) {
	for _, tc := range []struct {
		name, source, resource string
		limit                  starlark.BudgetUsage
	}{
		{"string", `"ab"`, "bytes", starlark.BudgetUsage{Bytes: 4}},
		{"bytes", `b"ab"`, "bytes", starlark.BudgetUsage{Bytes: 4}},
		{"list", `[1, 2]`, "elements", starlark.BudgetUsage{Elements: 4}},
		{"tuple", `(1, 2)`, "elements", starlark.BudgetUsage{Elements: 4}},
	} {
		for _, inplace := range []bool{false, true} {
			if inplace && tc.name == "list" { // Mutable extension is explicitly unmetered.
				continue
			}
			t.Run(tc.name+map[bool]string{false: "/plus", true: "/inplace"}[inplace], func(t *testing.T) {
				value, err := starlark.Eval(&starlark.Thread{}, "input.star", tc.source, nil)
				if err != nil {
					t.Fatal(err)
				}
				value.Freeze()
				source := "def join(a, b):\n    return a + b\n"
				if inplace {
					source = "def join(a, b):\n    a += b\n    return a\n"
				}
				thread := &starlark.Thread{Budget: starlark.NewBudget(tc.limit)}
				globals, err := starlark.ExecFile(thread, "concat.star", source, nil)
				if err != nil {
					t.Fatal(err)
				}
				call := func() (starlark.Value, error) {
					return starlark.Call(thread, globals["join"], starlark.Tuple{value, value}, nil)
				}
				got, err := call()
				if err != nil || starlark.Len(got) != 4 {
					t.Fatalf("exact limit: %v, %v", got, err)
				}
				if list, ok := got.(*starlark.List); ok {
					if err := list.Append(starlark.None); err != nil {
						t.Fatalf("concatenation of frozen lists must return mutable list: %v", err)
					}
				}
				before := thread.Budget.Usage()
				_, err = call()
				var budgetErr *starlark.BudgetError
				var evalErr *starlark.EvalError
				if !errors.As(err, &budgetErr) || budgetErr.Resource != tc.resource || budgetErr.Requested != 4 || thread.Budget.Usage() != before {
					t.Fatalf("refused concat: %v, %+v", err, thread.Budget.Usage())
				}
				if !errors.As(err, &evalErr) || !strings.Contains(evalErr.Backtrace(), "concat.star:2") || thread.CallStackDepth() != 0 {
					t.Fatalf("diagnostic/cleanup: %v", err)
				}
				if starlark.Len(value) != 2 {
					t.Fatal("source sequence mutated")
				}
				// A fresh nil budget preserves the original result.
				thread.Budget = nil
				if _, err := call(); err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}

func TestBudgetSequenceConstruction(t *testing.T) {
	for _, tc := range []struct {
		source string
		used   uint64
	}{
		{`[]`, 0}, {`[1, 2, 3]`, 3}, {`[x for x in range(3)]`, 3},
		{`[x for x in range(6) if x % 2 == 0]`, 3},
		{`[[x] for x in range(3)]`, 6},
		{`[x for x in ()]`, 0}, {`[1, 2] + [3]`, 3},
	} {
		t.Run(tc.source, func(t *testing.T) {
			thread := &starlark.Thread{Budget: starlark.NewBudget(starlark.BudgetUsage{Elements: tc.used})}
			v, err := starlark.Eval(thread, "construct.star", tc.source, nil)
			if err != nil || thread.Budget.Usage().Elements != tc.used {
				t.Fatalf("exact limit: %v, %v, %+v", v, err, thread.Budget.Usage())
			}
			if list, ok := v.(*starlark.List); !ok || list.Append(starlark.None) != nil {
				t.Fatal("constructed list must be mutable")
			}
		})
	}
	for _, source := range []string{`[1, 2, 3]`, `[x for x in range(3)]`} {
		thread := &starlark.Thread{Budget: starlark.NewBudget(starlark.BudgetUsage{Elements: 2})}
		v, err := starlark.Eval(thread, "refused.star", source, nil)
		wantUsed := uint64(0)
		if strings.Contains(source, "for") {
			wantUsed = 2
		}
		var budgetErr *starlark.BudgetError
		var evalErr *starlark.EvalError
		if v != nil || !errors.As(err, &budgetErr) || !errors.As(err, &evalErr) || !strings.Contains(evalErr.Backtrace(), "refused.star:1") || thread.Budget.Usage().Elements != wantUsed || thread.CallStackDepth() != 0 {
			t.Fatalf("construction refusal %s: %v, %v, %+v", source, v, err, thread.Budget.Usage())
		}
	}
}

func TestBudgetSequenceComprehensionCleanupAndSideEffects(t *testing.T) {
	items := starlark.NewList([]starlark.Value{starlark.None, starlark.None, starlark.None})
	calls := 0
	tick := starlark.NewBuiltin("tick", func(_ *starlark.Thread, _ *starlark.Builtin, _ starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
		calls++
		return starlark.None, nil
	})
	thread := &starlark.Thread{Budget: starlark.NewBudget(starlark.BudgetUsage{Elements: 2})}
	_, err := starlark.Eval(thread, "cleanup.star", `[tick() for x in items]`, starlark.StringDict{"tick": tick, "items": items})
	var budgetErr *starlark.BudgetError
	if !errors.As(err, &budgetErr) || calls != 3 || thread.Budget.Usage().Elements != 2 || thread.CallStackDepth() != 0 {
		t.Fatalf("comprehension failure order: %v, calls %d, usage %+v", err, calls, thread.Budget.Usage())
	}
	if err := items.Append(starlark.None); err != nil {
		t.Fatalf("failed comprehension retained iterator: %v", err)
	}
	thread.Budget = starlark.NewBudget(starlark.BudgetUsage{Elements: 1})
	_, err = starlark.Eval(thread, "literal-order.star", `[tick(), tick()]`, starlark.StringDict{"tick": tick})
	if !errors.As(err, &budgetErr) || calls != 5 || thread.Budget.Usage().Elements != 0 {
		t.Fatalf("literal must evaluate elements before atomic slot charge: %v, calls %d", err, calls)
	}
}

func TestBudgetSequenceErrorOrderAndOverflow(t *testing.T) {
	frozen := starlark.NewList([]starlark.Value{starlark.None})
	frozen.Freeze()
	for _, source := range []string{`"x" + 1`, `frozen + ()`, "def f():\n    a = frozen\n    a += (1,)\nf()", `[1 // 0, 2]`} {
		thread := &starlark.Thread{Budget: starlark.NewBudget(starlark.BudgetUsage{Elements: 1, Bytes: 1})}
		_, err := starlark.ExecFile(thread, "order.star", source, starlark.StringDict{"frozen": frozen})
		var budgetErr *starlark.BudgetError
		if err == nil || errors.As(err, &budgetErr) || thread.Budget.Usage() != (starlark.BudgetUsage{}) || frozen.Len() != 1 {
			t.Fatalf("precedence %s: %v, %+v", source, err, thread.Budget.Usage())
		}
	}
	for _, tc := range []struct {
		source  string
		initial starlark.BudgetUsage
	}{
		{`s + ""`, starlark.BudgetUsage{Bytes: math.MaxUint64}},
		{`b + b""`, starlark.BudgetUsage{Bytes: math.MaxUint64}},
		{`[1]`, starlark.BudgetUsage{Elements: math.MaxUint64}},
		{`[x for x in (1,)]`, starlark.BudgetUsage{Elements: math.MaxUint64}},
		{`a + a`, starlark.BudgetUsage{Elements: math.MaxUint64}},
	} {
		thread := &starlark.Thread{Budget: starlark.NewBudget(starlark.BudgetUsage{})}
		if err := thread.Budget.Charge(tc.initial); err != nil {
			t.Fatal(err)
		}
		_, err := starlark.Eval(thread, "overflow.star", tc.source, starlark.StringDict{"a": starlark.Tuple{starlark.None}, "s": starlark.String("x"), "b": starlark.Bytes("x")})
		var budgetErr *starlark.BudgetError
		if !errors.As(err, &budgetErr) || thread.Budget.Usage() != tc.initial {
			t.Fatalf("overflow %s: %v, %+v", tc.source, err, thread.Budget.Usage())
		}
	}
	// Zero-length results remain legal when the counter has reached MaxUint64.
	thread := &starlark.Thread{Budget: starlark.NewBudget(starlark.BudgetUsage{})}
	if err := thread.Budget.Charge(starlark.BudgetUsage{Bytes: math.MaxUint64, Elements: math.MaxUint64}); err != nil {
		t.Fatal(err)
	}
	for _, source := range []string{`"" + ""`, `b"" + b""`, `[]`, `[] + []`, `() + ()`, `[x for x in ()]`} {
		if _, err := starlark.Eval(thread, "empty.star", source, nil); err != nil {
			t.Fatalf("empty %s: %v", source, err)
		}
	}
}

func TestBudgetSequenceByteUnitsAndCompilerFolding(t *testing.T) {
	thread := &starlark.Thread{Budget: starlark.NewBudget(starlark.BudgetUsage{Bytes: 6})}
	v, err := starlark.Eval(thread, "unicode.star", `s + s`, starlark.StringDict{"s": starlark.String("雪")})
	if err != nil || v != starlark.String("雪雪") || thread.Budget.Usage().Bytes != 6 {
		t.Fatalf("byte units: %v, %v, %+v", v, err, thread.Budget.Usage())
	}
	thread.Budget = starlark.NewBudget(starlark.BudgetUsage{Bytes: 1, Elements: 3})
	for _, source := range []string{`"ab" + "cd"`, `b"ab" + b"cd"`, `(1, 2) + (3,)`, `[1, 2] + [3]`} {
		if _, err := starlark.Eval(thread, "fold.star", source, nil); err != nil {
			t.Fatalf("compiler folding %s: %v", source, err)
		}
	}
	if thread.Budget.Usage() != (starlark.BudgetUsage{Elements: 3}) {
		t.Fatalf("folded-operation contract: %+v", thread.Budget.Usage())
	}
}

func TestBudgetSequenceIndependentThreads(t *testing.T) {
	globals, err := starlark.ExecFile(&starlark.Thread{}, "parallel.star", "def f():\n    return [x for x in range(3)] + [4]\n", nil)
	if err != nil {
		t.Fatal(err)
	}
	globals.Freeze()
	var wg sync.WaitGroup
	for range 16 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			thread := &starlark.Thread{Budget: starlark.NewBudget(starlark.BudgetUsage{Elements: 8})}
			v, err := starlark.Call(thread, globals["f"], nil, nil)
			if err != nil || starlark.Len(v) != 4 || thread.Budget.Usage().Elements != 8 {
				t.Errorf("independent thread: %v, %v, %+v", v, err, thread.Budget.Usage())
			}
		}()
	}
	wg.Wait()
}

func TestBudgetSequenceRejectsLargeConcat(t *testing.T) {
	if os.Getenv("STARLARK_SEQUENCE_ALLOCATION_CHILD") != "1" {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestBudgetSequenceRejectsLargeConcat$")
		cmd.Env = append(os.Environ(), "STARLARK_SEQUENCE_ALLOCATION_CHILD=1")
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("allocation child: %v\n%s", err, output)
		}
		return
	}
	text := strings.Repeat("x", 8<<20)
	values := make([]starlark.Value, 1<<19)
	for i := range values {
		values[i] = starlark.None
	}
	for _, input := range []starlark.Value{starlark.String(text), starlark.Bytes(text), starlark.NewList(values), starlark.Tuple(values)} {
		thread := &starlark.Thread{Budget: starlark.NewBudget(starlark.BudgetUsage{Bytes: 1, Elements: 1})}
		globals, err := starlark.ExecFile(thread, "large-concat.star", "def f(a):\n    return a + a\n", nil)
		if err != nil {
			t.Fatal(err)
		}
		var before, after runtime.MemStats
		runtime.ReadMemStats(&before)
		_, err = starlark.Call(thread, globals["f"], starlark.Tuple{input}, nil)
		runtime.ReadMemStats(&after)
		var budgetErr *starlark.BudgetError
		if !errors.As(err, &budgetErr) || thread.Budget.Usage() != (starlark.BudgetUsage{}) {
			t.Fatalf("large %s: %v", input.Type(), err)
		}
		t.Logf("refused %s concat allocated %d bytes", input.Type(), after.TotalAlloc-before.TotalAlloc)
		if allocated := after.TotalAlloc - before.TotalAlloc; allocated >= 4<<20 {
			t.Fatalf("refused %s concat allocated %d bytes; charge must precede result backing allocation", input.Type(), allocated)
		}
	}
}
