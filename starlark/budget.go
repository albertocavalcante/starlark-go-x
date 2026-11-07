// Copyright 2026 Alberto Cavalcante. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package starlark

import (
	"fmt"
	"math"
)

// BudgetUsage counts declared logical resources, not Go heap or CPU usage.
// The interpreter charges Bytes for string/bytes repetition results and Elements
// for list/tuple repetition results. Work is charged only by cooperating hosts.
// Other operators, constructors, parsing, and builtins are not charged automatically.
type BudgetUsage struct {
	Bytes, Elements, Work uint64
}

// Budget is an optional, cumulative logical resource budget. Construct it with
// NewBudget and attach it to Thread.Budget before evaluation. Its counters span
// calls and modules using that thread. Use a new budget to reset them.
//
// A Budget must not be shared by concurrently executing threads or accessed while
// evaluation is running. It is not a memory sandbox, cancellation mechanism, or
// substitute for bounding inputs, host capabilities, and untrusted workers.
type Budget struct {
	limits BudgetUsage
	used   BudgetUsage
}

// NewBudget creates a budget with the given limits. A zero limit is unlimited;
// accounting overflow is rejected even for an unlimited resource.
func NewBudget(limits BudgetUsage) *Budget { return &Budget{limits: limits} }

// Usage returns successfully charged resources. A nil budget reports zero.
func (b *Budget) Usage() BudgetUsage {
	if b == nil {
		return BudgetUsage{}
	}
	return b.used
}

// Charge reserves resources before work begins. A nil budget is a no-op.
// On failure, no counters change. Hosts must propagate the returned error before
// performing the charged work; ignored errors do not stop arbitrary Go code.
// Charges are cumulative and are not refunded when results become unreachable.
func (b *Budget) Charge(amount BudgetUsage) error {
	if b == nil {
		return nil
	}
	for _, r := range [...]struct {
		name                string
		limit, used, amount uint64
	}{
		{"bytes", b.limits.Bytes, b.used.Bytes, amount.Bytes},
		{"elements", b.limits.Elements, b.used.Elements, amount.Elements},
		{"work", b.limits.Work, b.used.Work, amount.Work},
	} {
		limit := r.limit
		if limit == 0 {
			limit = math.MaxUint64
		}
		if r.amount > limit-r.used {
			return &BudgetError{Resource: r.name, Limit: limit, Used: r.used, Requested: r.amount}
		}
	}
	b.used.Bytes += amount.Bytes
	b.used.Elements += amount.Elements
	b.used.Work += amount.Work
	return nil
}

// BudgetError describes a refused charge. It is preserved as the cause of an
// EvalError, so callers can inspect it with errors.As without parsing messages.
type BudgetError struct {
	Resource               string
	Limit, Used, Requested uint64
}

func (e *BudgetError) Error() string {
	return fmt.Sprintf("resource budget exceeded: %s (limit %d, used %d, requested %d)", e.Resource, e.Limit, e.Used, e.Requested)
}
