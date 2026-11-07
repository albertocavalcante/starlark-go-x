// Copyright 2026 Alberto Cavalcante. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package starlark_test

import (
	"strings"
	"testing"

	"go.starlark.net/resolve"
	"go.starlark.net/starlark"
	"go.starlark.net/syntax"
)

func TestAnnotationsCannotExecute(t *testing.T) {
	for _, src := range []string{
		"def f(x: int): pass\nf(1)\n",
		"def f(x: int): return x\nf(1)\n",
		"def f(x: int = 42): pass\nf()\n",
		"def f(*args: int): pass\nf(1)\n",
		"def f(**kwargs: str): pass\nf(a=1)\n",
		"def f(*, x: int): pass\nf(x=1)\n",
		"def f() -> int: return 1\nf()\n",
		"def outer():\n    def f(x: int): pass\n    f(1)\nouter()\n",
	} {
		for _, api := range []string{"ExecFileOptions", "SourceProgramOptions", "FileProgram", "resolve.File", "resolve.REPLChunk"} {
			t.Run(api+"/"+src, func(t *testing.T) {
				defer func() {
					if p := recover(); p != nil {
						t.Errorf("unsupported annotations caused a host panic: %v", p)
					}
				}()
				opts := &syntax.FileOptions{Types: syntax.TypesParseOnly}
				predeclared := starlark.StringDict{}
				f, err := opts.Parse("typed.star", src, 0)
				if err != nil {
					t.Fatal(err)
				}
				switch api {
				case "ExecFileOptions":
					_, err = starlark.ExecFileOptions(opts, new(starlark.Thread), "typed.star", src, nil)
				case "SourceProgramOptions":
					_, _, err = starlark.SourceProgramOptions(opts, "typed.star", src, predeclared.Has)
				case "FileProgram":
					// The parsed AST must remain guarded even if its options change.
					f.Options = &syntax.FileOptions{}
					_, err = starlark.FileProgram(f, predeclared.Has)
				case "resolve.File":
					err = resolve.File(f, predeclared.Has, starlark.Universe.Has)
				case "resolve.REPLChunk":
					err = resolve.REPLChunk(f, nil, predeclared.Has, starlark.Universe.Has)
				}
				if err == nil || !strings.Contains(err.Error(), "type annotations are not supported during evaluation") {
					t.Errorf("got %v, want unsupported annotation error", err)
				}
			})
		}
	}
}

func TestAnnotationExpressionsCannotExecute(t *testing.T) {
	opts := &syntax.FileOptions{Types: syntax.TypesParseOnly}
	f, err := opts.Parse("typed.star", "def f(x: int): pass\n", 0)
	if err != nil {
		t.Fatal(err)
	}
	param := f.Stmts[0].(*syntax.DefStmt).Params[0].(*syntax.TypedParam)
	for _, expr := range []syntax.Expr{param, param.Type} {
		t.Run("expression", func(t *testing.T) {
			defer func() {
				if p := recover(); p != nil {
					t.Errorf("%T caused a host panic: %v", expr, p)
				}
			}()
			if _, err := starlark.EvalExprOptions(opts, new(starlark.Thread), expr, nil); err == nil || !strings.Contains(err.Error(), "type annotations are not supported during evaluation") {
				t.Errorf("%T: got %v, want unsupported annotation error", expr, err)
			}
		})
	}
}

func TestStaticTypeCheckingUnavailable(t *testing.T) {
	opts := &syntax.FileOptions{Types: syntax.TypesEnabled}
	_, err := starlark.ExecFileOptions(opts, new(starlark.Thread), "typed.star", "def f() -> int: return \"wrong type\"\nf()\n", nil)
	if err == nil || !strings.Contains(err.Error(), "static type checking is not supported") {
		t.Errorf("ExecFileOptions: got %v, want unsupported checking error", err)
	}
	if _, err := starlark.EvalOptions(opts, new(starlark.Thread), "typed.star", "1", nil); err == nil || !strings.Contains(err.Error(), "static type checking is not supported") {
		t.Errorf("EvalOptions: got %v, want unsupported checking error", err)
	}
}

func TestParseOnlyUnannotatedExecution(t *testing.T) {
	opts := &syntax.FileOptions{Types: syntax.TypesParseOnly}
	src := "def f(x=1, *args, **kwargs):\n    return x + args[0] + kwargs[\"y\"]\nresult = f(2, 3, y=4)\n"
	globals, err := starlark.ExecFileOptions(opts, new(starlark.Thread), "plain.star", src, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := globals["result"]; got != starlark.MakeInt(9) {
		t.Errorf("result = %v, want 9", got)
	}
}
