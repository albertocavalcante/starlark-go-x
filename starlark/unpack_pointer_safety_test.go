// Copyright 2026 Alberto Cavalcante. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package starlark

import (
	"fmt"
	"runtime"
	"strings"
	"testing"
)

func TestReflectUnpackLocalDoesNotAllocate(t *testing.T) {
	var input Value = String("metadata")
	allocs := testing.AllocsPerRun(100, func() {
		var output String // exercises the generic reflection branch, not *string
		if err := unpackArgNoEscape(input, &output); err != nil {
			t.Fatal(err)
		}
		if output != String("metadata") {
			t.Fatal("incorrect reflected assignment")
		}
	})
	if allocs != 0 {
		t.Fatalf("local reflected destination: %v allocations, want zero", allocs)
	}
}

func TestReflectUnpackStackAndGC(t *testing.T) {
	// A new goroutine starts with a small stack. Recursive frames grow that
	// stack around reflected writes; collection checks retained interface data.
	done := make(chan error, 1)
	go func() {
		for range 3 {
			if err := reflectUnpackFrames(32); err != nil {
				done <- err
				return
			}
			runtime.GC()
		}
		done <- nil
	}()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func reflectUnpackFrames(depth int) error {
	var padding [1024]byte
	padding[depth] = byte(depth)
	input := String(strings.Repeat("metadata", depth+1))
	var output String
	if err := unpackArgNoEscape(input, &output); err != nil {
		return err
	}
	if depth > 0 {
		if err := reflectUnpackFrames(depth - 1); err != nil {
			return err
		}
	} else {
		runtime.GC() // all caller destinations and padding remain live
	}
	if output != input || padding[depth] != byte(depth) {
		return fmt.Errorf("reflected destination changed after stack growth")
	}
	runtime.KeepAlive(padding)
	return nil
}

func BenchmarkReflectUnpackLocal(b *testing.B) {
	var input Value = String("metadata")
	b.ReportAllocs()
	for range b.N {
		var output String
		if err := unpackArgNoEscape(input, &output); err != nil {
			b.Fatal(err)
		}
	}
}
