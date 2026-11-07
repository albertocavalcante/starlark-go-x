// Copyright 2026 Alberto Cavalcante. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package syntax_test

import (
	"testing"

	"go.starlark.net/syntax"
)

func TestAnnotationModeLambdas(t *testing.T) {
	for _, mode := range []syntax.TypeMode{syntax.TypesDisabled, syntax.TypesParseOnly} {
		opts := &syntax.FileOptions{Types: mode}
		for _, src := range []string{
			"lambda: 1",
			"lambda x: x",
			"lambda x, y: x + y",
			"lambda x=1: x",
			"lambda *args: args",
			"lambda **kwargs: kwargs",
			"lambda x, *, y: x + y",
			"lambda x: lambda y: x + y",
		} {
			if _, err := opts.ParseExpr("lambda.star", src, 0); err != nil {
				t.Errorf("mode %d, %s: %v", mode, src, err)
			}
		}
	}
	opts := &syntax.FileOptions{Types: syntax.TypesParseOnly}
	if _, err := opts.Parse("lambda.star", "def f(x: int):\n    return lambda y: y\n", 0); err != nil {
		t.Errorf("lambda in annotated function: %v", err)
	}
}
