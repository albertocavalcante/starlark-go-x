// Copyright 2026 Alberto Cavalcante. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bench

import (
	"testing"

	"go.starlark.net/starlark"
	"go.starlark.net/syntax"
)

// Fixed synthetic workloads: no file discovery, host builtins or IO in timing.
const program = `def loop(n):
    total = 0
    for i in range(n):
        if i % 2 == 0:
            total += i
        else:
            total += i
    return total

def add(x):
    return x + 1

def calls(n):
    x = 0
    for i in range(n):
        x = add(x)
    return x
`

// Arguments keep concatenation dynamic, rather than compiler-folded constants.
const sequenceProgram = `def concat(x):
    return x + x

def literal(x):
    return [x, x, x, x]

def comprehension(x):
    return [x for i in range(16)]
`

type usage struct{ bytes, elements uint64 }

type fixture struct {
	name, source, function      string
	args                        starlark.Tuple
	want                        starlark.Value
	charge                      usage
	calls, branches, iterations uint64
}

func fixtures() []fixture {
	x := starlark.String("abcdefgh")
	repeated := func(n int) *starlark.List {
		values := make([]starlark.Value, n)
		for i := range values {
			values[i] = x
		}
		return starlark.NewList(values)
	}
	return []fixture{
		{"Loop", program, "loop", starlark.Tuple{starlark.MakeInt(1000)}, starlark.MakeInt(499500), usage{}, 1, 1000, 1001},
		{"Calls", program, "calls", starlark.Tuple{starlark.MakeInt(1000)}, starlark.MakeInt(1000), usage{}, 1001, 0, 1001},
		{"Concat", sequenceProgram, "concat", starlark.Tuple{x}, starlark.String("abcdefghabcdefgh"), usage{bytes: 16}, 1, 0, 0},
		{"Literal", sequenceProgram, "literal", starlark.Tuple{x}, repeated(4), usage{elements: 4}, 1, 0, 0},
		{"Comprehension", sequenceProgram, "comprehension", starlark.Tuple{x}, repeated(16), usage{elements: 16}, 1, 0, 17},
	}
}

func modeName(hooks, budget bool) string {
	h, b := "nil", "nil"
	if hooks {
		h = "active"
	}
	if budget {
		b = "enabled"
	}
	return "hooks=" + h + "/budget=" + b
}

type run struct {
	thread                  *starlark.Thread
	function                starlark.Value
	checkHooks, checkBudget func(uint64) error
}

func prepare(t testing.TB, f fixture, hooks, budget bool) run {
	t.Helper()
	thread := new(starlark.Thread)
	globals, err := starlark.ExecFile(thread, "bench.star", f.source, nil)
	if err != nil {
		t.Fatal(err)
	}
	return run{
		thread: thread, function: globals[f.function],
		checkHooks: configureHooks(thread, hooks, f), checkBudget: configureBudget(thread, budget, f.charge),
	}
}

func (r run) validate(t testing.TB, f fixture, value starlark.Value, calls uint64) {
	t.Helper()
	equal, err := starlark.Equal(value, f.want)
	if err != nil || !equal {
		t.Fatalf("%s fixture = %v, %v; want %v", f.name, value, err, f.want)
	}
	if err := r.checkHooks(calls); err != nil {
		t.Fatal(err)
	}
	if err := r.checkBudget(calls); err != nil {
		t.Fatal(err)
	}
	if depth := r.thread.CallStackDepth(); depth != 0 {
		t.Fatalf("retained %d call frames", depth)
	}
}

func TestFixtures(t *testing.T) {
	for _, f := range fixtures() {
		for _, hooks := range hookModes() {
			for _, budget := range budgetModes() {
				t.Run(f.name+"/"+modeName(hooks, budget), func(t *testing.T) {
					r := prepare(t, f, hooks, budget)
					value, err := starlark.Call(r.thread, r.function, f.args, nil)
					if err != nil {
						t.Fatal(err)
					}
					r.validate(t, f, value, 1)
				})
			}
		}
	}
}

func BenchmarkParse(b *testing.B) {
	b.ReportAllocs()
	b.SetBytes(int64(len(program)))
	for i := 0; i < b.N; i++ {
		file, err := syntax.LegacyFileOptions().Parse("bench.star", program, 0)
		if err != nil || len(file.Stmts) != 3 {
			b.Fatalf("parse fixture: %v", err)
		}
	}
}

func BenchmarkLoop(b *testing.B)          { benchmarkCall(b, fixtures()[0]) }
func BenchmarkCalls(b *testing.B)         { benchmarkCall(b, fixtures()[1]) }
func BenchmarkConcat(b *testing.B)        { benchmarkCall(b, fixtures()[2]) }
func BenchmarkLiteral(b *testing.B)       { benchmarkCall(b, fixtures()[3]) }
func BenchmarkComprehension(b *testing.B) { benchmarkCall(b, fixtures()[4]) }

func benchmarkCall(b *testing.B, f fixture) {
	for _, hooks := range hookModes() {
		for _, budget := range budgetModes() {
			b.Run(modeName(hooks, budget), func(b *testing.B) {
				r := prepare(b, f, hooks, budget)
				var value starlark.Value
				var err error
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					value, err = starlark.Call(r.thread, r.function, f.args, nil)
					if err != nil {
						b.Fatal(err)
					}
				}
				b.StopTimer()
				r.validate(b, f, value, uint64(b.N))
			})
		}
	}
}
