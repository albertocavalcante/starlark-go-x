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

func BenchmarkLoop(b *testing.B)  { benchmarkCall(b, "loop", "499500") }
func BenchmarkCalls(b *testing.B) { benchmarkCall(b, "calls", "1000") }

func benchmarkCall(b *testing.B, name, want string) {
	for _, active := range hookModes() {
		label := "nil"
		if active {
			label = "active"
		}
		b.Run(label, func(b *testing.B) {
			thread := new(starlark.Thread)
			globals, err := starlark.ExecFile(thread, "bench.star", program, nil)
			if err != nil {
				b.Fatal(err)
			}
			validate := configureHooks(thread, active, name == "loop")
			args := starlark.Tuple{starlark.MakeInt(1000)}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				value, err := starlark.Call(thread, globals[name], args, nil)
				if err != nil || value.String() != want {
					b.Fatalf("%s fixture = %v, %v; want %s", name, value, err, want)
				}
			}
			b.StopTimer()
			if err := validate(); err != nil {
				b.Fatal(err)
			}
		})
	}
}
