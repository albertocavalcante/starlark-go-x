// Copyright 2026 Alberto Cavalcante. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package starlark_test

import (
	"testing"

	"go.starlark.net/starlark"
)

func BenchmarkBudgetSequence(b *testing.B) {
	for _, tc := range []struct{ name, source string }{
		{"Concat", "def f(x):\n    return x + x\n"},
		{"Literal", "def f(x):\n    return [x, x, x, x]\n"},
		{"Comprehension", "def f(x):\n    return [x for i in range(16)]\n"},
	} {
		for _, enabled := range []bool{false, true} {
			name := tc.name + "/Nil"
			if enabled {
				name = tc.name + "/Budget"
			}
			b.Run(name, func(b *testing.B) {
				thread := &starlark.Thread{}
				if enabled {
					thread.Budget = starlark.NewBudget(starlark.BudgetUsage{})
				}
				globals, err := starlark.ExecFile(thread, "benchmark.star", tc.source, nil)
				if err != nil {
					b.Fatal(err)
				}
				args := starlark.Tuple{starlark.String("abcdefgh")}
				b.ReportAllocs()
				b.ResetTimer()
				for range b.N {
					if _, err := starlark.Call(thread, globals["f"], args, nil); err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}
