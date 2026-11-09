//go:build (linux || darwin || dragonfly || freebsd || netbsd || solaris) && (amd64 || arm64 || mips64x || ppc64 || ppc64le || loong64 || s390x)

// Copyright 2026 Alberto Cavalcante. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package starlark

import (
	"math"
	"reflect"
	"runtime"
	"testing"
)

var intPointerSink Value

func TestIntPointerBoundariesSurviveGC(t *testing.T) {
	want := []int64{math.MinInt64, math.MinInt32 - 1, math.MinInt32, -1, 0, 1, math.MaxInt32, math.MaxInt32 + 1, math.MaxInt64}
	values := make([]Value, len(want))
	for i, n := range want {
		values[i] = MakeInt64(n)
	}
	for range 3 {
		runtime.GC()
		for i, value := range values {
			got, ok := value.(Int).Int64()
			if !ok || got != want[i] {
				t.Fatalf("integer %d after GC = %d (%v), want %d", i, got, ok, want[i])
			}
		}
	}
	runtime.KeepAlive(values)
}

func TestSmallIntInterfaceDoesNotAllocate(t *testing.T) {
	if reflect.ValueOf(smallints).IsZero() {
		t.Skip("mmap integer optimization unavailable; fallback has separate coverage")
	}
	for _, n := range []int64{math.MinInt32, -1, 0, 1, math.MaxInt32} {
		if allocs := testing.AllocsPerRun(100, func() { intPointerSink = MakeInt64(n) }); allocs != 0 {
			t.Fatalf("small integer %d: %v allocations, want zero", n, allocs)
		}
	}
}

func BenchmarkSmallIntPointer(b *testing.B) {
	b.ReportAllocs()
	for n := 0; n < b.N; n++ {
		intPointerSink = MakeInt64(int64(int32(n)))
	}
}
