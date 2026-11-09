# Pointer diagnostics and remaining review obligation

The mmap-backed small-integer representation now retains its mapping base as an
`unsafe.Pointer` and derives integer addresses with `unsafe.Add`. It no longer
reconstructs pointers from a stored numeric base. Range classification uses a
subtraction after the lower-bound comparison, avoiding upper-bound addition.
The mapping is never unmapped during process lifetime. Integer construction must
keep its existing signed-32-bit precondition before calculating the offset.

This change does not remove every use of `unsafe`, establish a complete memory
safety proof, or change the generic representation used on other platforms.
Small-integer interface conversion still allocates zero in its regression test.

## The existing unpack diagnostic remains visible

`go vet ./...` still reports the pointer reconstruction inside `noescape` in
`starlark/unpack.go`. No analyzer, diagnostic or test is disabled by this change.
This document is an assessment for independent maintainer review, not an
allowlist or permission to ignore future reports.

The assessed `starlark/unpack.go` SHA-256 is
`3981dcd86a5614380450e038046df56487c68fa397f49a011b65cd2c21a885f1`.
The public-reflection replacement probes used Go 1.27.3 on darwin/arm64.

The optimized reflection path depends on these invariants:

- Its destination is a live, non-nil Go pointer of the checked assignable type.
  It is supplied by the embedding Go program, not constructed by Starlark code.
- The target pointer is used synchronously for assignment, not retained. The
  optimized path excludes custom `Unpacker` callbacks that could retain it.
- The empty-interface representation used to obtain the data pointer matches the
  supported Go implementation. This is an implementation dependency, not a
  language-level representation guarantee.
- The pointer-to-integer round trip is inside a tiny `nosplit` helper with no
  calls or intervening safepoint. After return, the local is again an
  `unsafe.Pointer`, so subsequent reflection calls have pointer-typed liveness.
- The assigned value must retain ordinary Go escape/GC behavior. Hiding the
  target pointer must not allow a heap destination to retain stack-only data.

The implementation carries `nocheckptr` on this helper. Strict pointer checking
therefore cannot validate the hidden conversion itself. Stack-growth/GC tests,
ordinary and race suites, allocation checks, source review and the supported
toolchain matrix are complementary evidence, not a proof across future Go ABIs.

Public-reflection replacements were tried using direct `Value.Set`,
`Value.UnsafePointer` with `NewAt`, and the documented immediate `Value.Pointer`
conversion. Each introduced an allocation in the existing noescape regression
on the assessed Go toolchain. The escape effect propagates through the generic
unpack helper, so silently relaxing that test would change its performance
contract. These results do not justify replacing the optimization merely to
obtain a clean vet result.

Strict `checkptr=2` instrumentation itself changes escape/allocation behavior:
the unchanged unpack allocation assertion fails under that mode. Keep ordinary
allocation tests and strict-checkptr semantic tests separate; do not interpret
instrumented allocation counts as normal execution costs.

## Before treating the remaining diagnostic as understood debt

A maintainer and independent reviewer must assess this exact source, its
supported compiler versions, tests and real consumers. Any changed location,
source, toolchain, warning or caller requires renewed inspection. Raw vet must
remain visibly non-green until a reviewed safe implementation or an explicit
repository policy disposition is adopted. No filtering wrapper is supplied here.

Prefer a supported primitive or narrower typed unpack API if it can meet the
same behavior and allocation contract. Adding runtime linknames, assembly,
broader `nocheckptr`, or suppression to conceal the diagnostic would create
additional compatibility obligations and is not part of this change.
