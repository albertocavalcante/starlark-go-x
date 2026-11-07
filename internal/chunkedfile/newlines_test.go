// Copyright 2026 Alberto Cavalcante. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package chunkedfile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadNewlines(t *testing.T) {
	for _, newline := range []string{"\n", "\r\n"} {
		t.Run(strings.ReplaceAll(newline, "\n", "LF"), func(t *testing.T) {
			filename := filepath.Join(t.TempDir(), "fixture.star")
			source := strings.Join([]string{"first", "---", `second ### "expected"`, ""}, newline)
			if err := os.WriteFile(filename, []byte(source), 0600); err != nil {
				t.Fatal(err)
			}
			chunks := Read(filename, t)
			if len(chunks) != 2 {
				t.Fatalf("got %d chunks, want 2", len(chunks))
			}
			if chunks[0].Source != "first" || chunks[1].Source != "\n\nsecond ### \"expected\"\n" {
				t.Fatalf("wrong source/line padding: %q, %q", chunks[0].Source, chunks[1].Source)
			}
			chunks[1].GotError(3, "expected")
			chunks[1].Done()
		})
	}
}
