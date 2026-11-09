//go:build !forkhooks

// Copyright 2026 Alberto Cavalcante. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bench

import "go.starlark.net/starlark"

func hookModes() []bool { return []bool{false} }
func configureHooks(*starlark.Thread, bool, fixture) func(uint64) error {
	return func(uint64) error { return nil }
}
