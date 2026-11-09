//go:build !forkbudget

// Copyright 2026 Alberto Cavalcante. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bench

import "go.starlark.net/starlark"

func budgetModes() []bool { return []bool{false} }
func configureBudget(*starlark.Thread, bool, usage) func(uint64) error {
	return func(uint64) error { return nil }
}
